package treasury_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/adapters"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
	"github.com/monaco/monaco/apps/backend/internal/testkit/marketfake"
)

func TestPotValue_USDCOnly(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	user, cabal := f.user(t), f.cabal(t)
	u, c, err := f.fund(user, cabal, 100_000_000, 100, domain.TxnSettled)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.postPair(u, c); err != nil {
		t.Fatal(err)
	}
	q := newQueries(f)
	got, err := q.PotValue(t.Context(), cabal)
	if err != nil || got != money.MicrosFromUint64(100_000_000) {
		t.Fatalf("PotValue() = %v, %v", got, err)
	}
}

func TestPotValue_WithHoldings(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	user, cabal := f.user(t), f.cabal(t)
	u, c, err := f.fund(user, cabal, 100_000_000, 100, domain.TxnSettled)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.postPair(u, c); err != nil {
		t.Fatal(err)
	}
	swap, err := f.swap(cabal, 40_000_000, 200_000_000)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.postCabal(swap); err != nil {
		t.Fatal(err)
	}
	prices := &marketfake.PricesFake{}
	prices.Set(marketfake.AAPLx().ID, money.MicrosFromUint64(30_000_000), f.clock.Now())
	q := adapterQueries(f, prices)
	got, err := q.PotValue(t.Context(), cabal)
	if err != nil || got != money.MicrosFromUint64(120_000_000) {
		t.Fatalf("PotValue() = %v, %v", got, err)
	}
	positions, err := q.Positions(t.Context(), cabal)
	if err != nil || len(positions) != 2 || string(positions[0].Mint) != usdcMint ||
		positions[1].Units.Decimals() != 8 {
		t.Fatalf("Positions() = %#v, %v", positions, err)
	}
}

func TestPotValue_StalePriceUnavailable(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	cabal := seedHolding(t, f)
	prices := &marketfake.PricesFake{}
	prices.Set(
		marketfake.AAPLx().ID, money.MicrosFromUint64(30_000_000), f.clock.Now().Add(-5*time.Minute-time.Nanosecond),
	)
	q := adapterQueries(f, prices)
	_, err := q.PotValue(t.Context(), cabal)
	wantCode(t, err, errs.CodePriceUnavailable)
}

func TestPotValue_ExactPriceBoundaryFresh(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	cabal := seedHolding(t, f)
	prices := &marketfake.PricesFake{}
	prices.Set(marketfake.AAPLx().ID, money.MicrosFromUint64(30_000_000), f.clock.Now().Add(-5*time.Minute))
	got, err := adapterQueries(f, prices).PotValue(t.Context(), cabal)
	if err != nil || got != money.MicrosFromUint64(120_000_000) {
		t.Fatalf("PotValue() = %v, %v", got, err)
	}
}

func TestPotValue_MissingPriceUnavailable(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	cabal := seedHolding(t, f)
	q := adapterQueries(f, &marketfake.PricesFake{})
	_, err := q.PotValue(t.Context(), cabal)
	wantCode(t, err, errs.CodePriceUnavailable)
}

func TestPotValue_EmptyCabalZero(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	got, err := newQueries(f).PotValue(t.Context(), f.cabal(t))
	if err != nil || !got.IsZero() {
		t.Fatalf("PotValue() = %v, %v", got, err)
	}
}

func TestPotValue_ChecksHoldingsBeforePrices(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	reader := app.PriceReader(func(context.Context) (map[uuid.UUID]app.Price, error) {
		return nil, errs.New(errs.CodeDBUnavailable, "test")
	})
	q := adapterQueriesWithReader(f, reader)
	if got, err := q.PotValue(t.Context(), f.cabal(t)); err != nil || !got.IsZero() {
		t.Fatalf("empty PotValue() = %v, %v", got, err)
	}
	usdcOnly := seedUSDCOnly(t, f)
	got, err := q.PotValue(t.Context(), usdcOnly)
	if err != nil || got != money.MicrosFromUint64(100_000_000) {
		t.Fatalf("USDC-only PotValue() = %v, %v", got, err)
	}
}

