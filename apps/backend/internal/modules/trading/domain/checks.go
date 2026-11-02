package domain

import (
	"math/big"

	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

const (
	DefaultSlippageBps int64 = 100
	MaxSlippageBps     int64 = 300
	bpsScale                 = 10_000
)

type Slippage struct{ bps int64 }

func SlippageOf(cabalBps int32) Slippage {
	switch bps := int64(cabalBps); {
	case bps <= 0:
		return Slippage{bps: DefaultSlippageBps}
	case bps > MaxSlippageBps:
		return Slippage{bps: MaxSlippageBps}
	default:
		return Slippage{bps: bps}
	}
}

func (s Slippage) Bps() int64 { return s.bps }

func (s Slippage) MinOut(quoteOut money.Units) money.Units {
	out := new(big.Int).Mul(quoteOut.Big(), big.NewInt(bpsScale-s.bps))
	floor, err := money.UnitsFromBig(out.Quo(out, big.NewInt(bpsScale)))
	if err != nil {
		return money.Units{}
	}
	return floor
}

func SlippageKeeping(out, floor money.Units) int64 {
	if out.IsZero() || out.Cmp(floor) <= 0 {
		return 0
	}
	gap := new(big.Int).Sub(out.Big(), floor.Big())
	gap.Mul(gap, big.NewInt(bpsScale))
	return gap.Quo(gap, out.Big()).Int64()
}
