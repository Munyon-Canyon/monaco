package app

import (
	"math"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/sqlc"
)

func marksBase() time.Time { return time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC) }

func marksTrade(i int, side string, units, usdc uint64) trade {
	return trade{
		id:    uuid.UUID{byte(uint(i) % 200)},
		at:    marksBase().Add(time.Duration(i) * time.Hour),
		side:  side,
		units: units,
		usdc:  usdc,
	}
}

func TestReplayTrades_costOfASaleIsTheTruncatedShareOfTheAverageCost(t *testing.T) {
	t.Parallel()
	name := AssetName{Decimals: 8}
	marks, err := replayTrades([]trade{
		marksTrade(0, sideBuy, 100_000_000, 300_000_000),
		marksTrade(1, sideBuy, 200_000_000, 600_000_001),
		marksTrade(2, sideSell, 100_000_000, 500_000_000),
		marksTrade(3, sideSell, 200_000_000, 700_000_000),
		marksTrade(4, sideBuy, 100_000_000, 400_000_000),
	}, name)
	if err != nil || len(marks) != 5 {
		t.Fatalf("marks = %d, err %v, want 5", len(marks), err)
	}
	if got := marks[2].CostOfSoldMicros.Uint64(); got != 300_000_000 {
		t.Fatalf("first sale cost = %d, want 300000000 (one third of 900000001 truncated)", got)
	}
	if got := marks[3].CostOfSoldMicros.Uint64(); got != 600_000_001 {
		t.Fatalf("second sale cost = %d, want the remainder 600000001", got)
	}
	if got := marks[0].PriceMicros.Uint64(); got != 300_000_000 {
		t.Fatalf("buy price = %d, want 300000000", got)
	}
	for i, want := range []time.Time{marksBase(), marksBase(), marksBase(), marksBase(), marksBase().Add(4 * time.Hour)} {
		if !marks[i].CycleStart.Equal(want) {
			t.Fatalf("mark %d cycle start = %v, want %v", i, marks[i].CycleStart, want)
		}
	}
}

func TestReplayTrades_pricesAtTheMultiplierOfEachTrade(t *testing.T) {
	t.Parallel()
	name := AssetName{Decimals: 8, MultiplierAt: func(time.Time) (int64, int64) { return 2, 1 }}
	marks, err := replayTrades([]trade{marksTrade(0, sideBuy, 100_000_000, 300_000_000)}, name)
	if err != nil || marks[0].PriceMicros.Uint64() != 150_000_000 {
		t.Fatalf("marks = %+v, err %v, want a price of 150000000 for two shares", marks, err)
	}
}

func TestReplayTrades_refusesWhatItCannotReplay(t *testing.T) {
	t.Parallel()
	name := AssetName{Decimals: 8}
	for label, c := range map[string]struct {
		trades []trade
		name   AssetName
		code   errs.Code
	}{
		"sell with nothing held": {[]trade{marksTrade(0, sideSell, 1, 1)}, name, errs.CodeInternal},
		"sell more than held":    {[]trade{marksTrade(0, sideBuy, 1, 1), marksTrade(1, sideSell, 2, 1)}, name, errs.CodeInternal},
		"unknown side":           {[]trade{marksTrade(0, "fund", 1, 1)}, name, errs.CodeInternal},
		"price past uint64":      {[]trade{marksTrade(0, sideBuy, 1, math.MaxUint64)}, name, errs.CodeInvalidInput},
		"too many decimals":      {[]trade{marksTrade(0, sideBuy, 1, 1)}, AssetName{Decimals: 20}, errs.CodeInvalidInput},
		"cost past uint64":       {[]trade{marksTrade(0, sideBuy, 100_000_000, math.MaxUint64), marksTrade(1, sideBuy, 100_000_000, 1)}, name, errs.CodeInternal},
	} {
		if _, err := replayTrades(c.trades, c.name); errs.CodeOf(err) != c.code {
			t.Errorf("%s: err = %v, want code %s", label, err, c.code)
		}
	}
}

func TestTradeOf_refusesRowsThatAreNotATrade(t *testing.T) {
	t.Parallel()
	valid := sqlc.ListTradesForPairsRow{ID: uuid.UUID{1}, Kind: sideBuy, OccurredAt: marksBase()}
	if _, err := tradeOf(valid); errs.CodeOf(err) != errs.CodeInternal {
		t.Fatalf("null amounts: err = %v, want internal", err)
	}
}
