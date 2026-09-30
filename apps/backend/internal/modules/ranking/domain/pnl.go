package domain

import (
	"log/slog"
	"math/big"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

const bpsScale = 10_000

func Lifetime(equity money.Micros, netContributed money.SignedMicros) (money.SignedMicros, *Bps, error) {
	net := big.NewInt(netContributed.Int64())
	gain := new(big.Int).Sub(new(big.Int).SetUint64(equity.Uint64()), net)
	pnl, err := signedMicros("ranking.Lifetime", gain)
	if err != nil {
		return money.SignedMicros{}, nil, err
	}
	if net.Sign() <= 0 {
		return pnl, nil, nil
	}
	ret, err := floorBps("ranking.Lifetime", gain, net)
	if err != nil {
		return money.SignedMicros{}, nil, err
	}
	return pnl, &ret, nil
}

func signedMicros(op string, v *big.Int) (money.SignedMicros, error) {
	if !v.IsInt64() {
		return money.SignedMicros{}, errs.New(errs.CodeInvalidInput, op, slog.String("value", v.String()))
	}
	return money.SignedMicrosFromInt64(v.Int64()), nil
}

func floorBps(op string, gain, base *big.Int) (Bps, error) {
	q := new(big.Int).Mul(gain, big.NewInt(bpsScale))
	q.Div(q, base)
	if !q.IsInt64() {
		return 0, errs.New(errs.CodeInvalidInput, op, slog.String("return_bps", q.String()))
	}
	return Bps(q.Int64()), nil
}
