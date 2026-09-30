package domain

import (
	"log/slog"
	"math/big"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

type Flow struct {
	Amount money.SignedMicros
	At     time.Time
}

type DietzInput struct {
	T0, T1     time.Time
	Start, End money.Micros
	Flows      []Flow
}

func ModifiedDietz(in DietzInput) (money.SignedMicros, *Bps, error) {
	const op = "ranking.ModifiedDietz"
	if !in.T1.After(in.T0) {
		return money.SignedMicros{}, nil, errs.New(errs.CodeInvalidInput, op,
			slog.Time("t0", in.T0), slog.Time("t1", in.T1))
	}
	span := big.NewInt(in.T1.Sub(in.T0).Nanoseconds())
	start := new(big.Int).SetUint64(in.Start.Uint64())
	gain := new(big.Int).Sub(new(big.Int).SetUint64(in.End.Uint64()), start)
	base := new(big.Int).Mul(start, span)
	for _, f := range in.Flows {
		if f.At.Before(in.T0) || f.At.After(in.T1) {
			return money.SignedMicros{}, nil, errs.New(errs.CodeInvalidInput, op,
				slog.Time("flow_at", f.At), slog.Time("t0", in.T0), slog.Time("t1", in.T1))
		}
		amount := big.NewInt(f.Amount.Int64())
		gain.Sub(gain, amount)
		base.Add(base, new(big.Int).Mul(amount, big.NewInt(in.T1.Sub(f.At).Nanoseconds())))
	}
	g, err := signedMicros(op, gain)
	if err != nil {
		return money.SignedMicros{}, nil, err
	}
	if base.Sign() <= 0 {
		return g, nil, nil
	}
	ret, err := floorBps(op, gain.Mul(gain, span), base)
	if err != nil {
		return money.SignedMicros{}, nil, err
	}
	return g, &ret, nil
}
