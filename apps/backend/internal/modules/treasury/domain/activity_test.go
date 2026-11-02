package domain_test

import (
	"fmt"
	"strconv"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

func optional(v *uint64) string {
	if v == nil {
		return "null"
	}
	return strconv.FormatUint(*v, 10)
}

func optionalUnits(v *money.Units) string {
	if v == nil {
		return "null"
	}
	return v.String()
}

func TestTradeActivity_readsTheCashAndTheAssetFromTheSideTheActionNames(t *testing.T) {
	t.Parallel()
	aapl := domain.Asset("XsbEhLAtcf6HdfpFZ5xEMdqW8nfAvcsP5bdudRLJzJp")
	swap := testkit.NewIDs(1).NewV7()
	cash, stock := domain.Leg{Asset: usdc, Amount: wide(60)}, domain.Leg{Asset: aapl, Amount: wide(3)}
	tests := map[string]struct {
		trade domain.Trade
		want  string
	}{
		"buy":          {domain.Trade{Action: "buy", In: cash, Out: stock}, "buy 60 3"},
		"sell":         {domain.Trade{Action: "sell", In: stock, Out: cash}, "sell 60 3"},
		"unfilled buy": {domain.Trade{Action: "buy", In: cash, Out: domain.Leg{Asset: aapl}}, "buy 60 null"},
		"bridged buy":  {domain.Trade{Action: "buy", In: cash, Out: stock, Fee: 5}, "buy 65 3"},
		"bridged sell": {domain.Trade{Action: "sell", In: stock, Out: cash, Fee: 5}, "sell 60 3"},
	}
	for name, tc := range tests {
		tc.trade.SwapID = swap
		a, err := domain.TradeActivity(tc.trade, domain.ActivityPending)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		got := fmt.Sprintf("%s %s %s", a.Kind, optional(a.USDCMicros), optionalUnits(a.Units))
		if got != tc.want || a.ID != swap || a.Status != domain.ActivityPending || a.Asset != aapl {
			t.Fatalf("%s: activity = %+v (%s), want %s of %s", name, a, got, tc.want, aapl)
		}
	}
	_, err := domain.TradeActivity(domain.Trade{Action: "hold"}, domain.ActivityFailed)
	wantCode(t, err, errs.CodeInvalidInput)
}

func TestTradeActivity_keepsTokenAmountsPastUint64AndRefusesCashItCannotHold(t *testing.T) {
	t.Parallel()
	aapl := domain.Asset("XsbEhLAtcf6HdfpFZ5xEMdqW8nfAvcsP5bdudRLJzJp")
	twentyFive := bigUnits(t, "25000000000000000000")
	buy, err := domain.TradeActivity(domain.Trade{
		Action: "buy",
		In:     domain.Leg{Asset: usdc, Amount: wide(125_000_000)},
		Out:    domain.Leg{Asset: aapl, Amount: twentyFive},
	}, domain.ActivityConfirmed)
	if err != nil || buy.Units == nil || !buy.Units.Equal(twentyFive) || optional(buy.USDCMicros) != "125000000" {
		t.Fatalf("25 token buy = %+v, %v", buy, err)
	}
	sell, err := domain.TradeActivity(domain.Trade{
		Action: "sell",
		In:     domain.Leg{Asset: aapl, Amount: twentyFive},
		Out:    domain.Leg{Asset: usdc, Amount: wide(130_000_000)},
	}, domain.ActivityConfirmed)
	if err != nil || sell.Units == nil || !sell.Units.Equal(twentyFive) || optional(sell.USDCMicros) != "130000000" {
		t.Fatalf("25 token sell = %+v, %v", sell, err)
	}
	for name, trade := range map[string]domain.Trade{
		"cash past uint64": {Action: "buy", In: domain.Leg{Asset: usdc, Amount: twentyFive}, Out: domain.Leg{Asset: aapl, Amount: wide(1)}},
		"cash plus fee past 2^256": {
			Action: "buy", In: domain.Leg{Asset: usdc, Amount: bigUnits(t, maxUnitsText)}, Fee: 1,
			Out: domain.Leg{Asset: aapl, Amount: wide(1)},
		},
	} {
		if _, err := domain.TradeActivity(trade, domain.ActivityConfirmed); errs.CodeOf(err) != errs.CodeInvalidInput {
			t.Fatalf("%s: err = %v, want invalid_input", name, err)
		}
	}
}

func TestPositionDeltas_refusesASumPast2To256(t *testing.T) {
	t.Parallel()
	top := mustSignedUnits(t, maxUnitsText)
	txn := cabalTxn(t,
		domain.CabalEntry{Account: domain.CabalTreasury, Asset: aapl, Amount: top},
		domain.CabalEntry{Account: domain.CabalTreasury, Asset: aapl, Amount: top},
		domain.CabalEntry{Account: domain.CabalVenue, Asset: aapl, Amount: top.Neg()},
		domain.CabalEntry{Account: domain.CabalVenue, Asset: aapl, Amount: top.Neg()},
	)
	_, err := txn.PositionDeltas(usdc)
	wantCode(t, err, errs.CodeInvalidInput)
}
