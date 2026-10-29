package domain

import (
	"log/slog"
	"math/big"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

func RangeReturnOf(points []PnLPoint) (money.SignedMicros, Bps, error) {
	if len(points) < 2 {
		return money.SignedMicros{}, 0, nil
	}
	return RangeReturn(points[0], points[len(points)-1])
}

func RangeReturn(first, last PnLPoint) (money.SignedMicros, Bps, error) {
	const op = "ranking.RangeReturn"
	gain := new(big.Int).Sub(big.NewInt(last.PnL.Int64()), big.NewInt(first.PnL.Int64()))
	pnl, err := signedMicros(op, gain)
	if err != nil {
		return money.SignedMicros{}, 0, err
	}
	deposited := new(big.Int).Sub(netOf(last), netOf(first))
	base := new(big.Int).Add(new(big.Int).SetUint64(first.Equity.Uint64()), deposited)
	if base.Sign() <= 0 {
		return pnl, 0, nil
	}
	q := new(big.Int).Mul(gain, big.NewInt(bpsScale))
	q.Quo(q, base)
	if !q.IsInt64() {
		return money.SignedMicros{}, 0, errs.New(errs.CodeInvalidInput, op, slog.String("return_bps", q.String()))
	}
	return pnl, Bps(q.Int64()), nil
}

func netOf(p PnLPoint) *big.Int {
	return new(big.Int).Sub(new(big.Int).SetUint64(p.Equity.Uint64()), big.NewInt(p.PnL.Int64()))
}
