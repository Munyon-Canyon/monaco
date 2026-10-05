package app

import (
	"context"
	"log/slog"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/modules/ranking/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
	"github.com/monaco/monaco/apps/backend/internal/platform/poller"
)

const (
	SnapshotThinningInterval = 30 * 24 * time.Hour
	SnapshotFullRetention    = 7 * 24 * time.Hour
)

type ThinSnapshots struct {
	db    sqlc.DBTX
	clock clock.Clock
}

func NewThinSnapshots(db sqlc.DBTX, c clock.Clock) *ThinSnapshots {
	return &ThinSnapshots{db: db, clock: c}
}

func (*ThinSnapshots) Name() string { return "ranking.snapshot_thinning" }

func (*ThinSnapshots) Interval() time.Duration { return SnapshotThinningInterval }

func (t *ThinSnapshots) Tick(ctx context.Context) (poller.Report, error) {
	before := t.clock.Now().Add(-SnapshotFullRetention)
	deleted, err := sqlc.New(t.db).ThinCabalValueSnapshots(ctx, before)
	if err != nil {
		return poller.Report{}, err
	}
	observability.Info(ctx, observability.RankingSnapshotsThinned,
		slog.Int64("deleted", deleted), slog.Time("before", before))
	return poller.Report{Scanned: int(deleted), Changed: int(deleted)}, nil
}
