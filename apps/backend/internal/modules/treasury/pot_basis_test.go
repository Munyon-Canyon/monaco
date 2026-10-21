package treasury_test

import (
	"context"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/adapters"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
)

const insertUSDCPosition = `INSERT INTO cabal_positions (cabal_id, asset, units, cost_basis_micros, updated_at)
	VALUES ($1, $2, 1, 1, $3)`

func TestPotBasis_changesOnlyWhenWhatTheCabalHoldsChanges(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	cabal := f.cabal(t)
	usdc := chain.SolanaAddress(f.cfg.Solana.USDCMint)
	q := adapters.NewQueries(f.pool, nil, nil, f.clock, usdc)
	first, err := q.PotBasis(t.Context(), cabal)
	again, againErr := q.PotBasis(t.Context(), cabal)
	if err != nil || againErr != nil || first == "" || again != first {
		t.Fatalf("PotBasis twice = %q, %v then %q, %v, want the same basis", first, err, again, againErr)
	}
	if _, err := f.pool.Exec(t.Context(), insertUSDCPosition, cabal.UUID(), usdc, f.clock.Now()); err != nil {
		t.Fatal(err)
	}
	moved, err := q.PotBasis(t.Context(), cabal)
	if err != nil || moved == first {
		t.Fatalf("PotBasis after a new position = %q, %v, want a basis other than %q", moved, err, first)
	}
	if _, err := q.PotBasis(canceled(t.Context()), cabal); err == nil {
		t.Fatal("PotBasis on a canceled context = nil error")
	}
}

func TestPotBasis_readsThroughTheTransactionItIsGiven(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	cabal := f.cabal(t)
	usdc := chain.SolanaAddress(f.cfg.Solana.USDCMint)
	q := adapters.NewQueries(f.pool, nil, nil, f.clock, usdc)
	err := f.uow.Do(f.ctx(), func(ctx context.Context, tx db.Tx) error {
		if _, err := tx.Queries().Exec(ctx, insertUSDCPosition, cabal.UUID(), usdc, f.clock.Now()); err != nil {
			return err
		}
		outside, outsideErr := q.PotBasis(ctx, cabal)
		inside, insideErr := q.On(tx.Queries()).PotBasis(ctx, cabal)
		if outsideErr != nil || insideErr != nil || outside == inside {
			t.Errorf("PotBasis outside = %q, %v inside = %q, %v, want the transaction to see its own insert",
				outside, outsideErr, inside, insideErr)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
