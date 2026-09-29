package adapters

import (
	"context"

	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/system/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
)

type Hints interface {
	PublishHint(ctx context.Context, key string, payload []byte)
}

type Echo struct {
	Clock clock.Clock
	Hints Hints
}

func (h Echo) Handle(ctx context.Context, tx db.Tx, e events.SystemPinged) error {
	q := sqlc.New(tx.Queries())
	if err := q.InsertPing(ctx, sqlc.InsertPingParams{ID: e.PingID, UserID: e.UserID, Note: e.Note}); err != nil {
		return err
	}
	echoed, err := q.EchoPing(ctx, sqlc.EchoPingParams{ID: e.PingID, EchoedAt: h.Clock.Now()})
	if err != nil || echoed == 0 {
		return err
	}
	tx.AfterCommit(func(ctx context.Context) {
		h.Hints.PublishHint(ctx, "user."+e.UserID.String()+".ping_echoed", nil)
	})
	return nil
}
