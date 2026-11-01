package app

import (
	"context"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/funding/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

var _ money.StuckReader = StuckMoney{}

type StuckMoney struct {
	Reads sqlc.DBTX
	Clock clock.Clock
}

func (s StuckMoney) List(
	ctx context.Context,
	kind string,
	olderThan time.Duration,
	limit int,
) ([]money.StuckRow, error) {
	now := s.Clock.Now()
	rows, err := sqlc.New(s.Reads).ListStuckFundingMoney(ctx, sqlc.ListStuckFundingMoneyParams{
		Now: now, CreatedBefore: now.Add(-olderThan), Kind: kind, RowLimit: int64(limit),
	})
	if err != nil {
		return nil, errs.Wrap(err, errs.CodeDBUnavailable, "funding.StuckMoney.List")
	}
	out := make([]money.StuckRow, len(rows))
	for i, r := range rows {
		out[i] = money.StuckRow{
			Kind: r.Kind, Ref: r.Ref, UserID: r.UserID, CabalID: r.CabalID, Status: r.Status, Since: r.Since,
		}
	}
	return out, nil
}

func (s StuckMoney) Summary(ctx context.Context, olderThan time.Duration) ([]money.StuckSummary, error) {
	now := s.Clock.Now()
	rows, err := sqlc.New(s.Reads).SummarizeStuckFundingMoney(ctx, sqlc.SummarizeStuckFundingMoneyParams{
		Now: now, CreatedBefore: now.Add(-olderThan),
	})
	if err != nil {
		return nil, errs.Wrap(err, errs.CodeDBUnavailable, "funding.StuckMoney.Summary")
	}
	out := make([]money.StuckSummary, len(rows))
	for i, r := range rows {
		out[i] = money.StuckSummary{Kind: r.Kind, Count: int(r.Total), Oldest: r.Oldest}
	}
	return out, nil
}
