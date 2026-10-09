package adapters

import (
	"context"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

type FundOutflows struct{ reads sqlc.DBTX }

func NewFundOutflows(reads sqlc.DBTX) FundOutflows { return FundOutflows{reads: reads} }

func (o FundOutflows) InFlightMicros(ctx context.Context, user ids.UserID) (money.Micros, error) {
	raw, err := sqlc.New(o.reads).InFlightFundMicros(ctx, user.UUID())
	if err != nil {
		return money.Micros{}, err
	}
	return money.ParseMicros(raw)
}

func (o FundOutflows) LastChange(ctx context.Context, user ids.UserID) (time.Time, error) {
	return sqlc.New(o.reads).LastFundChange(ctx, user.UUID())
}

func (o FundOutflows) Submitted(ctx context.Context, user ids.UserID) (map[chain.Signature]money.Micros, error) {
	rows, err := sqlc.New(o.reads).UserSubmittedFunds(ctx, user.UUID())
	if err != nil {
		return nil, err
	}
	out := make(map[chain.Signature]money.Micros, len(rows))
	for _, row := range rows {
		amount, err := money.ParseMicros(row.AmountMicros)
		if err != nil {
			return nil, err
		}
		out[chain.Signature(row.TxSignature)] = amount
	}
	return out, nil
}
