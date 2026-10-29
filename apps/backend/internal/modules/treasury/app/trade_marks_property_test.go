package app

import (
	"math"
	"math/big"
	"testing"
	"time"

	"pgregory.net/rapid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
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
			units, usdc := new(big.Int).SetUint64(tr.units), new(big.Int).SetUint64(tr.usdc)
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

func TestReplayTrades_fullSaleRemovesAllCost(t *testing.T) {
	t.Parallel()
	rapid.Check(t, func(t *rapid.T) {
		trades := drawTrades(t)
		var held, bought uint64
		for _, tr := range trades {
			if tr.side == sideBuy {
				held += tr.units
				bought += tr.usdc
			} else {
				held -= tr.units
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
		switch {
		case tr.side == sideSell:
			held -= tr.units
		case held == 0:
			cycle, held = tr.at, tr.units
		default:
			held += tr.units
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
			if shorter[i] != marks[i] {
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
