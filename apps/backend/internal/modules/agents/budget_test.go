package agents_test

import (
	"math"
	"testing"

	"pgregory.net/rapid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/agents/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

const xStockDecimals = 8

func micros(v uint64) money.Micros { return money.MicrosFromUint64(v) }

func tokens(v uint64) money.BaseUnits { return money.NewBaseUnits(v, xStockDecimals) }

func drawAmount(t *rapid.T, label string) uint64 { return rapid.Uint64Range(0, 1<<62).Draw(t, label) }

func TestAvailableBudget_neverExceedsAllocationPlusProceeds(t *testing.T) {
	t.Parallel()
	rapid.Check(t, func(t *rapid.T) {
		allocation, executed := drawAmount(t, "allocation"), drawAmount(t, "executed")
		inFlight, proceeds := drawAmount(t, "inFlight"), drawAmount(t, "proceeds")
		got, err := domain.AvailableBudget(micros(allocation), micros(executed), micros(inFlight), micros(proceeds))
		if err != nil || got.Uint64() > allocation+proceeds {
			t.Fatalf("AvailableBudget(%d, %d, %d, %d) = %v, %v; want at most %d",
				allocation, executed, inFlight, proceeds, got, err, allocation+proceeds)
		}
	})
}

func TestAvailableBudget_isZeroOnceSpendingReachesAllocationPlusProceeds(t *testing.T) {
	t.Parallel()
	rapid.Check(t, func(t *rapid.T) {
		allocation, proceeds := drawAmount(t, "allocation"), drawAmount(t, "proceeds")
		credit := allocation + proceeds
		executed := rapid.Uint64Range(0, credit).Draw(t, "executed")
		inFlight := credit - executed + rapid.Uint64Range(0, 1<<30).Draw(t, "overshoot")
		got, err := domain.AvailableBudget(micros(allocation), micros(executed), micros(inFlight), micros(proceeds))
		if err != nil || !got.IsZero() {
			t.Fatalf("AvailableBudget(%d, %d, %d, %d) = %v, %v; want zero",
				allocation, executed, inFlight, proceeds, got, err)
		}
	})
}

func TestAvailableBudget_risesByExactlyTheNewSellProceedsWhileUnclamped(t *testing.T) {
	t.Parallel()
	rapid.Check(t, func(t *rapid.T) {
		allocation, proceeds, extra := drawAmount(t, "allocation"), drawAmount(t, "proceeds"), drawAmount(t, "extra")
		credit := allocation + proceeds
		executed := rapid.Uint64Range(0, credit).Draw(t, "executed")
		inFlight := rapid.Uint64Range(0, credit-executed).Draw(t, "inFlight")
		budget := func(proceeds uint64) (money.Micros, error) {
			return domain.AvailableBudget(micros(allocation), micros(executed), micros(inFlight), micros(proceeds))
		}
		before, errBefore := budget(proceeds)
		after, errAfter := budget(proceeds + extra)
		if errBefore != nil || errAfter != nil || after.Uint64() != before.Uint64()+extra {
			t.Fatalf("budget %v then %v after %d more proceeds, errors %v and %v",
				before, after, extra, errBefore, errAfter)
		}
	})
}

func TestAvailableBudget_examples(t *testing.T) {
	t.Parallel()
	for name, tt := range map[string]struct{ allocation, executed, inFlight, proceeds, want uint64 }{
		"a sell refills the budget":   {100_000_000, 100_000_000, 0, 110_000_000, 110_000_000},
		"executed and in flight buys": {100_000_000, 30_000_000, 20_000_000, 0, 50_000_000},
		"spent to the last micro":     {100_000_000, 60_000_000, 40_000_000, 0, 0},
		"overspent clamps at zero":    {100_000_000, 70_000_000, 40_000_000, 0, 0},
		"nothing at all":              {0, 0, 0, 0, 0},
	} {
		got, err := domain.AvailableBudget(micros(tt.allocation), micros(tt.executed), micros(tt.inFlight),
			micros(tt.proceeds))
		if err != nil || got != micros(tt.want) {
			t.Errorf("%s: AvailableBudget = %v, %v; want %d", name, got, err, tt.want)
		}
	}
}

func TestAvailableBudget_refusesASumPastUint64(t *testing.T) {
	t.Parallel()
	for name, args := range map[string][4]uint64{
		"allocation plus proceeds":     {math.MaxUint64, 0, 0, 1},
		"executed plus in flight buys": {0, math.MaxUint64, 1, 0},
	} {
		got, err := domain.AvailableBudget(micros(args[0]), micros(args[1]), micros(args[2]), micros(args[3]))
		if errs.CodeOf(err) != errs.CodeInvalidInput || err == nil || !got.IsZero() {
			t.Errorf("%s: AvailableBudget%v = %v, %v; want zero and invalid_input", name, args, got, err)
		}
	}
}

func TestSellable_neverExceedsWhatTheAgentBoughtOrTheTreasuryHolds(t *testing.T) {
	t.Parallel()
	rapid.Check(t, func(t *rapid.T) {
		bought, sold, inFlight := drawAmount(t, "bought"), drawAmount(t, "sold"), drawAmount(t, "inFlight")
		held := drawAmount(t, "held")
		got, err := domain.Sellable(tokens(bought), tokens(sold), tokens(inFlight), tokens(held))
		if err != nil || got.Uint64() > held || got.Uint64() > bought || got.Decimals() != xStockDecimals {
			t.Fatalf("Sellable(%d, %d, %d, %d) = %v, %v; want at most %d and %d with %d decimals",
				bought, sold, inFlight, held, got, err, bought, held, xStockDecimals)
		}
	})
}

func TestSellable_examples(t *testing.T) {
	t.Parallel()
	for name, tt := range map[string]struct{ bought, sold, inFlight, held, want uint64 }{
		"what is left of the buys":    {100, 30, 20, 1000, 50},
		"limited by the treasury":     {100, 30, 20, 40, 40},
		"every buy still held":        {100, 0, 0, 100, 100},
		"sold and selling cover buys": {100, 60, 40, 1000, 0},
		"sold and selling past buys":  {100, 70, 40, 1000, 0},
		"an empty treasury":           {100, 0, 0, 0, 0},
	} {
		got, err := domain.Sellable(tokens(tt.bought), tokens(tt.sold), tokens(tt.inFlight), tokens(tt.held))
		if err != nil || got != tokens(tt.want) {
			t.Errorf("%s: Sellable = %v, %v; want %d with %d decimals", name, got, err, tt.want, xStockDecimals)
		}
	}
}

func TestSellable_refusesMismatchedDecimalsInAnyPosition(t *testing.T) {
	t.Parallel()
	rapid.Check(t, func(t *rapid.T) {
		decimals := rapid.Uint8().Draw(t, "decimals")
		other := rapid.Uint8().Filter(func(d uint8) bool { return d != decimals }).Draw(t, "other")
		position := rapid.IntRange(0, 3).Draw(t, "position")
		args := [4]money.BaseUnits{}
		for i := range args {
			args[i] = money.NewBaseUnits(drawAmount(t, "amount"), decimals)
		}
		args[position] = money.NewBaseUnits(drawAmount(t, "odd one out"), other)
		got, err := domain.Sellable(args[0], args[1], args[2], args[3])
		if errs.CodeOf(err) != errs.CodeInvalidInput || err == nil || got != (money.BaseUnits{}) {
			t.Fatalf("Sellable%v = %v, %v; want zero and invalid_input", args, got, err)
		}
	})
}

func TestSellable_refusesSoldPlusSellingPastUint64(t *testing.T) {
	t.Parallel()
	got, err := domain.Sellable(tokens(1), tokens(math.MaxUint64), tokens(1), tokens(1))
	if errs.CodeOf(err) != errs.CodeInvalidInput || err == nil || got != (money.BaseUnits{}) {
		t.Fatalf("Sellable = %v, %v; want zero and invalid_input", got, err)
	}
}
