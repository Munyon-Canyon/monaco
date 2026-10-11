package app

import (
	"context"
	"log/slog"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/funding/port"
	"github.com/monaco/monaco/apps/backend/internal/modules/funding/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

type DepositFees struct{ Reads sqlc.DBTX }

var _ port.DepositFees = DepositFees{}

func (d DepositFees) EstimatedDepositFees(ctx context.Context) (map[chain.Network]money.Micros, error) {
	const op = "funding.DepositFees.EstimatedDepositFees"
	rows, err := sqlc.New(d.Reads).LatestEVMDepositForwardFees(ctx)
	if err != nil {
		return nil, errs.Wrap(err, errs.CodeInternal, op)
	}
	fees := make(map[chain.Network]money.Micros, len(rows))
	for _, row := range rows {
		fee, err := money.ParseMicros(row.FeeMicros)
		if err != nil {
			return nil, errs.Wrap(err, errs.CodeDecodeFailed, op, slog.String("network", row.Network))
		}
		fees[chain.Network(row.Network)] = fee
	}
	return fees, nil
}
