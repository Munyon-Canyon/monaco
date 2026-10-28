package ranking_test

import (
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/modules/ranking"
	"github.com/monaco/monaco/apps/backend/internal/modules/ranking/port"
	"github.com/monaco/monaco/apps/backend/internal/modules/ranking/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
)

func seedCabalReturn(t *testing.T, s server, at time.Time, rng string, rank int, cabal uuid.UUID, bps *int64) {
	t.Helper()
	_, err := s.pool.Exec(t.Context(), `INSERT INTO leaderboard_entries (
		board, range, rank, subject_id, subject_name, subject_created_at, value_micros, pnl_micros, return_bps,
		prices_as_of, computed_at, join_mode, member_count
	) VALUES ('cabals', $1, $2, $3, 'AC Capital', $4, 20000000000, -5, $5, $4, $4, 'request', 8)`,
		rng, rank, cabal, at, bps)
	if err != nil {
		t.Fatal(err)
	}
}

func TestCabalReturns_EqualsTheCabalsBoardOneMonthRow(t *testing.T) {
	t.Parallel()
	s := newServer(t)
	at := s.clock.Now().UTC()
	acCapital, other := ids.Real{}.NewV7(), ids.Real{}.NewV7()
	run := sqlc.InsertLeaderboardRunParams{
		RunID: ids.Real{}.NewV7(), AsOf: at, PricesAsOf: at, StartedAt: at, FinishedAt: at,
	}
	if err := sqlc.New(s.pool).InsertLeaderboardRun(t.Context(), run); err != nil {
		t.Fatal(err)
	}
	loss, gain := int64(-410), int64(950)
	seedCabalReturn(t, s, at, "1M", 2, acCapital, &loss)
	seedCabalReturn(t, s, at, "1M", 1, other, &gain)
	seedCabalReturn(t, s, at, "ALL", 1, acCapital, &gain)

	got, err := ranking.New(module.Deps{Pool: s.pool}).CabalReturns().CabalReturns(
		t.Context(), port.Range1M, []ids.CabalID{ids.CabalIDFrom(acCapital)})
	if err != nil || len(got) != 1 {
		t.Fatalf("CabalReturns = %+v, %v, want one cabal", got, err)
	}
	ret := got[ids.CabalIDFrom(acCapital)]
	if ret.ReturnBps == nil || *ret.ReturnBps != loss || !ret.AsOf.Equal(at) {
		t.Fatalf("CabalReturns = %+v, want %d as of %s", ret, loss, at)
	}

	rec := s.get(t, "/v1/leaderboards/cabals?range=1M", ids.UserIDFrom(ids.Real{}.NewV7()))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET board = %d %s", rec.Code, rec.Body)
	}
	var boardBps *int64
	for _, row := range pageOf(t, rec).Rows {
		if row.Subject.Id == acCapital {
			boardBps = row.ReturnBps
		}
	}
	if boardBps == nil || *boardBps != *ret.ReturnBps {
		t.Fatalf("board 1M row return_bps = %v, CabalReturns = %v, want them equal; body %s",
			boardBps, *ret.ReturnBps, rec.Body)
	}
}

func TestCabalReturns_IsNilWithoutASnapshotAtTheRangeStart(t *testing.T) {
	t.Parallel()
	s := newServer(t)
	at := s.clock.Now().UTC()
	young, unknown := ids.Real{}.NewV7(), ids.Real{}.NewV7()
	seedCabalReturn(t, s, at, "1M", 1, young, nil)
	reader := ranking.New(module.Deps{Pool: s.pool}).CabalReturns()

	got, err := reader.CabalReturns(t.Context(), port.Range1M,
		[]ids.CabalID{ids.CabalIDFrom(young), ids.CabalIDFrom(unknown)})
	if err != nil {
		t.Fatal(err)
	}
	if ret, ok := got[ids.CabalIDFrom(young)]; !ok || ret.ReturnBps != nil {
		t.Errorf("young cabal = %+v, %v, want a row with no return", ret, ok)
	}
	if ret, ok := got[ids.CabalIDFrom(unknown)]; ok {
		t.Errorf("unranked cabal = %+v, want no row", ret)
	}
	none, err := reader.CabalReturns(t.Context(), port.Range1M, nil)
	if err != nil || len(none) != 0 {
		t.Errorf("CabalReturns(nil) = %v, %v, want empty", none, err)
	}
}
