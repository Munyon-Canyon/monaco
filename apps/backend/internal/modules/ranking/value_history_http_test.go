package ranking_test

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/ranking/adapters"
	apibase "github.com/monaco/monaco/apps/backend/internal/platform/httpx/api"
	api "github.com/monaco/monaco/apps/backend/internal/platform/httpx/api/rankingapi"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

const (
	hourlySnapshots = `INSERT INTO cabal_value_snapshots (cabal_id, at, value_micros, nav_per_share_micros, total_shares)
SELECT $1, $2::timestamptz - (n * interval '1 hour'), $3::bigint + (($4::int - n) * 1000), 1000, 100
FROM generate_series(0, $4::int) AS n`
	stepSnapshots = `INSERT INTO cabal_value_snapshots (cabal_id, at, value_micros, nav_per_share_micros, total_shares)
SELECT $1, $2::timestamptz - (n * interval '1 hour'), CASE WHEN $2::timestamptz - (n * interval '1 hour') < $3 THEN 100000000
  ELSE 200000000 END, 1000000, 100
FROM generate_series(0, $4::int) AS n`
	moveFund = `UPDATE user_txns SET created_at = $3 WHERE id = (SELECT id FROM user_txns
WHERE cabal_id = $1 AND kind = 'fund' ORDER BY created_at, id OFFSET $2 LIMIT 1)`
)

func valueHistory(t *testing.T, s server, cabal ids.CabalID, query string) api.CabalValueHistory {
	t.Helper()
	rec := s.get(t, "/v1/cabals/"+cabal.String()+"/value-history"+query, ids.UserIDFrom(ids.Real{}.NewV7()))
	var out api.CabalValueHistory
	if rec.Code != http.StatusOK {
		t.Fatalf("GET value-history%s = %d %s", query, rec.Code, rec.Body)
	}
	decode(t, rec, &out)
	return out
}

func seedRun(t *testing.T, s server) {
	t.Helper()
	seedBoard(t, s.pool, s.clock.Now().UTC(), boardRows("cabals", 1))
}

func exec(t *testing.T, pool *pgxpool.Pool, query string, args ...any) {
	t.Helper()
	if _, err := pool.Exec(t.Context(), query, args...); err != nil {
		t.Fatal(err)
	}
}

func TestValueHistory_BucketsPerRange(t *testing.T) {
	t.Parallel()
	s := newServer(t)
	cabal := testkit.NewCabal(t, s.pool)
	now := s.clock.Now().UTC()
	exec(t, s.pool, hourlySnapshots, cabal.ID.UUID(), now, 1_000_000, 8*24)
	seedRun(t, s)
	for query, want := range map[string]int{"?range=1H": 31, "?range=1D": 145, "?range=1W": 169, "?range=1M": 49, "": 9} {
		got := valueHistory(t, s, cabal.ID, query)
		if len(got.Points) != want || !got.Points[len(got.Points)-1].At.Equal(now) ||
			got.Points[len(got.Points)-1].ValueMicros != 1_000_000+8*24*1000 || got.PricesAsOf == nil ||
			got.CabalId != cabal.ID.UUID() {
			t.Fatalf(
				"GET%s = %d points, last %+v, want %d ending at now",
				query,
				len(got.Points),
				got.Points[len(got.Points)-1],
				want,
			)
		}
		for i := 1; i < len(got.Points); i++ {
			if !got.Points[i].At.After(got.Points[i-1].At) {
				t.Fatalf("GET%s points out of order at %d", query, i)
			}
		}
	}
	if got := valueHistory(t, s, cabal.ID, "?range=1D"); got.Range != api.CabalValueHistoryRangeN1D ||
		got.Points[0].PnlMicros != got.Points[0].ValueMicros {
		t.Fatalf("1D = %+v, want range 1D and P&L equal to value with no contributions", got.Points[0])
	}
}

