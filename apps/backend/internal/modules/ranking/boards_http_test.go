package ranking_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/sync/errgroup"

	"github.com/monaco/monaco/apps/backend/internal/modules/ranking/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/ranking/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	apibase "github.com/monaco/monaco/apps/backend/internal/platform/httpx/api"
	api "github.com/monaco/monaco/apps/backend/internal/platform/httpx/api/rankingapi"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

type boardRow struct {
	board   string
	rank    int
	subject uuid.UUID
	bps     *int64
	value   int64
	handle  *string
}

func seedBoard(t *testing.T, pool *pgxpool.Pool, at time.Time, rows []boardRow) uuid.UUID {
	t.Helper()
	run := ids.Real{}.NewV7()
	seedBoardRun(t, pool, at, run, rows)
	return run
}

func seedBoardRun(t *testing.T, pool *pgxpool.Pool, at time.Time, run uuid.UUID, rows []boardRow) {
	t.Helper()
	q := sqlc.New(pool)
	if err := q.InsertLeaderboardRun(t.Context(), sqlc.InsertLeaderboardRunParams{
		RunID: run, AsOf: at, PricesAsOf: at, StartedAt: at, FinishedAt: at,
	}); err != nil {
		t.Fatal(err)
	}
	wire := make([]map[string]any, 0, len(rows))
	for _, r := range rows {
		wire = append(wire, map[string]any{
			"board": r.board, "range": "ALL", "rank": r.rank, "subject_id": r.subject.String(),
			"subject_name": "Subject", "subject_handle": r.handle, "subject_created_at": at.Format(time.RFC3339Nano),
			"value_micros": r.value, "pnl_micros": -5, "return_bps": r.bps,
			"prices_as_of": at.Format(time.RFC3339Nano), "computed_at": at.Format(time.RFC3339Nano),
			"flags": []string{"unpriced_assets"},
		})
	}
	raw, err := json.Marshal(wire)
	if err != nil {
		t.Fatal(err)
	}
	if err := q.InsertLeaderboardEntries(t.Context(), raw); err != nil {
		t.Fatal(err)
	}
}

func boardRows(board string, n int) []boardRow {
	rows := make([]boardRow, 0, n)
	for i := 1; i <= n; i++ {
		bps := int64(500 - i)
		rows = append(rows, boardRow{
			board: board, rank: i, subject: ids.Real{}.NewV7(), bps: &bps, value: int64(1000 * i),
		})
	}
	return rows
}

func TestBoards_InvalidRange(t *testing.T) {
	t.Parallel()
	s := newServer(t)
	viewer := ids.UserIDFrom(ids.Real{}.NewV7())
	for _, query := range []string{"range=2Y", "limit=0", "limit=51", "cursor=!!", "cursor=LTE"} {
		rec := s.get(t, "/v1/leaderboards/cabals?"+query, viewer)
		if rec.Code != http.StatusBadRequest || problemOf(t, rec) != apibase.InvalidInput {
			t.Errorf("GET ?%s = %d %s, want 400 invalid_input", query, rec.Code, rec.Body)
		}
	}
}

func TestBoards_CabalsRouteServesAPageFromTheLatestRun(t *testing.T) {
	t.Parallel()
	s := newServer(t)
	at := s.clock.Now().UTC()
	run := seedBoard(t, s.pool, at, boardRows("cabals", 3))
	viewer := ids.UserIDFrom(ids.Real{}.NewV7())
	rec := s.get(t, "/v1/leaderboards/cabals?limit=2", viewer)
	page := pageOf(t, rec)
	if rec.Code != http.StatusOK || page.RunId == nil || *page.RunId != run || page.Board != "cabals" ||
		page.Range != api.LeaderboardPageRangeALL || len(page.Rows) != 2 || page.NextCursor == nil || page.Me != nil {
		t.Fatalf("GET = %d %s", rec.Code, rec.Body)
	}
	assertFirstRow(t, page.Rows[0])
	rec = s.get(t, "/v1/leaderboards/cabals?limit=2&cursor="+*page.NextCursor, viewer)
	if next := pageOf(t, rec); len(next.Rows) != 1 || next.Rows[0].Rank != 3 || next.NextCursor != nil {
		t.Fatalf("second page = %d %s", rec.Code, rec.Body)
	}
}

func assertFirstRow(t *testing.T, first api.LeaderboardRow) {
	t.Helper()
	if first.Rank != 1 || first.Subject.Kind != api.Cabal || first.ValueMicros != 1000 || first.PnlMicros != -5 ||
		first.ReturnBps == nil || *first.ReturnBps != 499 || len(first.Flags) != 1 || first.Subject.Handle != nil {
		t.Fatalf("first row = %+v", first)
	}
}

