package domain_test

import (
	"testing"

	"pgregory.net/rapid"

	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

func micros(v int64) money.Micros {
	m, _ := money.SignedMicrosFromInt64(v).Micros()
	return m
}

func TestBridgeAdjustProperty_theLandingPlusItsAdjustmentBooksTheRealFee(t *testing.T) {
	t.Parallel()
	ids := testkit.NewIDs(1)
	swap, landedID, adjustID := ids.NewV7(), ids.NewV7(), ids.NewV7()
	cash := domain.EVMCashAsset(chain.NetworkRobinhood)
	rapid.Check(t, func(rt *rapid.T) {
		sent := rapid.Int64Range(2, 1<<50).Draw(rt, "sent")
		estimate := rapid.Int64Range(0, sent-1).Draw(rt, "estimate")
		paid := rapid.Int64Range(0, sent-1).Draw(rt, "paid")
		b := domain.Bridge{
			SwapID: swap, CabalID: cabalID(t, 7), Direction: domain.BridgeOut, Network: chain.NetworkRobinhood,
			Sent: micros(sent), Fee: micros(estimate),
		}
		landed, err := domain.NewBridgeLandedTxn(landedID, b, usdc)
		if err != nil {
			rt.Fatal(err)
		}
		b.Fee = micros(paid)
		adjust, posted, err := domain.NewBridgeAdjustedTxn(adjustID, b, micros(estimate), usdc)
		if err != nil || posted != (paid != estimate) {
			rt.Fatalf("adjust = %v, %v, want one only when the fee moved", posted, err)
		}
		balance := map[domain.CabalAccount]int64{}
		for _, e := range append(landed.Entries(), adjust.Entries()...) {
			if e.Asset == cash {
				balance[e.Account] += e.Amount.Int64()
			}
		}
		if !zeroPerAsset(adjust.Entries()) || balance[domain.CabalTreasury] != sent-paid ||
			balance[domain.CabalFees] != paid {
			rt.Fatalf("treasury %d, fees %d, want %d landed and the %d fee", balance[domain.CabalTreasury],
				balance[domain.CabalFees], sent-paid, paid)
		}
	})
}
