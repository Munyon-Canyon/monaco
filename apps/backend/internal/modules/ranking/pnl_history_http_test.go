package ranking_test

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/ranking/adapters"
	apibase "github.com/monaco/monaco/apps/backend/internal/platform/httpx/api"
	api "github.com/monaco/monaco/apps/backend/internal/platform/httpx/api/rankingapi"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

const (
	cabalShares = `SELECT coalesce(sum(share_units), 0)::bigint FROM user_positions WHERE cabal_id = $1`
	flatSeries  = `INSERT INTO cabal_value_snapshots (cabal_id, at, value_micros, nav_per_share_micros, total_shares)
SELECT $1, $2::timestamptz - (n * interval '1 hour'),
  CASE WHEN $2::timestamptz - (n * interval '1 hour') < $3 THEN $4::bigint ELSE $5::bigint END, 1,
  CASE WHEN $2::timestamptz - (n * interval '1 hour') < $3 THEN $6::bigint ELSE $7::bigint END
FROM generate_series(0, $8::int) AS n`
)

func sharesOf(t *testing.T, s server, cabal ids.CabalID) int64 {
	t.Helper()
	var n int64
	if err := s.pool.QueryRow(t.Context(), cabalShares, cabal.UUID()).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func pnlHistory(t *testing.T, s server, user ids.UserID, query string) api.MyPnlHistory {
	t.Helper()
	rec := s.get(t, "/v1/me/pnl-history"+query, user)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET pnl-history%s = %d %s", query, rec.Code, rec.Body)
	}
	var out api.MyPnlHistory
	decode(t, rec, &out)
	return out
}

func TestPnLHistory_MidRangeFundHasNoFakeGain(t *testing.T) {
	t.Parallel()
	s := newServer(t)
	cabal := testkit.NewCabal(t, s.pool)
	user := cabal.Creator.ID
	now := s.clock.Now().UTC()
	ledger := testkit.NewLedger(t, s.pool)
	ledger.WithFundedMember(user, cabal.ID, money.MicrosFromUint64(100_000_000))
	before := sharesOf(t, s, cabal.ID)
	ledger.WithFundedMember(user, cabal.ID, money.MicrosFromUint64(100_000_000))
	after := sharesOf(t, s, cabal.ID)
	secondFund := now.Add(-48*time.Hour + 30*time.Minute)
	exec(t, s.pool, moveFund, cabal.ID.UUID(), 0, now.Add(-6*24*time.Hour))
	exec(t, s.pool, moveFund, cabal.ID.UUID(), 1, secondFund)
	exec(t, s.pool, flatSeries, cabal.ID.UUID(), now, secondFund, 100_000_000, 200_000_000, before, after, 5*24)
	seedRun(t, s)
	got := pnlHistory(t, s, user, "?range=1W")
	steps := 0
	for i, p := range got.Points {
		if p.PnlMicros != 0 {
			t.Fatalf("point %d at %v pnl = %d, want 0 across the fund", i, p.At, p.PnlMicros)
		}
		if i > 0 && p.EquityMicros != got.Points[i-1].EquityMicros {
			steps++
		}
	}
	if len(got.Points) < 100 || steps != 1 || got.Points[0].EquityMicros != 100_000_000 ||
		got.Points[len(got.Points)-1].EquityMicros != 200_000_000 || got.Range != api.MyPnlHistoryRangeN1W {
		t.Fatalf("curve = %d points, %d steps, want one step from 100M to 200M", len(got.Points), steps)
	}
}

