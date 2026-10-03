package app

import (
	"context"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/modules/funding/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/funding/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/poller"
)

const (
	OnrampExpiryInterval = 60 * time.Second
	OnrampOpenedTTL      = 2 * time.Hour
	OnrampExpiryBatch    = 100
)

type OnrampExpiryPoller struct {
	uow   *db.UnitOfWork
	clock clock.Clock
}

func NewOnrampExpiryPoller(uow *db.UnitOfWork, c clock.Clock) *OnrampExpiryPoller {
	return &OnrampExpiryPoller{uow: uow, clock: c}
}

func (*OnrampExpiryPoller) Name() string { return "funding.onramp-expiry" }

func (*OnrampExpiryPoller) Interval() time.Duration { return OnrampExpiryInterval }

func (p *OnrampExpiryPoller) Tick(ctx context.Context) (poller.Report, error) {
	now := p.clock.Now()
	var report poller.Report
	err := p.uow.Do(ctx, func(ctx context.Context, tx db.Tx) error {
		rows, err := sqlc.New(tx.Queries()).ExpireOnrampSessions(ctx, sqlc.ExpireOnrampSessionsParams{
			Now: now, OpenedBefore: now.Add(-OnrampOpenedTTL), Batch: OnrampExpiryBatch,
		})
		if err != nil {
			return err
		}
		for _, row := range rows {
			suggested, err := suggestedAmount(row.SuggestedAmountMicros)
			if err != nil {
				return err
			}
			if err := tx.Events.Append(ctx, statusChanged(row.ID, row.UserID, domain.OnrampStatus(row.FromStatus),
				domain.OnrampExpired, suggested, nil)); err != nil {
				return err
			}
		}
		report = poller.Report{Scanned: len(rows), Changed: len(rows)}
		return nil
	})
	if err != nil {
		return poller.Report{}, err
	}
	return report, nil
}
