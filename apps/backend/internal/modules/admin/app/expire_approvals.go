package app

import (
	"context"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/modules/admin/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/poller"
)

const ApprovalsExpireInterval = 5 * time.Minute

type ApprovalsExpirePoller struct {
	uow   *db.UnitOfWork
	clock clock.Clock
}

func NewApprovalsExpirePoller(uow *db.UnitOfWork, c clock.Clock) *ApprovalsExpirePoller {
	return &ApprovalsExpirePoller{uow: uow, clock: c}
}

func (*ApprovalsExpirePoller) Name() string { return "admin.approvals_expire" }

func (*ApprovalsExpirePoller) Interval() time.Duration { return ApprovalsExpireInterval }

func (p *ApprovalsExpirePoller) Tick(ctx context.Context) (poller.Report, error) {
	var expired int64
	err := p.uow.Do(ctx, func(ctx context.Context, tx db.Tx) (err error) {
		expired, err = sqlc.New(tx.Queries()).ExpireApprovals(ctx, p.clock.Now())
		return err
	})
	return poller.Report{Scanned: int(expired), Changed: int(expired)}, err
}
