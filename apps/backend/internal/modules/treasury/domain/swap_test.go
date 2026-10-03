package domain_test

import (
	"math"
	"slices"
	"strconv"
	"testing"

	"pgregory.net/rapid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

func TestNewSwapTxn_refusesLegsPastInt64AndASwapWithNoLegs(t *testing.T) {
	t.Parallel()
	aapl := domain.Asset("XsbEhLAtcf6HdfpFZ5xEMdqW8nfAvcsP5bdudRLJzJp")
	id := testkit.NewIDs(1).NewV7()
	for name, s := range map[string]domain.Swap{
		"in past int64":   {In: domain.Leg{Asset: usdc, Amount: math.MaxInt64 + 1}},
		"out past uint63": {Out: domain.Leg{Asset: aapl, Amount: math.MaxUint64}},
		"fee only zero":   {Fee: money.MicrosFromUint64(0)},
	} {
		if _, err := domain.NewSwapTxn(id, s, usdc); errs.CodeOf(err) != errs.CodeInvalidInput {
			t.Errorf("%s: err = %v, want invalid_input", name, err)
		}
	}
}

func TestNewSwapTxn_postsEachNonZeroLegAgainstItsCounterparty(t *testing.T) {
	t.Parallel()
	aapl := domain.Asset("XsbEhLAtcf6HdfpFZ5xEMdqW8nfAvcsP5bdudRLJzJp")
	g := testkit.NewIDs(1)
	id, swap := g.NewV7(), g.NewV7()
	txn, err := domain.NewSwapTxn(id, domain.Swap{
		ID: swap, In: domain.Leg{Asset: usdc, Amount: 60}, Out: domain.Leg{Asset: aapl, Amount: 3},
		Fee: money.MicrosFromUint64(2),
	}, usdc)
	if err != nil {
		t.Fatal(err)
	}
	want := []domain.CabalEntry{
		{Account: domain.CabalTreasury, Asset: usdc, Amount: amount(-60)},
		{Account: domain.CabalVenue, Asset: usdc, Amount: amount(60)},
		{Account: domain.CabalVenue, Asset: aapl, Amount: amount(-3)},
		{Account: domain.CabalTreasury, Asset: aapl, Amount: amount(3)},
		{Account: domain.CabalTreasury, Asset: usdc, Amount: amount(-2)},
		{Account: domain.CabalFees, Asset: usdc, Amount: amount(2)},
	}
	if !slices.Equal(txn.Entries(), want) || txn.ID != id || txn.SwapID != swap ||
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
			In: domain.Leg{Asset: usdc, Amount: in}, Out: domain.Leg{Asset: aapl, Amount: out},
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
