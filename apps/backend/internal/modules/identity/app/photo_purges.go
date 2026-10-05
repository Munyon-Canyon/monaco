package app

import (
	"context"
	"log/slog"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/modules/identity/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
	"github.com/monaco/monaco/apps/backend/internal/platform/poller"
)

const PhotoPurgeBatch = 100

type PhotoPurges struct {
	db       sqlc.DBTX
	store    PhotoStore
	clock    clock.Clock
	interval time.Duration
}

func NewPhotoPurges(db sqlc.DBTX, store PhotoStore, c clock.Clock, interval time.Duration) *PhotoPurges {
	return &PhotoPurges{db: db, store: store, clock: c, interval: interval}
}

func (*PhotoPurges) Name() string { return "identity.photo_purges" }

func (p *PhotoPurges) Interval() time.Duration { return p.interval }

func (p *PhotoPurges) Tick(ctx context.Context) (poller.Report, error) {
	q := sqlc.New(p.db)
	due, err := q.PhotoPurgesDue(ctx, PhotoPurgeBatch)
	if err != nil {
		return poller.Report{}, err
	}
	report := poller.Report{Scanned: len(due)}
	for _, raw := range due {
		user := ids.UserIDFrom(raw)
		if err := p.store.DeleteAll(ctx, user); err != nil {
			observability.Degraded(ctx, observability.IdentityPhotoPurgeFailed, slog.String("user_id", user.String()))
			continue
		}
		n, err := q.MarkPhotoPurged(ctx, sqlc.MarkPhotoPurgedParams{At: p.clock.Now(), ID: raw})
		if err != nil {
			return report, err
		}
		report.Changed += int(n)
	}
	return report, nil
}
