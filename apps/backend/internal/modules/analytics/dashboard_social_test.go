package analytics_test

import (
	"context"
	"encoding/json"
	"net/http"
	"slices"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/analytics/adapters"
	"github.com/monaco/monaco/apps/backend/internal/modules/analytics/app"
	cabalport "github.com/monaco/monaco/apps/backend/internal/modules/cabal/port"
	identityport "github.com/monaco/monaco/apps/backend/internal/modules/identity/port"
	"github.com/monaco/monaco/apps/backend/internal/platform/bus"
	dbsqlc "github.com/monaco/monaco/apps/backend/internal/platform/db/sqlc"
	httpxapi "github.com/monaco/monaco/apps/backend/internal/platform/httpx/api"
	api "github.com/monaco/monaco/apps/backend/internal/platform/httpx/api/analyticsapi"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

type signalSeed struct {
	t    *testing.T
	pool *pgxpool.Pool
	ids  *testkit.IDs
}

func (s signalSeed) event(typ string, when time.Time, payload map[string]any) {
	s.t.Helper()
	payload["v"] = 1
	raw, err := json.Marshal(payload)
	if err != nil {
		s.t.Fatal(err)
	}
	if _, err := s.pool.Exec(s.t.Context(), `INSERT INTO events
		(id, aggregate_type, aggregate_id, type, payload, actor_type, actor_id, created_at)
		VALUES ($1, 'test', $2, $3, $4, 'system', 'test', $5)`,
		s.ids.NewV7(), s.ids.NewV7(), typ, raw, when); err != nil {
		s.t.Fatal(err)
	}
}

func seedSocial(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	s := signalSeed{t: t, pool: pool, ids: testkit.NewIDs(11)}
	for _, hour := range []int{9, 10} {
		s.event("user.created", at(1, hour, 0), map[string]any{})
	}
	s.event("user.created", at(8, 9, 0), map[string]any{})
	s.event("follow.created", at(1, 9, 0), map[string]any{"source": "contacts"})
	s.event("follow.created", at(1, 10, 0), map[string]any{"source": "contacts"})
	s.event("follow.created", at(2, 11, 0), map[string]any{"source": "search"})
	s.event("follow.removed", at(2, 12, 0), map[string]any{})
	s.event("comment.created", at(3, 1, 0), map[string]any{"parent_comment_id": nil})
	s.event("comment.created", at(3, 2, 0), map[string]any{"parent_comment_id": "c1"})
	s.event("comment.created", at(20, 2, 0), map[string]any{"parent_comment_id": "c1"})
	testkit.NewCabal(t, pool, testkit.WithMembers(2))
	banned := testkit.NewCabal(t, pool, testkit.WithMembers(4))
	if _, err := pool.Exec(
		t.Context(),
		`UPDATE cabals SET status = 'banned' WHERE id = $1`,
		banned.ID.UUID(),
	); err != nil {
		t.Fatal(err)
	}
	testkit.SeedUser(t, pool, testkit.UserOpts{AuthState: "AWAITING_PHONE"})
	testkit.SeedUser(t, pool, testkit.UserOpts{AccountStatus: "deleted"})
}

func TestDashboard_Social_FollowsBySource(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	seedSocial(t, pool)
	var got api.SocialDashboard
	decode(t, dashboardGet(t, productHandler(t, pool), "social", viewerToken,
		window("2026-09-01", "2026-09-15", "day")), &got)
	want := []api.DashboardPoint{
		{BucketStart: at(1, 0, 0), Metric: "follows_created", Group: "contacts", Count: 2},
		{BucketStart: at(1, 0, 0), Metric: "users_created", Count: 2},
		{BucketStart: at(2, 0, 0), Metric: "follows_created", Group: "search", Count: 1},
		{BucketStart: at(2, 0, 0), Metric: "follows_removed", Count: 1},
		{BucketStart: at(3, 0, 0), Metric: "comments_created", Count: 2},
		{BucketStart: at(3, 0, 0), Metric: "replies_created", Count: 1},
		{BucketStart: at(8, 0, 0), Metric: "users_created", Count: 1},
	}
	slices.SortFunc(got.Series, comparePoints)
	slices.SortFunc(want, comparePoints)
	if !slices.Equal(got.Series, want) {
		t.Fatalf("series = %+v, want %+v", got.Series, want)
	}
}

func comparePoints(a, b api.DashboardPoint) int {
	if c := a.BucketStart.Compare(b.BucketStart); c != 0 {
		return c
	}
	if a.Metric != b.Metric {
		if a.Metric < b.Metric {
			return -1
		}
		return 1
	}
	if a.Group < b.Group {
		return -1
	}
	if a.Group > b.Group {
		return 1
	}
	return 0
}

func TestDashboard_Social_CountsUsersAndCabals(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	seedSocial(t, pool)
	var got api.SocialDashboard
	decode(t, dashboardGet(t, productHandler(t, pool), "social", viewerToken,
		window("2026-09-01", "2026-09-15", "week")), &got)
	wantCabals := api.DashboardCabals{Total: 2, Banned: 1, MembersP50: 2, MembersP90: 4, MembersMax: 4}
	if got.Cabals != wantCabals {
		t.Errorf("cabals = %+v, want %+v", got.Cabals, wantCabals)
	}
	states := map[[2]string]int64{}
	for _, s := range got.UserStates {
		states[[2]string{s.AuthState, s.AccountStatus}] = s.Users
	}
	if got.UsersTotal != 8 || states[[2]string{"AWAITING_PHONE", "active"}] != 1 ||
		states[[2]string{"CREATED", "deleted"}] != 1 {
		t.Errorf("users_total %d states %v, want 8 users with the seeded states", got.UsersTotal, states)
	}
}

func TestDashboard_Social_RefusesABadWindow(t *testing.T) {
	t.Parallel()
	w := dashboardGet(t, productHandler(t, testkit.DB(t)), "social", viewerToken,
		window("2026-09-02", "2026-09-01", "day"))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d %s, want 400", w.Code, w.Body)
	}
}

