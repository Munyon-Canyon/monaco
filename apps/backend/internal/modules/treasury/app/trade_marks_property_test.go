package app

import (
	"math"
	"math/big"
	"reflect"
	"testing"
	"time"

	"pgregory.net/rapid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

func drawTrades(t *rapid.T) []trade {
	var trades []trade
	var held uint64
	for i := range rapid.IntRange(1, 14).Draw(t, "count") {
		if held == 0 || rapid.Bool().Draw(t, "buy") {
			units := rapid.Uint64Range(1, 1_000_000).Draw(t, "units")
			trades = append(trades, marksTrade(i, sideBuy, units, rapid.Uint64Range(0, 1_000_000_000).Draw(t, "usdc")))
			held += units
			continue
		}
		units := rapid.Uint64Range(1, held).Draw(t, "sold")
		trades = append(trades, marksTrade(i, sideSell, units, rapid.Uint64Range(0, 1_000_000_000).Draw(t, "proceeds")))
		held -= units
	}
	return trades
}

func TestReplayTrades_conservesCostAndRemovesItInProportion(t *testing.T) {
	t.Parallel()
	rapid.Check(t, func(t *rapid.T) {
		trades := drawTrades(t)
		marks, err := replayTrades(trades, AssetName{Decimals: 8})
		if err != nil {
			t.Fatal(err)
		}
		held, cost, bought, removedTotal := new(big.Int), new(big.Int), new(big.Int), new(big.Int)
		for i, tr := range trades {
			units, usdc := tr.units.Big(), new(big.Int).SetUint64(tr.usdc)
			if tr.side == sideBuy {
				held.Add(held, units)
				cost.Add(cost, usdc)
				bought.Add(bought, usdc)
				continue
			}
			want := new(big.Int).Quo(new(big.Int).Mul(cost, units), held)
			got := new(big.Int).SetUint64(marks[i].CostOfSoldMicros.Uint64())
			if got.Cmp(want) != 0 {
				t.Fatalf("sale %d removed %s, want floor(%s × %s ÷ %s) = %s", i, got, cost, units, held, want)
			}
			held.Sub(held, units)
			cost.Sub(cost, want)
			removedTotal.Add(removedTotal, want)
		}
		if sum := new(big.Int).Add(removedTotal, cost); sum.Cmp(bought) != 0 {
			t.Fatalf("removed %s + remaining %s != bought %s", removedTotal, cost, bought)
		}
		if held.Sign() == 0 && cost.Sign() != 0 {
			t.Fatalf("empty position keeps cost %s", cost)
		}
	})
}

func TestReplayTrades_keepsHoldingsPastUint64Exact(t *testing.T) {
	t.Parallel()
	wholeShares := func(n int64) money.Units {
		v, err := money.UnitsFromBig(
			new(big.Int).Mul(big.NewInt(n), new(big.Int).Exp(big.NewInt(10), big.NewInt(18), nil)),
		)
		if err != nil {
			t.Fatal(err)
		}
		return v
	}
	buy, sell := marksTrade(0, sideBuy, 1, 125_000_000), marksTrade(1, sideSell, 1, 70_000_000)
	buy.units, sell.units = wholeShares(25), wholeShares(12)
	marks, err := replayTrades([]trade{buy, sell}, AssetName{Decimals: 18})
	if err != nil || len(marks) != 2 {
		t.Fatalf("marks = %d, err %v, want 2", len(marks), err)
	}
	if !marks[0].Units.Equal(wholeShares(25)) || marks[0].PriceMicros.Uint64() != 5_000_000 {
		t.Fatalf("25 token buy mark = %+v, want 25 tokens at 5000000", marks[0])
	}
	if want := uint64(60_000_000); marks[1].CostOfSoldMicros.Uint64() != want {
		t.Fatalf("12 of 25 tokens removed %d of cost, want floor(125000000 × 12 ÷ 25) = %d",
			marks[1].CostOfSoldMicros.Uint64(), want)
	}
	oversell := marksTrade(1, sideSell, 1, 1)
	oversell.units = wholeShares(26)
	if _, err := replayTrades([]trade{buy, oversell}, AssetName{Decimals: 18}); errs.CodeOf(err) != errs.CodeInternal {
		t.Fatalf("selling 26 of 25 tokens = %v, want internal", err)
	}
}

func TestReplayTrades_fullSaleRemovesAllCost(t *testing.T) {
	t.Parallel()
	rapid.Check(t, func(t *rapid.T) {
		trades := drawTrades(t)
		var held, bought uint64
		for _, tr := range trades {
			units := tr.units.Big().Uint64()
			if tr.side == sideBuy {
				held += units
				bought += tr.usdc
			} else {
				held -= units
			}
		}
		if held > 0 {
			trades = append(trades, marksTrade(len(trades), sideSell, held, 1))
		}
		marks, err := replayTrades(trades, AssetName{Decimals: 8})
		if err != nil {
			t.Fatal(err)
		}
		var removed uint64
		for _, m := range marks {
			removed += m.CostOfSoldMicros.Uint64()
		}
		if removed != bought {
			t.Fatalf("cost removed = %d, want exactly the %d bought", removed, bought)
		}
	})
}

func modelCycles(trades []trade) []time.Time {
	var held uint64
	var cycle time.Time
	out := make([]time.Time, len(trades))
	for i, tr := range trades {
		units := tr.units.Big().Uint64()
		switch {
		case tr.side == sideSell:
			held -= units
		case held == 0:
			cycle, held = tr.at, units
		default:
			held += units
		}
		out[i] = cycle
	}
	return out
}

func TestReplayTrades_cycleStartIsFixedAtEachTrade(t *testing.T) {
	t.Parallel()
	rapid.Check(t, func(t *rapid.T) {
		trades := drawTrades(t)
		marks, err := replayTrades(trades, AssetName{Decimals: 8})
		if err != nil {
			t.Fatal(err)
		}
		prefix := rapid.IntRange(1, len(trades)).Draw(t, "prefix")
		shorter, err := replayTrades(trades[:prefix], AssetName{Decimals: 8})
		if err != nil {
			t.Fatal(err)
		}
		for i, want := range modelCycles(trades) {
			if !marks[i].CycleStart.Equal(want) {
				t.Fatalf("mark %d cycle start = %v, want %v", i, marks[i].CycleStart, want)
			}
		}
		for i := range shorter {
			if !reflect.DeepEqual(shorter[i], marks[i]) {
				t.Fatalf("mark %d changed when later trades were appended: %+v vs %+v", i, shorter[i], marks[i])
			}
		}
	})
}

func TestReplayTrades_returnsAnErrorNearMaxUint64InsteadOfWrapping(t *testing.T) {
	t.Parallel()
	rapid.Check(t, func(t *rapid.T) {
		first := rapid.Uint64Range(math.MaxUint64/2+1, math.MaxUint64).Draw(t, "first")
		second := rapid.Uint64Range(math.MaxUint64/2+1, math.MaxUint64).Draw(t, "second")
		_, err := replayTrades([]trade{
			marksTrade(0, sideBuy, 100_000_000, first), marksTrade(1, sideBuy, 100_000_000, second),
		}, AssetName{Decimals: 8})
		if errs.CodeOf(err) != errs.CodeInternal {
			t.Fatalf("summed cost past uint64: err = %v, want internal", err)
		}
		_, err = replayTrades([]trade{marksTrade(0, sideBuy, 1, first)}, AssetName{Decimals: 8})
		if errs.CodeOf(err) != errs.CodeInvalidInput {
			t.Fatalf("price past uint64: err = %v, want invalid input", err)
		}
	})
}

func TestReplayTrades_refusesHoldingsPastTwoToThe256(t *testing.T) {
	t.Parallel()
	top, err := money.UnitsFromBig(new(big.Int).Sub(new(big.Int).Lsh(big.NewInt(1), 256), big.NewInt(1)))
	if err != nil {
		t.Fatal(err)
	}
	first, second := marksTrade(0, sideBuy, 1, 1), marksTrade(1, sideBuy, 1, 1)
	first.units, second.units = top, top
	if _, err := replayTrades([]trade{first, second}, AssetName{Decimals: 0}); errs.CodeOf(err) != errs.CodeInternal {
		t.Fatalf("replay of two maximal buys = %v, want internal", err)
	}
}
