package app

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/notify/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/notify/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

type ReadItem struct {
	User ids.UserID
	Key  string
}

type ClearItem struct {
	User ids.UserID
	Key  string
}

type KeepItem struct {
	User ids.UserID
	Key  string
}

type CatchUpWrites struct {
	uow   *db.UnitOfWork
	reads *ReadCatchUp
	hints Hints
	clock clock.Clock
}

func NewCatchUpWrites(uow *db.UnitOfWork, reads *ReadCatchUp, hints Hints, clk clock.Clock) *CatchUpWrites {
	return &CatchUpWrites{uow: uow, reads: reads, hints: hints, clock: clk}
}

func (h *CatchUpWrites) Read(ctx context.Context, cmd ReadItem) error {
	return h.write(ctx, cmd.User, cmd.Key, "notify.ReadItem", func(q *sqlc.Queries, at time.Time, _ []byte) error {
		return q.UpsertRead(ctx, sqlc.UpsertReadParams{UserID: cmd.User.UUID(), ItemKey: cmd.Key, At: at})
	}, false)
}

func (h *CatchUpWrites) Keep(ctx context.Context, cmd KeepItem) error {
	return h.write(ctx, cmd.User, cmd.Key, "notify.KeepItem", func(q *sqlc.Queries, at time.Time, _ []byte) error {
		return q.UpsertKeep(ctx, sqlc.UpsertKeepParams{UserID: cmd.User.UUID(), ItemKey: cmd.Key, At: at})
	}, false)
}

func (h *CatchUpWrites) Clear(ctx context.Context, cmd ClearItem) error {
	return h.write(ctx, cmd.User, cmd.Key, "notify.ClearItem", func(q *sqlc.Queries, at time.Time, snap []byte) error {
		return q.UpsertClear(ctx, sqlc.UpsertClearParams{
			UserID: cmd.User.UUID(), ItemKey: cmd.Key, Reason: domain.ClearedManual, Snapshot: snap, At: at,
		})
	}, true)
}

func (h *CatchUpWrites) write(
	ctx context.Context,
	user ids.UserID,
	key, op string,
	apply func(q *sqlc.Queries, at time.Time, snapshot []byte) error,
	snapshot bool,
) error {
	items, err := h.reads.Render(ctx, user, []string{key})
	if err != nil {
		return err
	}
	item, ok := items[key]
	if !ok {
		return errs.New(errs.CodeNotFound, op, slog.String("item_key", key))
	}
	var snap []byte
	if snapshot {
		snap, _ = json.Marshal(item)
	}
	return h.uow.Do(ctx, func(_ context.Context, tx db.Tx) error {
		if err := apply(sqlc.New(tx.Queries()), h.clock.Now(), snap); err != nil {
			return errs.Wrap(err, errs.CodeDBUnavailable, op)
		}
		hintCatchUp(tx, h.hints, user)
		return nil
	})
}

func hintCatchUp(tx db.Tx, hints Hints, users ...ids.UserID) {
	if len(users) == 0 {
		return
	}
	tx.AfterCommit(func(ctx context.Context) {
		for _, u := range users {
			hints.PublishHint(ctx, events.UserCatchUpChangedHint(u), nil)
		}
	})
}
