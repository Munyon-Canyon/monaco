package domain

import (
	"math/big"

	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

const minRangeBase = 10_000

func RangeReturnOf(points []PnLPoint) (money.SignedMicros, Bps, error) {
	if len(points) < 2 {
		return money.SignedMicros{}, 0, nil
	}
	deposited := new(big.Int)
	for i := 1; i < len(points); i++ {
		if step := new(big.Int).Sub(netOf(points[i]), netOf(points[i-1])); step.Sign() > 0 {
			deposited.Add(deposited, step)
		}
	}
	return rangeReturn("ranking.RangeReturnOf", points[0], points[len(points)-1], deposited)
}

func RangeReturn(first, last PnLPoint) (money.SignedMicros, Bps, error) {
	deposited := new(big.Int).Sub(netOf(last), netOf(first))
	if deposited.Sign() < 0 {
		deposited.SetInt64(0)
	}
	return rangeReturn("ranking.RangeReturn", first, last, deposited)
}

func rangeReturn(op string, first, last PnLPoint, deposited *big.Int) (money.SignedMicros, Bps, error) {
	gain := new(big.Int).Sub(big.NewInt(last.PnL.Int64()), big.NewInt(first.PnL.Int64()))
	pnl, err := signedMicros(op, gain)
	if err != nil {
		return money.SignedMicros{}, 0, err
	}
	base := new(big.Int).Add(new(big.Int).SetUint64(first.Equity.Uint64()), deposited)
	if base.Cmp(big.NewInt(minRangeBase)) < 0 {
		return pnl, 0, nil
	}
	q := new(big.Int).Mul(gain, big.NewInt(bpsScale))
	q.Div(q, base)
	return pnl, Bps(q.Int64()), nil
}

func netOf(p PnLPoint) *big.Int {
	return new(big.Int).Sub(new(big.Int).SetUint64(p.Equity.Uint64()), big.NewInt(p.PnL.Int64()))
}
