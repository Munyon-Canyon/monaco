package domain_test

import (
	"math"
	"strconv"
	"testing"

	"pgregory.net/rapid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

func TestNewSwapTxn_refusesASwapWithNoLegs(t *testing.T) {
	t.Parallel()
	id := testkit.NewIDs(1).NewV7()
	_, err := domain.NewSwapTxn(id, domain.Swap{Fee: money.MicrosFromUint64(0)}, usdc)
	wantCode(t, err, errs.CodeInvalidInput)
}

func TestNewSwapTxn_refusesACashLegPastInt64(t *testing.T) {
	t.Parallel()
	aapl := domain.Asset("XsbEhLAtcf6HdfpFZ5xEMdqW8nfAvcsP5bdudRLJzJp")
	id := testkit.NewIDs(1).NewV7()
	pastInt64 := bigUnits(t, "9223372036854775808")
	for name, s := range map[string]domain.Swap{
		"usdc in":  {In: domain.Leg{Asset: usdc, Amount: pastInt64}, Out: domain.Leg{Asset: aapl, Amount: wide(1)}},
		"usdc out": {In: domain.Leg{Asset: aapl, Amount: wide(1)}, Out: domain.Leg{Asset: usdc, Amount: pastInt64}},
		"evm cash": {
			In:  domain.Leg{Asset: domain.EVMCashAsset(chain.NetworkBase), Amount: pastInt64},
			Out: domain.Leg{Asset: aapl, Amount: wide(1)},
		},
	} {
		if _, err := domain.NewSwapTxn(id, s, usdc); errs.CodeOf(err) != errs.CodeInvalidInput {
			t.Errorf("%s: err = %v, want invalid_input", name, err)
		}
	}
}

func TestNewSwapTxn_balancesTokenLegsPastInt64AndUint64(t *testing.T) {
	t.Parallel()
	aapl := domain.Asset("XsbEhLAtcf6HdfpFZ5xEMdqW8nfAvcsP5bdudRLJzJp")
	id := testkit.NewIDs(1).NewV7()
	tests := map[string]domain.Swap{
		"a 10 token buy": {
			In: domain.Leg{
				Asset:  usdc,
				Amount: wide(50_000_000),
			},
			Out: domain.Leg{Asset: aapl, Amount: bigUnits(t, "10000000000000000000")},
		},
		"a 25 token buy": {
			In: domain.Leg{
				Asset:  usdc,
				Amount: wide(125_000_000),
			},
			Out: domain.Leg{Asset: aapl, Amount: bigUnits(t, "25000000000000000000")},
			Fee: money.MicrosFromUint64(2),
		},
		"a 12 token sell": {
			In: domain.Leg{
				Asset:  aapl,
				Amount: bigUnits(t, "12000000000000000000"),
			},
			Out: domain.Leg{Asset: usdc, Amount: wide(60_000_000)},
		},
		"the largest amount": {
			In: domain.Leg{
				Asset:  usdc,
				Amount: wide(1),
			},
			Out: domain.Leg{Asset: aapl, Amount: bigUnits(t, maxUnitsText)},
		},
	}
	for name, s := range tests {
		txn, err := domain.NewSwapTxn(id, s, usdc)
		if err != nil || !zeroPerAsset(txn.Entries()) {
			t.Fatalf("%s: txn = %+v, %v, want a balanced swap", name, txn.Entries(), err)
		}
		for _, leg := range []domain.Leg{s.In, s.Out} {
			if leg.Asset == aapl && !carries(txn.Entries(), aapl, leg.Amount) {
				t.Fatalf("%s: no entry carries the exact %s units in %+v", name, leg.Amount, txn.Entries())
			}
		}
	}
}

func carries(entries []domain.CabalEntry, asset domain.Asset, amount money.Units) bool {
	for _, e := range entries {
		if e.Asset == asset && e.Amount.Abs().Equal(amount) {
			return true
		}
	}
	return false
}

const maxUnitsText = "115792089237316195423570985008687907853269984665640564039457584007913129639935"

func bigUnits(tb testing.TB, raw string) money.Units {
	tb.Helper()
	v, err := money.ParseUnits(raw)
	if err != nil {
		tb.Fatal(err)
	}
	return v
}

func TestNewSwapTxn_postsEachNonZeroLegAgainstItsCounterparty(t *testing.T) {
	t.Parallel()
	aapl := domain.Asset("XsbEhLAtcf6HdfpFZ5xEMdqW8nfAvcsP5bdudRLJzJp")
	g := testkit.NewIDs(1)
	id, swap := g.NewV7(), g.NewV7()
	txn, err := domain.NewSwapTxn(id, domain.Swap{
		ID: swap, In: domain.Leg{Asset: usdc, Amount: wide(60)}, Out: domain.Leg{Asset: aapl, Amount: wide(3)},
		Fee: money.MicrosFromUint64(2),
	}, usdc)
	if err != nil {
		t.Fatal(err)
	}
	want := []domain.CabalEntry{
		{Account: domain.CabalTreasury, Asset: usdc, Amount: units(-60)},
		{Account: domain.CabalVenue, Asset: usdc, Amount: units(60)},
		{Account: domain.CabalVenue, Asset: aapl, Amount: units(-3)},
		{Account: domain.CabalTreasury, Asset: aapl, Amount: units(3)},
		{Account: domain.CabalTreasury, Asset: usdc, Amount: units(-2)},
		{Account: domain.CabalFees, Asset: usdc, Amount: units(2)},
	}
	if !sameEntries(txn.Entries(), want) || txn.ID != id || txn.SwapID != swap ||
		txn.Kind != domain.CabalSwap || txn.Status != domain.TxnSettled {
		t.Fatalf("txn = %+v %+v, want a settled swap header with %+v", txn.CabalTxnHeader, txn.Entries(), want)
	}
}

func TestSwapProperty_aBuyBalancesPerAssetAndCostsWhatTheTreasuryPaid(t *testing.T) {
	t.Parallel()
	aapl := domain.Asset("XsbEhLAtcf6HdfpFZ5xEMdqW8nfAvcsP5bdudRLJzJp")
	id := testkit.NewIDs(1).NewV7()
	rapid.Check(t, func(t *rapid.T) {
		in := rapid.Uint64Range(1, 1<<50).Draw(t, "in")
		fee := rapid.Uint64Range(0, 1<<40).Draw(t, "fee")
		out := rapid.Uint64Range(1, math.MaxInt64).Draw(t, "out")
		txn, err := domain.NewSwapTxn(id, domain.Swap{
			In: domain.Leg{Asset: usdc, Amount: wide(in)}, Out: domain.Leg{Asset: aapl, Amount: wide(out)},
			Fee: money.MicrosFromUint64(fee),
		}, usdc)
		if err != nil {
			t.Fatal(err)
		}
		if !zeroPerAsset(txn.Entries()) {
			t.Fatalf("entries %+v do not sum to zero per asset", txn.Entries())
		}
		deltas, err := txn.PositionDeltas(usdc)
		if err != nil {
			t.Fatal(err)
		}
		for _, d := range deltas {
			if d.Asset == aapl && (d.CostIn.Uint64() != in+fee || d.Units.String() != strconv.FormatUint(out, 10)) {
				t.Fatalf("bought delta = %+v, want %d units costing %d", d, out, in+fee)
			}
		}
	})
}

func TestNewSwapTxn_aCashLegSoldForItselfNetsToNothing(t *testing.T) {
	t.Parallel()
	cash := domain.EVMCashAsset(chain.NetworkBase)
	txn, err := domain.NewSwapTxn(testkit.NewIDs(1).NewV7(), domain.Swap{
		ID:  testkit.NewIDs(2).NewV7(),
		In:  domain.Leg{Asset: cash, Amount: wide(40_000_000)},
		Out: domain.Leg{Asset: cash, Amount: wide(40_000_000)},
	}, usdc)
	if err != nil {
		t.Fatal(err)
	}
	net := map[domain.CabalAccount]int64{}
	for _, e := range txn.Entries() {
		if e.Asset != cash {
			t.Fatalf("entry %+v, want only the EVM cash asset", e)
		}
		net[e.Account] += e.Amount.Int64OrZero()
	}
	if net[domain.CabalTreasury] != 0 || net[domain.CabalVenue] != 0 {
		t.Fatalf("net by account = %v, want the treasury and venue unchanged", net)
	}
}
