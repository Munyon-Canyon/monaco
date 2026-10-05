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
	port := marketPort{catalog: fake, prices: fake, calendar: fake}
	ctx := t.Context()
	if _, err := port.ListAll(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := port.LatestPrices(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := port.PricesAsOf(ctx, nil, valuationTime()); err != nil {
		t.Fatal(err)
	}
	if _, err := port.Session(ctx, market.AssetID{}, valuationTime()); err != nil {
		t.Fatal(err)
	}
	if fake.calls != 4 {
		t.Fatalf("calls = %d, want 4", fake.calls)
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

func (f *marketPortFake) ListAll(context.Context) ([]market.Asset, error) {
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

func (f *marketPortFake) Session(context.Context, market.AssetID, time.Time) (market.SessionInfo, error) {
	f.calls++
	return market.SessionInfo{}, nil
}