func TestPnLHistory_TwoCabalsSumAndAnEmptyCallerHasNoPoints(t *testing.T) {
	t.Parallel()
	s := newServer(t)
	first, second := testkit.NewCabal(t, s.pool), testkit.NewCabal(t, s.pool)
	user := first.Creator.ID
	now := s.clock.Now().UTC()
	ledger := testkit.NewLedger(t, s.pool)
	ledger.WithFundedMember(user, first.ID, money.MicrosFromUint64(100_000_000))
	ledger.WithFundedMember(user, second.ID, money.MicrosFromUint64(50_000_000))
	for cabal, value := range map[ids.CabalID]int64{first.ID: 120_000_000, second.ID: 40_000_000} {
		exec(t, s.pool, flatSeries, cabal.UUID(), now, now.Add(-time.Hour), value, value,
			sharesOf(t, s, cabal), sharesOf(t, s, cabal), 3)
	}
	exec(t, s.pool, `UPDATE user_txns SET created_at = $1 WHERE user_id = $2 AND kind = 'fund'`,
		now.Add(-3*time.Hour), user.UUID())
	seedRun(t, s)
	got := pnlHistory(t, s, user, "?range=1D")
	last := got.Points[len(got.Points)-1]
	if len(got.Points) == 0 || last.EquityMicros != 160_000_000 || last.PnlMicros != 10_000_000 {
		t.Fatalf("last point = %+v of %d, want equity 160M and P&L 10M across both cabals", last, len(got.Points))
	}
	stranger := ids.UserIDFrom(ids.Real{}.NewV7())
	if empty := pnlHistory(t, s, stranger, ""); empty.Points == nil || len(empty.Points) != 0 {
		t.Fatalf("a caller with no stake = %+v, want empty points", empty)
	}
}

func TestPnLHistory_EmptyBeforeTheFirstRunAndRejections(t *testing.T) {
	t.Parallel()
	s := newServer(t)
	user := ids.UserIDFrom(ids.Real{}.NewV7())
	if got := pnlHistory(t, s, user, "?range=1D"); got.Points == nil || len(got.Points) != 0 {
		t.Fatalf("before the first run = %+v, want empty points", got)
	}
	if rec := s.get(t, "/v1/me/pnl-history?range=2Y", user); rec.Code != http.StatusBadRequest ||
		problemOf(t, rec) != apibase.InvalidInput {
		t.Fatalf("bad range = %d %s, want 400 invalid_input", rec.Code, rec.Body)
	}
	if rec := s.get(t, "/v1/me/pnl-history", ids.UserID{}); rec.Code != http.StatusUnauthorized {
		t.Fatalf("without a token = %d, want 401", rec.Code)
	}
}

func TestPnLHistory_IssuesOneStakeReadAndOneSnapshotRead(t *testing.T) {
	t.Parallel()
	s := newServer(t)
	cabal := testkit.NewCabal(t, s.pool)
	now := s.clock.Now().UTC()
	testkit.NewLedger(t, s.pool).WithFundedMember(cabal.Creator.ID, cabal.ID, money.MicrosFromUint64(10_000_000))
	exec(t, s.pool, flatSeries, cabal.ID.UUID(), now, now, 10_000_000, 10_000_000,
		sharesOf(t, s, cabal.ID), sharesOf(t, s, cabal.ID), 48)
	seedRun(t, s)
	testkit.AssertQueries(t, "pnl history", func() { pnlHistory(t, s, cabal.Creator.ID, "?range=1D") })
}

func TestPnLHistory_SnapshotsOfCabalsFailures(t *testing.T) {
	t.Parallel()
	s := newServer(t)
	cabal := ids.CabalIDFrom(ids.Real{}.NewV7())
	now := s.clock.Now().UTC()
	boards := adapters.Boards{DB: s.pool}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := boards.SnapshotsOfCabals(
		ctx,
		[]ids.CabalID{cabal},
		now.Add(-time.Hour),
		now,
	); errs.CodeOf(
		err,
	) != errs.CodeInternal {
		t.Fatalf("canceled read: err = %v, want internal", err)
	}
	exec(t, s.pool, `INSERT INTO cabal_value_snapshots VALUES ($1, $2, 1, 1, -1)`, cabal.UUID(), now)
	if _, err := boards.SnapshotsOfCabals(
		t.Context(),
		[]ids.CabalID{cabal},
		now.Add(-time.Hour),
		now,
	); errs.CodeOf(
		err,
	) != errs.CodeInternal {
		t.Fatalf("negative shares: err = %v, want internal", err)
	}
}
