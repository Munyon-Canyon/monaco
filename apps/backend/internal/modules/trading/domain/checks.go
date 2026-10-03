package domain

import "math/big"

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

func (s Slippage) MinOut(quoteOut uint64) uint64 {
	out := new(big.Int).Mul(new(big.Int).SetUint64(quoteOut), big.NewInt(bpsScale-s.bps))
	return out.Quo(out, big.NewInt(bpsScale)).Uint64()
}

func FeeHeadroom(amount uint64, feeBps uint16, maxFee uint64) uint64 {
	fee := new(big.Int).Mul(new(big.Int).SetUint64(amount), big.NewInt(int64(feeBps)))
	fee.Add(fee, big.NewInt(bpsScale-1))
	fee.Quo(fee, big.NewInt(bpsScale))
	if limit := new(big.Int).SetUint64(maxFee); fee.Cmp(limit) > 0 {
		return maxFee
	}
	return fee.Uint64()
}