func TestDashboard_Social_QueryCount(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	seedSocial(t, pool)
	h := productHandler(t, pool)
	testkit.AssertQueries(t, "analytics GetSocialDashboard", func() {
		w := dashboardGet(t, h, "social", viewerToken, window("2026-09-01", "2026-09-15", "day"))
		if w.Code != http.StatusOK {
			t.Fatalf("status = %d %s", w.Code, w.Body)
		}
	})
}

type countFake struct {
	rows []bus.EventCount
	err  error
}

func (f countFake) CountEvents(context.Context, bus.CountQuery) ([]bus.EventCount, error) {
	return f.rows, f.err
}

type cabalCountsFake struct {
	counts cabalport.Counts
	err    error
}

func (f cabalCountsFake) Counts(context.Context) (cabalport.Counts, error) { return f.counts, f.err }

type userCountsFake struct {
	rows []identityport.StatusCount
	err  error
}

func (f userCountsFake) StatusCounts(context.Context) ([]identityport.StatusCount, error) {
	return f.rows, f.err
}

func fakeSocial(e countFake, c cabalCountsFake, u userCountsFake) app.Social {
	return app.Social{
		Read: func(ctx context.Context, fn func(context.Context, dbsqlc.DBTX) error) error { return fn(ctx, nil) },
		Bind: func(dbsqlc.DBTX) app.SocialSources { return app.SocialSources{Events: e, Cabals: c, Users: u} },
	}
}

func TestSocial_Dashboard_Failures(t *testing.T) {
	t.Parallel()
	boom := errs.New(errs.CodeInternal, "test")
	tests := map[string]app.Social{
		"events": fakeSocial(countFake{err: boom}, cabalCountsFake{}, userCountsFake{}),
		"cabals": fakeSocial(countFake{}, cabalCountsFake{err: boom}, userCountsFake{}),
		"users":  fakeSocial(countFake{}, cabalCountsFake{}, userCountsFake{err: boom}),
	}
	for name, dash := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if got, err := dash.Dashboard(t.Context(), app.Window{}); err == nil {
				t.Fatalf("Dashboard() = %+v, want an error", got)
			}
		})
	}
}

func TestDashboard_Social_ReadFailureAnswers500(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	failing := fakeSocial(
		countFake{err: errs.New(errs.CodeInternal, "test")}, cabalCountsFake{}, userCountsFake{},
	)
	h := dashboardHandler(t, pool, func(m httpxapi.Mount) { api.Mount(adapters.HTTP{Social: failing}, m) })
	w := dashboardGet(t, h, "social", viewerToken, window("2026-09-01", "2026-09-02", "day"))
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d %s, want 500", w.Code, w.Body)
	}
}
