package adapters

import (
	"context"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/sqlc"
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
