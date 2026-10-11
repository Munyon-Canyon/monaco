package app

import (
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/modules/market"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/multichain"
)

func TestDisplayQuoteConvertsTheWinnersSharesIntoTheAssetsUnits(t *testing.T) {
	t.Parallel()
	winner := &multichain.Comparison{Ranked: []multichain.Quote{{NetShares: 306_671_961}}}
	at := time.Unix(1_790_000_000, 0)
	asset := func(decimals uint8, num, den int64) market.Asset {
		a := market.Asset{Decimals: decimals}
		a.UIMultiplier.Num, a.UIMultiplier.Den = num, den
		return a
	}
	for name, tc := range map[string]struct {
		asset      market.Asset
		comparison *multichain.Comparison
		want       uint64
	}{
		"no comparison": {asset(8, 1, 1), nil, 7},
		"one share per token": {
			asset(8, 1, 1), winner, 30_667_196,
		},
		"shares per token above one": {
			asset(8, 10_032_690, 10_000_000), winner, 30_567_271,
		},
		"no multiplier": {asset(8, 0, 0), winner, 7},
		"too large": {
			asset(250, 1, 1), winner, 7,
		},
	} {
		if got := displayQuote(tc.asset, tc.comparison, 7, at); got != tc.want {
			t.Errorf("%s: displayQuote = %d, want %d", name, got, tc.want)
		}
	}
}
