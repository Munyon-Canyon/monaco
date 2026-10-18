package ranking

import (
	"context"
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/modules/market"
)

func TestMarketPort_forwardsEveryRead(t *testing.T) {
	t.Parallel()
	fake := &marketPortFake{}
	port := marketPort{catalog: fake, prices: fake, listings: fake}
	ctx := t.Context()
	if _, err := port.AssetsByMints(ctx, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := port.PricesAsOf(ctx, nil, valuationTime()); err != nil {
		t.Fatal(err)
	}
	if _, err := port.ListingPrices(ctx, nil, valuationTime()); err != nil {
		t.Fatal(err)
	}
	if fake.calls != 3 {
		t.Fatalf("calls = %d, want 3", fake.calls)
	}
}

func valuationTime() time.Time { return time.Date(2026, time.October, 4, 12, 0, 0, 0, time.UTC) }

type marketPortFake struct{ calls int }

func (*marketPortFake) AssetByID(context.Context, market.AssetID) (market.Asset, error) {
	return market.Asset{}, nil
}

func (*marketPortFake) AssetByMint(context.Context, market.Mint) (market.Asset, error) {
	return market.Asset{}, nil
}

func (*marketPortFake) AssetBySymbol(context.Context, string) (market.Asset, error) {
	return market.Asset{}, nil
}

func (*marketPortFake) ListTradable(context.Context) ([]market.Asset, error) {
	return []market.Asset{}, nil
}

func (*marketPortFake) ListAll(context.Context) ([]market.Asset, error) {
	return []market.Asset{}, nil
}

func (f *marketPortFake) ByMints(context.Context, []string) ([]market.Asset, error) {
	f.calls++
	return []market.Asset{}, nil
}

func (f *marketPortFake) LatestPrices(context.Context) (map[market.AssetID]market.Price, error) {
	f.calls++
	return map[market.AssetID]market.Price{}, nil
}

func (f *marketPortFake) PricesAsOf(
	context.Context,
	[]market.AssetID,
	time.Time,
) (map[market.AssetID]market.Price, error) {
	f.calls++
	return map[market.AssetID]market.Price{}, nil
}

func (f *marketPortFake) ListingPrices(
	context.Context, []market.ListingKey, time.Time,
) (map[market.ListingKey]market.ListingPrice, error) {
	f.calls++
	return map[market.ListingKey]market.ListingPrice{}, nil
}
