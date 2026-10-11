package app

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/market/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/multichain"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

func stockOf(t *testing.T, id domain.AssetID) multichain.StockID {
	t.Helper()
	var stock multichain.StockID
	if err := stock.UnmarshalText([]byte(id.String())); err != nil {
		t.Fatal(err)
	}
	return stock
}

type samples struct {
	got map[domain.AssetID]domain.Sample
	err error
}

func (s samples) PricesAsOf(context.Context, []domain.AssetID, time.Time) (map[domain.AssetID]domain.Sample, error) {
	return s.got, s.err
}

func TestStockPricesReadsTheLatestSharePrice(t *testing.T) {
	t.Parallel()
	g := testkit.NewIDs(4973)
	id := domain.NewAssetID(g)
	stock := stockOf(t, id)
	clk := clock.Real{}
	price, err := NewStockPrices(samples{got: map[domain.AssetID]domain.Sample{
		id: {Micros: money.MicrosFromUint64(336_844_000)},
	}}, clk).SharePrice(t.Context(), stock)
	if err != nil || price.Uint64() != 336_844_000 {
		t.Fatalf("price = %s, %v", price, err)
	}
	failing := errs.New(errs.CodeInternal, "test")
	for name, tc := range map[string]struct {
		stock  multichain.StockID
		prices samples
	}{
		"zero stock":  {multichain.StockID{}, samples{}},
		"read fails":  {stock, samples{err: failing}},
		"no sample":   {stock, samples{got: map[domain.AssetID]domain.Sample{}}},
		"zero sample": {stock, samples{got: map[domain.AssetID]domain.Sample{id: {}}}},
	} {
		if _, err := NewStockPrices(tc.prices, clk).SharePrice(t.Context(), tc.stock); err == nil {
			t.Errorf("%s: no error", name)
		} else if name == "read fails" && !errors.Is(err, failing) {
			t.Errorf("%s: err = %v", name, err)
		}
	}
}
