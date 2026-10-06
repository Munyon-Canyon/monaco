package adapters

import (
	"context"
	"encoding/json"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
)

type Hints struct {
	Hints HintPublisher
}

func (h Hints) Snapshot(_ context.Context, tx db.Tx, e events.RankingSnapshotWritten, _ time.Time) error {
	payload, _ := json.Marshal(runHint{RunID: e.RunID, ComputedAt: e.ComputedAt})
	tx.AfterCommit(func(ctx context.Context) { h.Hints.PublishHint(ctx, leaderboardsUpdated, payload) })
	return nil
}