func TestPotValue_QueryCount(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	cabal := seedHolding(t, f)
	prices := &marketfake.PricesFake{}
	for _, asset := range marketfake.Fixtures() {
		prices.Set(asset.ID, money.MicrosFromUint64(30_000_000), f.clock.Now())
	}
	reads := 0
	reader := app.PriceReader(func(ctx context.Context) (map[uuid.UUID]app.Price, error) {
		reads++
		latest, err := prices.LatestPrices(ctx)
		if err != nil {
			return nil, err
		}
		out := make(map[uuid.UUID]app.Price, len(latest))
		for id, price := range latest {
			out[id.UUID()] = app.Price{Micros: price.Micros, ObservedAt: price.ObservedAt}
		}
		return out, nil
	})
	q := adapterQueriesWithReader(f, reader)
	assertPotValueQueries(t, q, cabal, &reads)
	const insert = `INSERT INTO cabal_positions (cabal_id, asset, units, cost_basis_micros, updated_at)
VALUES ($1, $2, 1, 1, $3), ($1, $4, 1, 1, $3)`
	tsla, jpst := marketfake.TSLAx().Mint.Address(), marketfake.JPSTx().Mint.Address()
	_, err := f.pool.Exec(
		t.Context(), insert, cabal.UUID(), tsla, f.clock.Now(), jpst,
	)
	if err != nil {
		t.Fatal(err)
	}
	assertPotValueQueries(t, q, cabal, &reads)
}

func assertPotValueQueries(t *testing.T, q *adapters.Queries, cabal ids.CabalID, reads *int) {
	t.Helper()
	*reads = 0
	testkit.AssertQueries(t, "PotValue one and three holdings", func() {
		if _, err := q.PotValue(t.Context(), cabal); err != nil {
			t.Fatal(err)
		}
	})
	if *reads != 1 {
		t.Fatalf("price reads = %d, want 1", *reads)
	}
}

func TestPositions_InvalidStoredMintFailsDecode(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	cabal := f.cabal(t)
	const insert = `INSERT INTO cabal_positions (cabal_id, asset, units, cost_basis_micros, updated_at)
VALUES ($1, 'not-a-mint', 1, 0, $2)`
	_, err := f.pool.Exec(t.Context(), insert, cabal.UUID(), f.clock.Now())
	if err != nil {
		t.Fatal(err)
	}
	_, err = newQueries(f).Positions(t.Context(), cabal)
	wantCode(t, err, errs.CodeDecodeFailed)
}

func TestQueries_CanceledReadsFail(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	q := newQueries(f)
	cabal := f.cabal(t)
	for _, call := range []func() error{
		func() error { _, err := q.Positions(ctx, cabal); return err },
		func() error { _, err := q.PotValue(ctx, cabal); return err },
		func() error { _, err := q.TotalShares(ctx, cabal); return err },
	} {
		if call() == nil {
			t.Fatal("error = nil")
		}
	}
}

func seedHolding(t *testing.T, f fixture) ids.CabalID {
	t.Helper()
	user, cabal := f.user(t), f.cabal(t)
	u, c, err := f.fund(user, cabal, 100_000_000, 100, domain.TxnSettled)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.postPair(u, c); err != nil {
		t.Fatal(err)
	}
	swap, err := f.swap(cabal, 40_000_000, 200_000_000)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.postCabal(swap); err != nil {
		t.Fatal(err)
	}
	return cabal
}

func seedUSDCOnly(t *testing.T, f fixture) ids.CabalID {
	t.Helper()
	user, cabal := f.user(t), f.cabal(t)
	u, c, err := f.fund(user, cabal, 100_000_000, 100, domain.TxnSettled)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.postPair(u, c); err != nil {
		t.Fatal(err)
	}
	return cabal
}

func newQueries(f fixture) *adapters.Queries {
	return adapterQueries(f, &marketfake.PricesFake{})
}

func adapterQueries(f fixture, prices *marketfake.PricesFake) *adapters.Queries {
	return adapterQueriesWithReader(f, priceReader(prices))
}

func adapterQueriesWithReader(f fixture, reader app.PriceReader) *adapters.Queries {
	resolver := app.MintResolver(func(_ context.Context, mint chain.SolanaAddress) (app.Asset, error) {
		for _, asset := range marketfake.Fixtures() {
			if string(mint) == string(asset.Mint.Address()) {
				return app.Asset{ID: asset.ID.UUID(), Decimals: asset.Decimals, ChainChecked: asset.ChainChecked}, nil
			}
		}
		return app.Asset{}, errs.New(errs.CodeAssetNotFound, "test.AssetByMint")
	})
	return adapters.NewQueries(f.pool, resolver, reader, f.clock, usdcMint)
}

func priceReader(prices *marketfake.PricesFake) app.PriceReader {
	return app.PriceReader(func(ctx context.Context) (map[uuid.UUID]app.Price, error) {
		latest, err := prices.LatestPrices(ctx)
		if err != nil {
			return nil, err
		}
		out := make(map[uuid.UUID]app.Price, len(latest))
		for id, price := range latest {
			out[id.UUID()] = app.Price{Micros: price.Micros, ObservedAt: price.ObservedAt}
		}
		return out, nil
	})
}
