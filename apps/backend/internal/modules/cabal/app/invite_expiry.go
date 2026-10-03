package app

import (
	"context"
	"errors"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/cabal/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/poller"
)

const (
	InviteExpiryInterval = 5 * time.Minute
	InviteExpiryBatch    = 100

	inviteExpiryOp = "cabal.InviteExpiry"
)

type InviteExpiryPoller struct {
	uow   *db.UnitOfWork
	reads sqlc.DBTX
	clock clock.Clock
}

func NewInviteExpiryPoller(uow *db.UnitOfWork, reads sqlc.DBTX, c clock.Clock) *InviteExpiryPoller {
	return &InviteExpiryPoller{uow: uow, reads: reads, clock: c}
}

func (*InviteExpiryPoller) Name() string { return "cabal.invite_expiry" }

func (*InviteExpiryPoller) Interval() time.Duration { return InviteExpiryInterval }

func (p *InviteExpiryPoller) Tick(ctx context.Context) (poller.Report, error) {
	now := p.clock.Now()
	due, err := sqlc.New(p.reads).ListDueInvites(ctx, sqlc.ListDueInvitesParams{Now: now, MaxRows: InviteExpiryBatch})
	if err != nil {
		return poller.Report{}, errs.Wrap(err, errs.CodeInternal, inviteExpiryOp)
	}
	report := poller.Report{Scanned: len(due)}
	var failed []error
	for _, row := range due {
		var expired bool
		err := p.uow.Do(ctx, func(ctx context.Context, tx db.Tx) error {
			var expireErr error
			expired, expireErr = expireInvite(ctx, tx, dueInvite{id: row.ID, cabalID: row.CabalID, userID: row.UserID},
				now, inviteExpiryOp)
			return expireErr
		})
		failed = append(failed, err)
		if err == nil && expired {
			report.Changed++
		}
	}
	return report, errors.Join(failed...)
}
