package domain

import (
	"math"
	"math/big"

	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

type ThresholdBps int64

const (
	ThresholdUp5    ThresholdBps = 500
	ThresholdUp10   ThresholdBps = 1000
	ThresholdDown5  ThresholdBps = -500
	ThresholdDown10 ThresholdBps = -1000
)

const bpsScale = 10_000

func Crossed(prev, mark money.Micros) []ThresholdBps {
	bps := basisPoints(prev, mark)
	if bps == nil {
		return nil
	}
	var out []ThresholdBps
	for _, th := range []ThresholdBps{ThresholdUp5, ThresholdUp10, ThresholdDown5, ThresholdDown10} {
		if reached(bps, th) {
			out = append(out, th)
		}
	}
	return out
}

func ChangeBps(prev, mark money.Micros) int64 {
	bps := basisPoints(prev, mark)
	if bps == nil {
		return 0
	}
	if !bps.IsInt64() {
		return math.MaxInt64
	}
	return bps.Int64()
}

func basisPoints(prev, mark money.Micros) *big.Int {
	if prev.IsZero() {
		return nil
	}
	delta := new(big.Int).Sub(new(big.Int).SetUint64(mark.Uint64()), new(big.Int).SetUint64(prev.Uint64()))
	delta.Mul(delta, big.NewInt(bpsScale))
	return delta.Quo(delta, new(big.Int).SetUint64(prev.Uint64()))
}

func reached(bps *big.Int, th ThresholdBps) bool {
	bound := big.NewInt(int64(th))
	if th > 0 {
		return bps.Cmp(bound) >= 0
	}
	return bps.Cmp(bound) <= 0
}
