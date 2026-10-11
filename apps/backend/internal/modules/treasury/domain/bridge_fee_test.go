package domain_test

import (
	"math"
	"slices"
	"testing"

	"pgregory.net/rapid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

func TestNewBridgeLandedTxn_recordsTheFeeOnTheFeesAccountInTheLandedAsset(t *testing.T) {
	t.Parallel()
	base := domain.EVMCashAsset(chain.NetworkBase)
	id := ids.Real{}.NewV7()
	for name, tc := range map[string]struct {
		direction domain.BridgeDirection
		asset     domain.Asset
	}{
		"out":  {domain.BridgeOut, base},
		"back": {domain.BridgeBack, usdc},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			txn, err := domain.NewBridgeLandedTxn(id, domain.Bridge{
				SwapID: id, CabalID: cabalID(t, 7), Direction: tc.direction, Network: chain.NetworkBase,
				Sent: money.MicrosFromUint64(10_000_000), Fee: money.MicrosFromUint64(7_500),
			}, usdc)
			want := []domain.CabalEntry{
				leg(domain.CabalVenue, tc.asset, -9_992_500), leg(domain.CabalTreasury, tc.asset, 9_992_500),
				leg(domain.CabalVenue, tc.asset, -7_500), leg(domain.CabalFees, tc.asset, 7_500),
			}
			if err != nil || !slices.Equal(txn.Entries(), want) {
				t.Fatalf("landed = %+v, %v, want %+v", txn.Entries(), err, want)
			}
			wantDeltas(t, name, txn, domain.CabalPositionDelta{
				Asset: tc.asset, Units: amount(9_992_500), CostIn: money.MicrosFromUint64(9_992_500),
			})
		})
	}
}

func TestNewBridgeLandedTxn_refusesAFeePastInt64(t *testing.T) {
	t.Parallel()
	id := ids.Real{}.NewV7()
	_, err := domain.NewBridgeLandedTxn(id, domain.Bridge{
		SwapID: id, CabalID: cabalID(t, 7), Direction: domain.BridgeOut, Network: chain.NetworkBase,
		Sent: money.MicrosFromUint64(math.MaxInt64 + 2), Fee: money.MicrosFromUint64(math.MaxInt64 + 1),
	}, usdc)
	wantCode(t, err, errs.CodeInvalidInput)
}

func TestNewSwapTxn_capitalizesTheBridgeFeeOfAnEVMBuyWithoutMovingSolanaCash(t *testing.T) {
	t.Parallel()
	base := domain.EVMCashAsset(chain.NetworkBase)
	token := domain.Asset("base:0xaa00000000000000000000000000000000000001")
	id := testkit.NewIDs(1).NewV7()
	txn, err := domain.NewSwapTxn(id, domain.Swap{
		In: domain.Leg{Asset: base, Amount: 9_992_500}, Out: domain.Leg{Asset: token, Amount: 400},
		Fee: money.MicrosFromUint64(7_500),
	}, usdc)
	want := []domain.CabalEntry{
		leg(domain.CabalTreasury, base, -9_992_500), leg(domain.CabalVenue, base, 9_992_500),
		leg(domain.CabalVenue, token, -400), leg(domain.CabalTreasury, token, 400),
		leg(domain.CabalFees, base, -7_500), leg(domain.CabalVenue, base, 7_500),
	}
	if err != nil || !slices.Equal(txn.Entries(), want) {
		t.Fatalf("swap = %+v, %v, want %+v", txn.Entries(), err, want)
	}
	wantDeltas(t, "evm buy", txn,
		domain.CabalPositionDelta{Asset: base, Units: amount(-9_992_500)},
		domain.CabalPositionDelta{Asset: token, Units: amount(400), CostIn: money.MicrosFromUint64(10_000_000)},
	)
}

func TestPositionDeltas_capitalizesOnlyNegativeFeesEntriesInACashAsset(t *testing.T) {
	t.Parallel()
	base := domain.EVMCashAsset(chain.NetworkBase)
	wantDeltas(t, "a negative fee in a token is not capitalized", cabalTxn(t,
		leg(domain.CabalTreasury, base, -10), leg(domain.CabalVenue, base, 10),
		leg(domain.CabalTreasury, aapl, 5), leg(domain.CabalVenue, aapl, -5),
		leg(domain.CabalFees, tsla, -3), leg(domain.CabalVenue, tsla, 3),
	),
		domain.CabalPositionDelta{Asset: aapl, Units: amount(5), CostIn: money.MicrosFromUint64(10)},
		domain.CabalPositionDelta{Asset: base, Units: amount(-10)},
	)
}

func TestSwapProperty_aBridgedBuyCostsTheLandedCashPlusTheBridgeFee(t *testing.T) {
	t.Parallel()
	base := domain.EVMCashAsset(chain.NetworkBase)
	token := domain.Asset("base:0xaa00000000000000000000000000000000000001")
	id := testkit.NewIDs(1).NewV7()
	rapid.Check(t, func(t *rapid.T) {
		landed := rapid.Uint64Range(1, 1<<50).Draw(t, "landed")
		fee := rapid.Uint64Range(0, 1<<40).Draw(t, "fee")
		txn, err := domain.NewSwapTxn(id, domain.Swap{
			In: domain.Leg{Asset: base, Amount: landed}, Out: domain.Leg{Asset: token, Amount: 1},
			Fee: money.MicrosFromUint64(fee),
		}, usdc)
		if err != nil {
			t.Fatal(err)
		}
		deltas, err := txn.PositionDeltas(usdc)
		if err != nil || !zeroPerAsset(txn.Entries()) || deltas[1].CostIn != money.MicrosFromUint64(landed+fee) {
			t.Fatalf("deltas %+v, %v for %+v, want a balanced swap costing %d", deltas, err, txn.Entries(), landed+fee)
		}
	})
}
