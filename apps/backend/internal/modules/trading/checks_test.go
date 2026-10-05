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

func TestFeeHeadroom_roundsUpAndStopsAtTheMaximumFee(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		amount uint64
		bps    uint16
		max    uint64
		want   uint64
	}{
		{25_000_000, 0, 0, 0},
		{25_000_000, 100, math.MaxUint64, 250_000},
		{25_000_001, 100, math.MaxUint64, 250_001},
		{25_000_000, 100, 1_000, 1_000},
		{1, 1, math.MaxUint64, 1},
	} {
		if got := domain.FeeHeadroom(tc.amount, tc.bps, tc.max); got != tc.want {
			t.Errorf("FeeHeadroom(%d, %d, %d) = %d, want %d", tc.amount, tc.bps, tc.max, got, tc.want)
		}
	}
	rapid.Check(t, func(t *rapid.T) {
		amount, bps, maxFee := rapid.Uint64().Draw(t, "amount"), rapid.Uint16().Draw(t, "bps"),
			rapid.Uint64().Draw(t, "max")
		got := domain.FeeHeadroom(amount, bps, maxFee)
		fee := new(big.Int).Mul(new(big.Int).SetUint64(amount), big.NewInt(int64(bps)))
		covered := new(big.Int).Mul(new(big.Int).SetUint64(got), big.NewInt(10_000))
		if got > maxFee || (got < maxFee && covered.Cmp(fee) < 0) {
			t.Fatalf("FeeHeadroom(%d, %d, %d) = %d: below the fee and under the cap", amount, bps, maxFee, got)
		}
		if got > 0 && new(big.Int).Sub(covered, big.NewInt(10_000)).Cmp(fee) >= 0 {
			t.Fatalf("FeeHeadroom(%d, %d, %d) = %d: one unit more than the rounded-up fee", amount, bps, maxFee, got)
		}
	})
}