func TestBoards_CabalsRouteIsEmptyBeforeTheFirstRunAndNeedsAToken(t *testing.T) {
	t.Parallel()
	s := newServer(t)
	rec := s.get(t, "/v1/leaderboards/cabals?range=1D", ids.UserIDFrom(ids.Real{}.NewV7()))
	if page := pageOf(t, rec); rec.Code != http.StatusOK || page.RunId != nil || len(page.Rows) != 0 ||
		page.Range != api.LeaderboardPageRangeN1D {
		t.Fatalf("GET = %d %s", rec.Code, rec.Body)
	}
	if rec = s.get(t, "/v1/leaderboards/cabals", ids.UserID{}); rec.Code != http.StatusUnauthorized {
		t.Fatalf("GET without a token = %d, want 401", rec.Code)
	}
}

func TestBoards_PeopleIncludesMe(t *testing.T) {
	t.Parallel()
	s := newServer(t)
	rows := boardRows("people", 45)
	seedBoard(t, s.pool, s.clock.Now().UTC(), rows)
	viewer := ids.UserIDFrom(rows[44].subject)
	for _, query := range []string{"", "?cursor=" + domain.EncodeCursor(40) + "&limit=20"} {
		rec := s.get(t, "/v1/leaderboards/people"+query, viewer)
		page := pageOf(t, rec)
		if rec.Code != http.StatusOK || page.Me == nil || page.Me.Rank != 45 || page.Me.Subject.Kind != api.User ||
			page.Me.Subject.Id != rows[44].subject || page.Rows[0].Subject.Kind != api.User {
			t.Fatalf("GET people%s = %d %s", query, rec.Code, rec.Body)
		}
	}
}

func TestBoards_PeopleMeIsNullForAViewerOnNoBoard(t *testing.T) {
	t.Parallel()
	s := newServer(t)
	seedBoard(t, s.pool, s.clock.Now().UTC(), boardRows("people", 2))
	rec := s.get(t, "/v1/leaderboards/people", ids.UserIDFrom(ids.Real{}.NewV7()))
	if page := pageOf(t, rec); rec.Code != http.StatusOK || page.Me != nil || len(page.Rows) != 2 {
		t.Fatalf("GET = %d %s", rec.Code, rec.Body)
	}
	if rec = s.get(t, "/v1/leaderboards/people", ids.UserID{}); rec.Code != http.StatusUnauthorized {
		t.Fatalf("GET without a token = %d, want 401", rec.Code)
	}
}

func TestBoards_UnknownCabal(t *testing.T) {
	t.Parallel()
	s := newServer(t)
	viewer := ids.UserIDFrom(ids.Real{}.NewV7())
	rec := s.get(t, "/v1/cabals/"+ids.Real{}.NewV7().String()+"/leaderboard", viewer)
	if rec.Code != http.StatusNotFound || problemOf(t, rec) != apibase.CabalNotFound {
		t.Fatalf("GET = %d %s, want 404 cabal_not_found", rec.Code, rec.Body)
	}
	if rec = s.get(t, "/v1/cabals/"+ids.Real{}.NewV7().String()+"/leaderboard", ids.UserID{}); rec.Code != 401 {
		t.Fatalf("GET without a token = %d, want 401", rec.Code)
	}
}

func TestBoards_MembersBoardListsZeroStakeMembers(t *testing.T) {
	t.Parallel()
	s := newServer(t)
	cabal := testkit.NewCabal(t, s.pool, testkit.WithMembers(2))
	board := domain.MembersBoard(cabal.ID.UUID())
	staked, idle := cabal.Members[0], cabal.Members[1]
	bps := int64(300)
	rows := []boardRow{
		{board: board, rank: 1, subject: staked.ID.UUID(), bps: &bps, value: 2000},
		{board: board, rank: 2, subject: idle.ID.UUID()},
	}
	seedBoard(t, s.pool, s.clock.Now().UTC(), rows)
	rec := s.get(t, "/v1/cabals/"+cabal.ID.String()+"/leaderboard", idle.ID)
	page := pageOf(t, rec)
	if rec.Code != http.StatusOK || page.Board != board || len(page.Rows) != 2 {
		t.Fatalf("GET = %d %s", rec.Code, rec.Body)
	}
	zero := page.Rows[1]
	if zero.Rank != 2 || zero.ReturnBps != nil || zero.ValueMicros != 0 || zero.Subject.Kind != api.User {
		t.Fatalf("zero-stake row = %+v", zero)
	}
	if page.Me == nil || page.Me.Rank != 2 || page.Me.ReturnBps != nil || page.Me.Subject.Id != idle.ID.UUID() {
		t.Fatalf("me = %+v, want the zero-stake member's own row", page.Me)
	}
}

func updateBoard(t *testing.T, s server, query string, args ...any) {
	t.Helper()
	if _, err := s.pool.Exec(t.Context(), query, args...); err != nil {
		t.Fatal(err)
	}
}

