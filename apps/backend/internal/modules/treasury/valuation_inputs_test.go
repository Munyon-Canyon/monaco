package treasury_test

import (
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

type jobRow struct {
	cabal   ids.CabalID
	status  string
	payout  int64
	created time.Time
	updated time.Time
}

func (j jobRow) insert(t *testing.T, f fixture) {
	t.Helper()
	if _, err := f.pool.Exec(t.Context(), `INSERT INTO cash_out_jobs
  (id, cabal_id, user_id, share_units, payout_micros, slice_micros, status, created_at, updated_at)
VALUES ($1, $2, $3, 10, $4, $4, $5, $6, $7)`,
		f.ids.NewV7(), j.cabal.UUID(), f.user(t).UUID(), j.payout, j.status, j.created, j.updated,
	); err != nil {
		t.Fatal(err)
	}
}

func TestValuationInputsAt_readsCashOutReservationsAsOfTheInstant(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	user, cabal := f.user(t), f.cabal(t)
	mustFundHistory(t, f, user, cabal, 1_000_000_000, 1_000_000_000)
	at := f.clock.Now()
	after := at.Add(time.Second)
	before := at.Add(-time.Second)
	for _, job := range []jobRow{
		{cabal, "started", 400_000_000, after, after},
		{cabal, "completed", 50_000_000, after, after.Add(time.Second)},
		{cabal, "paying", 3_000_000, before, before},
		{cabal, "completed", 7_000_000, before, after},
		{cabal, "failed", 11_000_000, before, before},
		{cabal, "partial", 13_000_000, before, at},
	} {
		job.insert(t, f)
	}
	got, err := newQueries(f).ValuationInputsAt(t.Context(), at)
	if err != nil {
		t.Fatal(err)
	}
	want := money.MicrosFromUint64(10_000_000)
	if got.Reserved[cabal] != want {
		t.Fatalf("Reserved = %v, want %v: only jobs begun by at and not settled by at reserve", got.Reserved, want)
	}
	if len(got.Positions) != 1 || len(got.Stakes) != 1 {
		t.Fatalf("ValuationInputsAt() = %#v, want the one funded cabal and stake", got)
	}
}

func TestValuationInputsAt_countsAnInTransitBridgeOnlyOnceItHasBegun(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	user, cabal := f.user(t), f.cabal(t)
	mustFundHistory(t, f, user, cabal, 1_000_000_000, 1_000_000_000)
	at := f.clock.Now()
	for _, row := range []struct {
		micros  int64
		created time.Time
	}{
		{25_000_000, at.Add(-time.Second)},
		{75_000_000, at},
		{900_000_000, at.Add(time.Second)},
	} {
		if _, err := f.pool.Exec(t.Context(), `INSERT INTO treasury_in_transit
  (cabal_id, trade_id, network, amount_micros, created_at) VALUES ($1, $2, 'base', $3, $4)`,
			cabal.UUID(), f.ids.NewV7(), row.micros, row.created,
		); err != nil {
			t.Fatal(err)
		}
	}
	got, err := newQueries(f).ValuationInputsAt(t.Context(), at)
	if err != nil || len(got.Positions) != 1 {
		t.Fatalf("ValuationInputsAt() = %#v, %v", got, err)
	}
	transit := got.Positions[0].InTransit
	if want := money.MicrosFromUint64(100_000_000); transit != want {
		t.Fatalf("InTransit = %v, want %v: a bridge begun after at is not in transit at at", transit, want)
	}
}

func TestValuationInputsAt_snapshotReadMatchesThePlainRead(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	user, cabal := f.user(t), f.cabal(t)
	mustFundHistory(t, f, user, cabal, 1_000_000_000, 1_000_000_000)
	at := f.clock.Now()
	jobRow{cabal, "selling", 40_000_000, at, at}.insert(t, f)
	plain, err := newQueries(f).ValuationInputsAt(t.Context(), at)
	if err != nil {
		t.Fatal(err)
	}
	snapped, err := newQueries(f).WithSnapshots(f.pool, time.Minute).ValuationInputsAt(t.Context(), at)
	reserved := snapped.Reserved[cabal]
	if err != nil || !sameCabalPositions(snapped.Positions, plain.Positions) ||
		!sameMemberStakes(snapped.Stakes, plain.Stakes) || reserved != money.MicrosFromUint64(40_000_000) {
		t.Fatalf("snapshot read = %#v, %v, want %#v", snapped, err, plain)
	}
}
