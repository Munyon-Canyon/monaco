package treasury_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/market"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/adapters"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/app"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
	"github.com/monaco/monaco/apps/backend/internal/testkit/marketfake"
)

func currentPriceReader(prices *marketfake.PricesFake) app.PriceReader {
	return app.PriceReader(func(ctx context.Context, ids []uuid.UUID, at time.Time) (map[uuid.UUID]app.Price, error) {
		asked := make([]market.AssetID, len(ids))
		for i, id := range ids {
			asked[i] = market.AssetIDFrom(id)
		}
		current, err := prices.CurrentPricesAsOf(ctx, asked, at)
		if err != nil {
			return nil, err
		}
		out := make(map[uuid.UUID]app.Price, len(current))
		for id, price := range current {
			out[id.UUID()] = app.Price{Micros: price.Micros, ObservedAt: price.ObservedAt}
		}
		return out, nil
	})
}

func queriesWithAsset(f fixture, prices *marketfake.PricesFake, edit func(*app.Asset)) *adapters.Queries {
	resolver := app.MintResolver(func(_ context.Context, mint chain.SolanaAddress) (app.Asset, error) {
		for _, asset := range marketfake.Fixtures() {
			if string(mint) == string(asset.Mint.Address()) {
				got := app.Asset{
					ID: asset.ID.UUID(), Symbol: asset.Symbol, DisplayName: asset.DisplayName, Kind: string(asset.Kind),
					Decimals: asset.Decimals, ChainChecked: asset.ChainChecked,
				}
				edit(&got)
				return got, nil
			}
		}
		return app.Asset{}, errs.New(errs.CodeAssetNotFound, "test.AssetByMint")
	})
	return adapters.NewQueries(f.pool, resolver, priceReader(prices), f.clock, usdcMint).
		WithFreshPrices(currentPriceReader(prices))
}

func splitAt(at time.Time) func(*app.Asset) {
	return func(a *app.Asset) {
		a.UIMultiplierNum, a.UIMultiplierDen = 1, 1
		a.NextUIMultiplierNum, a.NextUIMultiplierDen, a.NextUIMultiplierAt = 5, 1, at
	}
}

func TestPotValue_aSampleFromBeforeASplitInEffectIsUnavailable(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	cabal := seedHolding(t, f)
	at := f.clock.Now().Add(-time.Minute)
	prices := &marketfake.PricesFake{}
	prices.Set(marketfake.AAPLx().ID, money.MicrosFromUint64(30_000_000), at.Add(-time.Second))
	q := queriesWithAsset(f, prices, splitAt(at))
	_, err := q.PotValue(t.Context(), cabal)
	wantCode(t, err, errs.CodePriceUnavailable)
	_, err = q.Strict().PotValue(t.Context(), cabal)
	wantCode(t, err, errs.CodePriceUnavailable)

	prices.Set(marketfake.AAPLx().ID, money.MicrosFromUint64(6_000_000), at)
	got, err := q.PotValue(t.Context(), cabal)
	if err != nil || got != money.MicrosFromUint64(120_000_000) {
		t.Fatalf("PotValue after the first post-split sample = %v, %v, want 60 cash + 200 units x 6 x 5", got, err)
	}
}

func TestPotValue_aSplitStillAheadDoesNotHoldBackTheCurrentSample(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	cabal := seedHolding(t, f)
	prices := &marketfake.PricesFake{}
	prices.Set(marketfake.AAPLx().ID, money.MicrosFromUint64(30_000_000), f.clock.Now().Add(-time.Minute))
	q := queriesWithAsset(f, prices, splitAt(f.clock.Now().Add(time.Second)))
	got, err := q.PotValue(t.Context(), cabal)
	if err != nil || got != money.MicrosFromUint64(120_000_000) {
		t.Fatalf("PotValue before the split = %v, %v", got, err)
	}
}

func heldBackDrop(f fixture) *marketfake.PricesFake {
	prices := &marketfake.PricesFake{}
	id, now := marketfake.AAPLx().ID, f.clock.Now()
	prices.Set(id, money.MicrosFromUint64(30_000_000), now.Add(-4*time.Minute))
	prices.Set(id, money.MicrosFromUint64(30_000_000), now.Add(-2*time.Minute))
	prices.Set(id, money.MicrosFromUint64(18_000_000), now)
	return prices
}

func TestStrictPotValue_refusesToPriceAgainstAHeldBackDrop(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	cabal := seedHolding(t, f)
	prices := heldBackDrop(f)
	q := queriesWithAsset(f, prices, func(*app.Asset) {})

	_, err := q.Strict().PotValue(t.Context(), cabal)
	wantCode(t, err, errs.CodePriceUnavailable)

	prices.Set(marketfake.AAPLx().ID, money.MicrosFromUint64(18_000_000), f.clock.Now().Add(time.Minute))
	f.clock.Advance(time.Minute)
	got, err := q.Strict().PotValue(t.Context(), cabal)
	if err != nil || got != money.MicrosFromUint64(96_000_000) {
		t.Fatalf("Strict PotValue after the drop is confirmed = %v, %v, want 60 cash + 36", got, err)
	}
}

func TestCashOut_aHeldBackDropBlocksThePayoutInsteadOfPayingTheOldPrice(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	user, cabal := f.user(t), f.cabal(t)
	cashOutFund(t, f, user, cabal, 100_000_000)
	swap, err := f.swap(cabal, 40_000_000, 200_000_000)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.postCabal(swap); err != nil {
		t.Fatal(err)
	}
	prices := heldBackDrop(f)
	strict := queriesWithAsset(f, prices, func(*app.Asset) {}).Strict()
	h := cashOutHandlerWith(f, cashOutPauses{}, strict)
	ctx := observability.WithActor(f.ctx(), "user:"+user.String())

	_, err = h.Handle(ctx, app.CashOut{CabalID: cabal, UserID: user, All: true})
	wantCode(t, err, errs.CodePriceUnavailable)
	_, err = h.Preview(ctx, cabal, user)
	wantCode(t, err, errs.CodePriceUnavailable)
	if f.count(t, "cash_out_jobs") != 0 || f.positionUnits(t, cabal, user) != 100 {
		t.Fatal("a refused cash out reserved a payout or burned shares")
	}

	prices.Set(marketfake.AAPLx().ID, money.MicrosFromUint64(18_000_000), f.clock.Now().Add(time.Minute))
	f.clock.Advance(time.Minute)
	result, err := h.Handle(ctx, app.CashOut{CabalID: cabal, UserID: user, All: true})
	if err != nil || result.PayoutMicros != money.MicrosFromUint64(96_000_000) {
		t.Fatalf("CashOut once the drop is confirmed = %#v, %v, want a 96 USDC payout at the fresh price", result, err)
	}
}

func TestStrictQueries_keepTheirFreshReaderAcrossOn(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	cabal := seedHolding(t, f)
	strict := queriesWithAsset(f, heldBackDrop(f), func(*app.Asset) {}).Strict()
	_, err := strict.On(f.pool).PotValue(t.Context(), cabal)
	wantCode(t, err, errs.CodePriceUnavailable)
}
