package app

import (
	"context"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/funding/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

type ArrivingDeposits interface {
	ArrivingMicros(ctx context.Context, user ids.UserID) (money.Micros, error)
}

type EVMDepositArrivals struct{ Reads sqlc.DBTX }

func (a EVMDepositArrivals) ArrivingMicros(ctx context.Context, user ids.UserID) (money.Micros, error) {
	const op = "funding.EVMDepositArrivals.ArrivingMicros"
	raw, err := sqlc.New(a.Reads).SumPendingEVMDepositForwards(ctx, user.UUID())
	if err != nil {
		return money.Micros{}, errs.Wrap(err, errs.CodeInternal, op)
	}
	arriving, err := money.ParseMicros(raw)
	if err != nil {
		return money.Micros{}, errs.Wrap(err, errs.CodeDecodeFailed, op)
	}
	return arriving, nil
}
