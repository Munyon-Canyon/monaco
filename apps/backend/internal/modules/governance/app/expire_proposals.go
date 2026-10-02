package app

import (
	"context"
	"errors"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/governance/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/governance/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/poller"
)

const (
	ExpiryInterval = 30 * time.Second
	ExpiryBatch    = 100
)

type ExpiryPoller struct {
	uow   *db.UnitOfWork
	reads sqlc.DBTX
	clock clock.Clock
}

func NewExpiryPoller(uow *db.UnitOfWork, reads sqlc.DBTX, c clock.Clock) *ExpiryPoller {
	return &ExpiryPoller{uow: uow, reads: reads, clock: c}
}

func (*ExpiryPoller) Name() string { return "governance.proposal_expiry" }

func (*ExpiryPoller) Interval() time.Duration { return ExpiryInterval }

func (p *ExpiryPoller) Tick(ctx context.Context) (poller.Report, error) {
	now := p.clock.Now()
	due, err := sqlc.New(p.reads).DueForExpiry(ctx, sqlc.DueForExpiryParams{Now: now, Batch: ExpiryBatch})
	if err != nil {
		return poller.Report{}, err
	}
	report := poller.Report{Scanned: len(due)}
	var failed []error
	for _, row := range due {
		expired, err := p.expire(ctx, row, now)
		failed = append(failed, err)
		if expired {
			report.Changed++
		}
	}
	return report, errors.Join(failed...)
}

func (p *ExpiryPoller) expire(ctx context.Context, row sqlc.DueForExpiryRow, now time.Time) (bool, error) {
	var expired bool
	err := p.uow.Do(ctx, func(ctx context.Context, tx db.Tx) error {
		n, err := sqlc.New(tx.Queries()).Transition(ctx, sqlc.TransitionParams{
			ID: row.ID, FromStatus: string(domain.StatusOpen), ToStatus: string(domain.StatusExpired), At: now,
		})
		if err != nil || n == 0 {
			return err
		}
		expired = true
		return tx.Events.Append(ctx, events.ProposalExpired{V: 1, ProposalID: row.ID, CabalID: row.CabalID})
	})
	if err != nil {
		return false, err
	}
	return expired, nil
}
