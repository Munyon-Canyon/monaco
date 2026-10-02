package adapters

import (
	"context"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
)

type HintPublisher interface {
	PublishHint(ctx context.Context, key string, payload []byte)
}

type Hints struct {
	Publish HintPublisher
}

func (h Hints) Handle(_ context.Context, tx db.Tx, e events.CabalCreated, _ time.Time) error {
	tx.AfterCommit(func(ctx context.Context) {
		h.Publish.PublishHint(ctx, "user."+e.CreatorID.String()+".cabals", nil)
	})
	return nil
}
