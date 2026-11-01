package treasury_test

import (
	"context"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/domain"
)

func TestHoldsLiveAssets_namesACabalWhoseShareCountCannotPriceAFund(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	user, cabal := f.user(t), f.cabal(t)
	q := newQueries(f)
	holds := func(step string, want bool) {
		t.Helper()
		if got, err := q.HoldsLiveAssets(t.Context(), cabal); err != nil || got != want {
			t.Fatalf("%s: HoldsLiveAssets = %v, %v, want %v", step, got, err, want)
		}
	}
	holds("a new cabal", false)

	u, c, err := f.fund(user, cabal, 100_000_000, 100, domain.TxnSettled)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.postPair(u, c); err != nil {
		t.Fatal(err)
	}
	holds("a cabal that holds only cash", false)

	job := f.ids.NewV7()
	if _, err := f.pool.Exec(t.Context(), `INSERT INTO cash_out_jobs
		(id, cabal_id, user_id, share_units, payout_micros, slice_micros, status, created_at, updated_at)
		VALUES ($1, $2, $3, 10, 30000000, 30000000, 'completed', $4, $4)`,
		job, cabal.UUID(), user.UUID(), f.clock.Now()); err != nil {
		t.Fatal(err)
	}
	holds("a cash-out that finished", false)
	for _, status := range []string{"started", "selling", "paying"} {
		if _, err := f.pool.Exec(t.Context(),
			`UPDATE cash_out_jobs SET status = $2 WHERE id = $1`, job, status); err != nil {
			t.Fatal(err)
		}
		holds("a "+status+" cash-out", true)
	}
	if _, err := f.pool.Exec(t.Context(), `UPDATE cash_out_jobs SET status = 'failed' WHERE id = $1`, job); err != nil {
		t.Fatal(err)
	}
	holds("a cash-out that failed", false)

	if _, err := f.pool.Exec(t.Context(), `INSERT INTO cabal_positions
		(cabal_id, asset, units, cost_basis_micros, updated_at) VALUES ($1, 'MintHeld', 5, 0, $2)`,
		cabal.UUID(), f.clock.Now()); err != nil {
		t.Fatal(err)
	}
	holds("a cabal that holds a token", true)
}

func TestHoldsLiveAssets_surfacesAReadFailure(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := newQueries(f).HoldsLiveAssets(ctx, f.cabal(t)); err == nil {
		t.Fatal("HoldsLiveAssets with a cancelled context returned no error")
	}
}