func TestBoards_AHitIssuesOneQueryOnTheCabalsBoard(t *testing.T) {
	t.Parallel()
	s := newServer(t)
	seedBoard(t, s.pool, s.clock.Now().UTC(), boardRows("cabals", 30))
	viewer := ids.UserIDFrom(ids.Real{}.NewV7())
	testkit.AssertQueries(t, "cabals board miss", func() { s.get(t, "/v1/leaderboards/cabals", viewer) })
	testkit.AssertQueries(t, "cabals board hit", func() { s.get(t, "/v1/leaderboards/cabals", viewer) })
	testkit.AssertQueries(t, "people board miss with me", func() { s.get(t, "/v1/leaderboards/people", viewer) })
	testkit.AssertQueries(t, "people board hit with me", func() { s.get(t, "/v1/leaderboards/people", viewer) })
}

func TestBoards_ANewRunChangesTheServedRunWithNoRestart(t *testing.T) {
	t.Parallel()
	s := newServer(t)
	at := s.clock.Now().UTC()
	first := seedBoard(t, s.pool, at, boardRows("cabals", 2))
	viewer := ids.UserIDFrom(ids.Real{}.NewV7())
	if page := pageOf(t, s.get(t, "/v1/leaderboards/cabals", viewer)); page.RunId == nil || *page.RunId != first {
		t.Fatalf("first page run = %v, want %s", page.RunId, first)
	}
	second := ids.Real{}.NewV7()
	updateBoard(t, s, `INSERT INTO leaderboard_runs
		(run_id, as_of, prices_as_of, started_at, finished_at, rows_written, cabals_excluded)
		VALUES ($1, $2, $2, $2, $2, 0, 0)`, second, at.Add(time.Minute))
	if page := pageOf(t, s.get(t, "/v1/leaderboards/cabals", viewer)); page.RunId == nil || *page.RunId != second {
		t.Fatalf("page run after a new run = %v, want %s", page.RunId, second)
	}
}

func TestBoards_ARevBumpChangesTheCacheKey(t *testing.T) {
	t.Parallel()
	s := newServer(t)
	seedBoard(t, s.pool, s.clock.Now().UTC(), boardRows("cabals", 2))
	viewer := ids.UserIDFrom(ids.Real{}.NewV7())
	if page := pageOf(t, s.get(t, "/v1/leaderboards/cabals", viewer)); page.Rows[0].ValueMicros != 1000 {
		t.Fatalf("value = %d, want 1000", page.Rows[0].ValueMicros)
	}
	updateBoard(t, s, "UPDATE leaderboard_entries SET value_micros = 5 WHERE rank = 1")
	if page := pageOf(t, s.get(t, "/v1/leaderboards/cabals", viewer)); page.Rows[0].ValueMicros != 1000 {
		t.Fatalf("value = %d, want the cached 1000 while the run and rev are unchanged", page.Rows[0].ValueMicros)
	}
	updateBoard(t, s, "UPDATE leaderboard_runs SET rev = rev + 1")
	if page := pageOf(t, s.get(t, "/v1/leaderboards/cabals", viewer)); page.Rows[0].ValueMicros != 5 {
		t.Fatalf("value = %d, want 5 after the rev bump", page.Rows[0].ValueMicros)
	}
}

func TestBoards_ConcurrentMissesRunOnePageQuery(t *testing.T) {
	t.Parallel()
	s := newServer(t)
	seedBoard(t, s.pool, s.clock.Now().UTC(), boardRows("cabals", 30))
	viewer := ids.UserIDFrom(ids.Real{}.NewV7())
	testkit.AssertQueries(t, "cabals board 20 concurrent misses", func() {
		var group errgroup.Group
		for range 20 {
			group.Go(func() error {
				if rec := s.get(t, "/v1/leaderboards/cabals", viewer); rec.Code != http.StatusOK {
					t.Errorf("GET = %d %s", rec.Code, rec.Body)
				}
				return nil
			})
		}
		if err := group.Wait(); err != nil {
			t.Error(err)
		}
	})
}

func TestBoards_PeopleP95(t *testing.T) {
	s := newServer(t)
	rows := boardRows("people", 10000)
	seedBoard(t, s.pool, s.clock.Now().UTC(), rows)
	viewer := ids.UserIDFrom(rows[9999].subject)
	timedHandlerOnly := func(i int) time.Duration {
		req := httptest.NewRequestWithContext(t.Context(), http.MethodGet,
			"/v1/leaderboards/people?limit=20&cursor="+domain.EncodeCursor(i%100*20), nil)
		req.Header.Set("Authorization", "Bearer "+s.verifier.Mint(viewer.String(), s.clock.Now().Add(time.Hour)))
		rec := httptest.NewRecorder()
		start := clock.Real{}.Now()
		s.bare.ServeHTTP(rec, req)
		d := clock.Real{}.Now().Sub(start)
		if rec.Code != http.StatusOK {
			t.Fatalf("GET = %d %s", rec.Code, rec.Body)
		}
		return d
	}
	for i := range 20 {
		timedHandlerOnly(i)
	}
	took := make([]time.Duration, 0, 200)
	for i := range 200 {
		took = append(took, timedHandlerOnly(i))
	}
	slices.Sort(took)
	if p95 := took[189]; p95 >= 50*time.Millisecond {
		t.Fatalf("p95 = %s over 200 requests on 10000 rows, want under 50ms", p95)
	}
}
