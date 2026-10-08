package analytics_test

import (
	"context"
	"net/http"
	"slices"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/analytics/adapters"
	"github.com/monaco/monaco/apps/backend/internal/modules/analytics/app"
	dbsqlc "github.com/monaco/monaco/apps/backend/internal/platform/db/sqlc"
	httpxapi "github.com/monaco/monaco/apps/backend/internal/platform/httpx/api"
	api "github.com/monaco/monaco/apps/backend/internal/platform/httpx/api/analyticsapi"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

func seedSafety(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	s := signalSeed{t: t, pool: pool, ids: testkit.NewIDs(12)}
	s.event("cabal.paused", at(1, 9, 0), map[string]any{"reason": "low_relayer"})
	s.event("cabal.paused", at(1, 10, 0), map[string]any{"reason": "low_relayer"})
	s.event("cabal.paused", at(2, 9, 0), map[string]any{"reason": "ops"})
	s.event("cabal.external_deposit_detected", at(2, 10, 0), map[string]any{})
	s.event("cabal.external_deposit_bounced", at(2, 11, 0), map[string]any{})
	s.event("cabal.external_deposit_bounced", at(9, 11, 0), map[string]any{})
	s.event("admin.action", at(3, 9, 0), map[string]any{"action": "ops_pause"})
	s.event("admin.action", at(3, 10, 0), map[string]any{"action": "user_ban"})
	s.event("admin.action", at(3, 11, 0), map[string]any{"action": "user_ban"})
	s.event("admin.action", at(20, 11, 0), map[string]any{"action": "user_ban"})
	banned := testkit.NewCabal(t, pool, testkit.WithMembers(1))
	testkit.NewCabal(t, pool, testkit.WithMembers(1))
	if _, err := pool.Exec(
		t.Context(),
		`UPDATE cabals SET status = 'banned' WHERE id = $1`,
		banned.ID.UUID(),
	); err != nil {
		t.Fatal(err)
	}
	testkit.SeedUser(t, pool, testkit.UserOpts{AccountStatus: "banned"})
	testkit.SeedUser(t, pool, testkit.UserOpts{AccountStatus: "banned", AuthState: "ONBOARDING_COMPLETED"})
	testkit.SeedUser(t, pool, testkit.UserOpts{AccountStatus: "suspended"})
}

func TestDashboard_Safety_ReadsPausesDepositsAndAdminActions(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	seedSafety(t, pool)
	var got api.SafetyDashboard
	decode(t, dashboardGet(t, productHandler(t, pool), "safety", viewerToken,
		window("2026-09-01", "2026-09-15", "day")), &got)
	want := []api.DashboardPoint{
		{BucketStart: at(1, 0, 0), Metric: "cabals_paused", Group: "low_relayer", Count: 2},
		{BucketStart: at(2, 0, 0), Metric: "cabals_paused", Group: "ops", Count: 1},
		{BucketStart: at(2, 0, 0), Metric: "external_deposits_bounced", Count: 1},
		{BucketStart: at(2, 0, 0), Metric: "external_deposits_detected", Count: 1},
		{BucketStart: at(3, 0, 0), Metric: "admin_actions", Group: "ops_pause", Count: 1},
		{BucketStart: at(3, 0, 0), Metric: "admin_actions", Group: "user_ban", Count: 2},
		{BucketStart: at(9, 0, 0), Metric: "external_deposits_bounced", Count: 1},
	}
	slices.SortFunc(got.Series, comparePoints)
	slices.SortFunc(want, comparePoints)
	if !slices.Equal(got.Series, want) || got.BannedUsers != 2 || got.BannedCabals != 1 {
		t.Fatalf("series %+v banned users %d cabals %d, want %+v, 2 and 1",
			got.Series, got.BannedUsers, got.BannedCabals, want)
	}
}

func TestDashboard_Safety_RefusesABadWindow(t *testing.T) {
	t.Parallel()
	w := dashboardGet(t, productHandler(t, testkit.DB(t)), "safety", viewerToken,
		window("2026-09-02", "2026-09-01", "week"))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d %s, want 400", w.Code, w.Body)
	}
}

func TestDashboard_Safety_QueryCount(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	seedSafety(t, pool)
	h := productHandler(t, pool)
	testkit.AssertQueries(t, "analytics GetSafetyDashboard", func() {
		w := dashboardGet(t, h, "safety", viewerToken, window("2026-09-01", "2026-09-15", "week"))
		if w.Code != http.StatusOK {
			t.Fatalf("status = %d %s", w.Code, w.Body)
		}
	})
}

func fakeSafety(e countFake, c cabalCountsFake, u userCountsFake) app.Safety {
	return app.Safety{
		Read: func(ctx context.Context, fn func(context.Context, dbsqlc.DBTX) error) error { return fn(ctx, nil) },
		Bind: func(dbsqlc.DBTX) app.SafetySources { return app.SafetySources{Events: e, Cabals: c, Users: u} },
	}
}

func TestSafety_Dashboard_Failures(t *testing.T) {
	t.Parallel()
	boom := errs.New(errs.CodeInternal, "test")
	tests := map[string]app.Safety{
		"events": fakeSafety(countFake{err: boom}, cabalCountsFake{}, userCountsFake{}),
		"cabals": fakeSafety(countFake{}, cabalCountsFake{err: boom}, userCountsFake{}),
		"users":  fakeSafety(countFake{}, cabalCountsFake{}, userCountsFake{err: boom}),
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

func TestDashboard_Safety_ReadFailureAnswers500(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	failing := fakeSafety(
		countFake{err: errs.New(errs.CodeInternal, "test")}, cabalCountsFake{}, userCountsFake{},
	)
	h := dashboardHandler(t, pool, func(m httpxapi.Mount) { api.Mount(adapters.HTTP{Safety: failing}, m) })
	w := dashboardGet(t, h, "safety", viewerToken, window("2026-09-01", "2026-09-02", "day"))
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d %s, want 500", w.Code, w.Body)
	}
}
