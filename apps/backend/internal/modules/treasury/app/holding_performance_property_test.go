package app

import (
	"math/big"
	"testing"

	"pgregory.net/rapid"

	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/domain"
)

func replaySales(t *rapid.T, trades []trade) (*salesTotal, *replayed, *big.Int, *big.Int) {
	const asset = "mint"
	names := map[domain.Asset]AssetName{asset: {Decimals: 8}}
	sales, state, shares := &salesTotal{}, &replayed{}, sharesFor(asset, names, nil)
	bought, proceeds := new(big.Int), new(big.Int)
	for _, tr := range trades {
		if err := sales.apply(state, tr, shares); err != nil {
			t.Fatal(err)
		}
		total := bought
		if tr.side == sideSell {
			total = proceeds
		}
		total.Add(total, new(big.Int).SetUint64(tr.usdc))
	}
	return sales, state, bought, proceeds
}

func TestSalesTotal_realizedIsProceedsLessTheCostRemovedAndAvgNeverRoundsUp(t *testing.T) {
	t.Parallel()
	rapid.Check(t, func(t *rapid.T) {
		sales, state, bought, proceeds := replaySales(t, drawTrades(t))
		got, err := sales.result()
		if err != nil {
			t.Fatal(err)
		}
		removed := new(big.Int).Sub(bought, new(big.Int).SetUint64(state.cost))
		want := new(big.Int).Sub(proceeds, removed)
		if big.NewInt(got.RealizedPnLMicros.Int64()).Cmp(want) != 0 {
			t.Fatalf("realized = %d, want proceeds %s less removed %s = %s",
				got.RealizedPnLMicros.Int64(), proceeds, removed, want)
		}
		if sales.shares.Sign() == 0 {
			if got.AvgSellMicros != nil {
				t.Fatalf("avg = %v with nothing sold, want nil", got.AvgSellMicros)
			}
			return
		}
		avg := new(big.Rat).SetInt(new(big.Int).SetUint64(got.AvgSellMicros.Uint64()))
		if back := avg.Mul(avg, &sales.shares); back.Cmp(new(big.Rat).SetInt(proceeds)) > 0 {
			t.Fatalf("avg %s × shares %s = %s exceeds proceeds %s", got.AvgSellMicros, &sales.shares, back, proceeds)
		}
	})
}
