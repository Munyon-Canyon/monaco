package trading_test

import (
	"math"
	"math/big"
	"testing"

	"pgregory.net/rapid"

	"github.com/monaco/monaco/apps/backend/internal/modules/trading/domain"
)

func TestSlippageOf_defaultsUnsetAndCapsAtThePlatformMaximum(t *testing.T) {
	t.Parallel()
	for raw, want := range map[int32]int64{-5: 100, 0: 100, 1: 1, 250: 250, 300: 300, 301: 300, math.MaxInt32: 300} {
		if got := domain.SlippageOf(raw).Bps(); got != want {
			t.Errorf("SlippageOf(%d) = %d, want %d", raw, got, want)
		}
	}
}

func TestSlippageMinOut_isTheFlooredToleranceAndNeverAboveTheQuote(t *testing.T) {
	t.Parallel()
	if got := domain.SlippageOf(100).MinOut(21_000_000); got != 20_790_000 {
		t.Fatalf("MinOut = %d, want 20_790_000", got)
	}
	rapid.Check(t, func(t *rapid.T) {
		quote := rapid.Uint64().Draw(t, "quote")
		s := domain.SlippageOf(rapid.Int32().Draw(t, "bps"))
		got := s.MinOut(quote)
		scaled := new(big.Int).Mul(new(big.Int).SetUint64(quote), big.NewInt(10_000-s.Bps()))
		low := new(big.Int).Mul(new(big.Int).SetUint64(got), big.NewInt(10_000))
		high := new(big.Int).Add(low, big.NewInt(10_000))
		if got > quote || low.Cmp(scaled) > 0 || high.Cmp(scaled) <= 0 {
			t.Fatalf("MinOut(%d) at %d bps = %d, want floor(quote × (10000 − bps) / 10000)", quote, s.Bps(), got)
		}
	})
}

func TestSlippageKeeping_isTheWidestToleranceWhoseMinOutStaysAtTheFloor(t *testing.T) {
	t.Parallel()
	if got := domain.SlippageKeeping(104_475_000, 103_950_000); got != 50 {
		t.Fatalf("SlippageKeeping = %d, want 50", got)
	}
	minOut := func(out uint64, bps int64) *big.Int {
		n := new(big.Int).Mul(new(big.Int).SetUint64(out), big.NewInt(10_000-bps))
		return n.Quo(n, big.NewInt(10_000))
	}
	rapid.Check(t, func(t *rapid.T) {
		out := rapid.Uint64Min(1).Draw(t, "out")
		floor := rapid.Uint64Max(out).Draw(t, "floor")
		bps := domain.SlippageKeeping(out, floor)
		f := new(big.Int).SetUint64(floor)
		widest := bps == 10_000 || minOut(out, bps+1).Cmp(f) < 0
		if bps < 0 || bps > 10_000 || minOut(out, bps).Cmp(f) < 0 || !widest {
			t.Fatalf("SlippageKeeping(%d, %d) = %d, want the widest bps whose min out is at least the floor",
				out, floor, bps)
		}
	})
}