func TestValueHistory_AFundMidRangeIsNotAGain(t *testing.T) {
	t.Parallel()
	s := newServer(t)
	cabal := testkit.NewCabal(t, s.pool)
	now := s.clock.Now().UTC()
	secondFund := now.Add(-48*time.Hour + 30*time.Minute)
	ledger := testkit.NewLedger(t, s.pool)
	ledger.WithFundedMember(cabal.Creator.ID, cabal.ID, money.MicrosFromUint64(100_000_000))
	ledger.WithFundedMember(cabal.Creator.ID, cabal.ID, money.MicrosFromUint64(100_000_000))
	exec(t, s.pool, moveFund, cabal.ID.UUID(), 0, now.Add(-6*24*time.Hour))
	exec(t, s.pool, moveFund, cabal.ID.UUID(), 1, secondFund)
	exec(t, s.pool, stepSnapshots, cabal.ID.UUID(), now, secondFund, 5*24)
	seedRun(t, s)
	got := valueHistory(t, s, cabal.ID, "?range=1W")
	steps := 0
	for i, p := range got.Points {
		if p.PnlMicros != 0 {
			t.Fatalf("point %d at %v pnl = %d, want 0 across the fund", i, p.At, p.PnlMicros)
		}
		if i > 0 && p.ValueMicros != got.Points[i-1].ValueMicros {
			steps++
		}
	}
	if steps != 1 || got.Points[0].ValueMicros != 100_000_000 ||
		got.Points[len(got.Points)-1].ValueMicros != 200_000_000 {
		t.Fatalf("curve has %d steps from %d to %d, want one step from 100M to 200M",
			steps, got.Points[0].ValueMicros, got.Points[len(got.Points)-1].ValueMicros)
	}
}

func TestValueHistory_EmptyBeforeTheFirstRunAndForAnUnsnappedCabal(t *testing.T) {
	t.Parallel()
	s := newServer(t)
	cabal := testkit.NewCabal(t, s.pool)
	if got := valueHistory(t, s, cabal.ID, ""); got.Points == nil || len(got.Points) != 0 || got.PricesAsOf != nil {
		t.Fatalf("before the first run = %+v, want empty points and a null prices_as_of", got)
	}
	seedRun(t, s)
	if got := valueHistory(
		t,
		s,
		cabal.ID,
		"?range=1D",
	); got.Points == nil || len(got.Points) != 0 ||
		got.PricesAsOf == nil {
		t.Fatalf("with no snapshots = %+v, want empty points", got)
	}
}

func TestValueHistory_Rejections(t *testing.T) {
	t.Parallel()
	s := newServer(t)
	cabal := testkit.NewCabal(t, s.pool)
	viewer := ids.UserIDFrom(ids.Real{}.NewV7())
	path := "/v1/cabals/" + cabal.ID.String() + "/value-history"
	if rec := s.get(
		t,
		path+"?range=2Y",
		viewer,
	); rec.Code != http.StatusBadRequest ||
		problemOf(t, rec) != apibase.InvalidInput {
		t.Fatalf("bad range = %d %s, want 400 invalid_input", rec.Code, rec.Body)
	}
	if rec := s.get(t, path, ids.UserID{}); rec.Code != http.StatusUnauthorized {
		t.Fatalf("without a token = %d, want 401", rec.Code)
	}
	unknown := "/v1/cabals/" + ids.Real{}.NewV7().String() + "/value-history"
	for name, setup := range map[string]func(){"before a run": func() {}, "after a run": func() { seedRun(t, s) }} {
		setup()
		if rec := s.get(
			t,
			unknown,
			viewer,
		); rec.Code != http.StatusNotFound ||
			problemOf(t, rec) != apibase.CabalNotFound {
			t.Fatalf("unknown cabal %s = %d %s, want 404 cabal_not_found", name, rec.Code, rec.Body)
		}
	}
}

func TestValueHistory_AHitIssuesOneQuery(t *testing.T) {
	t.Parallel()
	s := newServer(t)
	cabal := testkit.NewCabal(t, s.pool)
	exec(t, s.pool, hourlySnapshots, cabal.ID.UUID(), s.clock.Now().UTC(), 1_000_000, 48)
	seedRun(t, s)
	testkit.AssertQueries(t, "value history miss", func() { valueHistory(t, s, cabal.ID, "?range=1D") })
	testkit.AssertQueries(t, "value history hit", func() { valueHistory(t, s, cabal.ID, "?range=1D") })
	testkit.AssertQueries(t, "value history other range miss", func() { valueHistory(t, s, cabal.ID, "?range=1W") })
}

func TestValueHistory_SnapshotReadFailures(t *testing.T) {
	t.Parallel()
	s := newServer(t)
	cabal := ids.CabalIDFrom(ids.Real{}.NewV7())
	boards := adapters.Boards{DB: s.pool}
	now := s.clock.Now().UTC()
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := boards.SnapshotsSince(ctx, cabal, now.Add(-time.Hour), now); errs.CodeOf(err) != errs.CodeInternal {
		t.Fatalf("canceled read: err = %v, want internal", err)
	}
	exec(t, s.pool, `INSERT INTO cabal_value_snapshots VALUES ($1, $2, -1, 1, 1)`, cabal.UUID(), now)
	if _, err := boards.SnapshotsSince(
		t.Context(),
		cabal,
		now.Add(-time.Hour),
		now,
	); errs.CodeOf(
		err,
	) != errs.CodeInternal {
		t.Fatalf("negative value: err = %v, want internal", err)
	}
}
