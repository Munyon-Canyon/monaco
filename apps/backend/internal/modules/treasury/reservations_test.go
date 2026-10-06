package treasury_test

import (
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

func TestCashOutReservations_sumsLiveJobsPerCabalAndMatchesPotValue(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	user, cabal, idle := f.user(t), f.cabal(t), f.cabal(t)
	u, c, err := f.fund(user, cabal, 100_000_000, 100, domain.TxnSettled)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.postPair(u, c); err != nil {
		t.Fatal(err)
	}
	insert := `INSERT INTO cash_out_jobs
  (id, cabal_id, user_id, share_units, payout_micros, slice_micros, status, created_at, updated_at)
VALUES ($1, $2, $3, 10, 30000000, 30000000, $4, $5, $5)`
	for _, status := range []string{"started", "selling", "completed", "failed"} {
		if _, err := f.pool.Exec(
			t.Context(), insert, f.ids.NewV7(), cabal.UUID(), f.user(t).UUID(), status, f.clock.Now(),
		); err != nil {
			t.Fatal(err)
		}
	}
	q := newQueries(f)
	reserved, err := q.CashOutReservations(t.Context())
	if err != nil || len(reserved) != 1 || reserved[cabal] != money.MicrosFromUint64(60_000_000) {
		t.Fatalf("CashOutReservations() = %v, %v, want 60 USDC for the one cabal with live jobs", reserved, err)
	}
	if _, idleReserved := reserved[idle]; idleReserved {
		t.Fatal("a cabal with no live job is reserved")
	}
	pot, err := q.PotValue(t.Context(), cabal)
	if err != nil || pot != money.MicrosFromUint64(40_000_000) {
		t.Fatalf("PotValue() = %v, %v, want the funded 100 USDC less the reservation", pot, err)
	}
}
