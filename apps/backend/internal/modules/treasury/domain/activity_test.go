package domain_test

import (
	"fmt"
	"strconv"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/domain"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

func optional(v *uint64) string {
	if v == nil {
		return "null"
	}
	return strconv.FormatUint(*v, 10)
}

func TestTradeActivity_readsTheCashAndTheAssetFromTheSideTheActionNames(t *testing.T) {
	t.Parallel()
	aapl := domain.Asset("XsbEhLAtcf6HdfpFZ5xEMdqW8nfAvcsP5bdudRLJzJp")
	swap := testkit.NewIDs(1).NewV7()
	cash, stock := domain.Leg{Asset: usdc, Amount: 60}, domain.Leg{Asset: aapl, Amount: 3}
	tests := map[string]struct {
		trade domain.Trade
		want  string
	}{
		"buy":          {domain.Trade{Action: "buy", In: cash, Out: stock}, "buy 60 3"},
		"sell":         {domain.Trade{Action: "sell", In: stock, Out: cash}, "sell 60 3"},
		"unfilled buy": {domain.Trade{Action: "buy", In: cash, Out: domain.Leg{Asset: aapl}}, "buy 60 null"},
	}
	for name, tc := range tests {
		tc.trade.SwapID = swap
		a, err := domain.TradeActivity(tc.trade, domain.ActivityPending)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		got := fmt.Sprintf("%s %s %s", a.Kind, optional(a.USDCMicros), optional(a.Units))
		if got != tc.want || a.ID != swap || a.Status != domain.ActivityPending || a.Asset != aapl {
			t.Fatalf("%s: activity = %+v (%s), want %s of %s", name, a, got, tc.want, aapl)
		}
	}
	_, err := domain.TradeActivity(domain.Trade{Action: "hold"}, domain.ActivityFailed)
	wantCode(t, err, errs.CodeInvalidInput)
}
