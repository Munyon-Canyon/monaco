package ranking_test

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/monaco/monaco/apps/backend/internal/modules/ranking/sqlc"
	apibase "github.com/monaco/monaco/apps/backend/internal/platform/httpx/api"
	api "github.com/monaco/monaco/apps/backend/internal/platform/httpx/api/rankingapi"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

type boardRow struct {
	board   string
	rank    int
	subject uuid.UUID
	bps     *int64
	handle  *string
}

func seedBoard(t *testing.T, pool *pgxpool.Pool, at time.Time, rows []boardRow) uuid.UUID {
	t.Helper()
	q := sqlc.New(pool)
	run := ids.Real{}.NewV7()
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
			"value_micros": 1000 * r.rank, "pnl_micros": -5, "return_bps": r.bps,
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
	return run
}

func boardRows(board string, n int) []boardRow {
	rows := make([]boardRow, 0, n)
	for i := 1; i <= n; i++ {
		bps := int64(500 - i)
		rows = append(rows, boardRow{board: board, rank: i, subject: ids.Real{}.NewV7(), bps: &bps})
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
