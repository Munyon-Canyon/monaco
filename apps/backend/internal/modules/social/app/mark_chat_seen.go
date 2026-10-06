package app

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

type MarkChatSeen struct {
	CabalID ids.CabalID
	Caller  ids.UserID
}

type MarkChatSeenHandler struct {
	d ChatDeps
}

func NewMarkChatSeenHandler(d ChatDeps) *MarkChatSeenHandler { return &MarkChatSeenHandler{d: d} }

func (h *MarkChatSeenHandler) Handle(ctx context.Context, cmd MarkChatSeen) (time.Time, error) {
	const op = "social.MarkChatSeen"
	if err := requireMember(ctx, h.d.Members, cmd.CabalID, cmd.Caller); err != nil {
		return time.Time{}, err
	}
	now := h.d.Clock.Now().UTC()
	var seen time.Time
	err := h.d.UoW.Do(ctx, func(ctx context.Context, tx db.Tx) error {
		var err error
		seen, err = sqlc.New(tx.Queries()).MarkChatSeen(ctx, sqlc.MarkChatSeenParams{
			CabalID: cmd.CabalID.UUID(), UserID: cmd.Caller.UUID(), Now: now,
		})
		return err
	})
	if err != nil {
		return time.Time{}, errs.Wrap(err, errs.CodeInternal, op)
	}
	h.publish(ctx, cmd, now)
	return seen.UTC(), nil
}

func (h *MarkChatSeenHandler) publish(ctx context.Context, cmd MarkChatSeen, now time.Time) {
	ctx = context.WithoutCancel(ctx)
	var claimed sqlc.ClaimSeenPublishRow
	err := h.d.UoW.Do(ctx, func(ctx context.Context, tx db.Tx) error {
		var err error
		claimed, err = sqlc.New(tx.Queries()).ClaimSeenPublish(ctx, sqlc.ClaimSeenPublishParams{
			CabalID: cmd.CabalID.UUID(), UserID: cmd.Caller.UUID(), Now: now,
		})
		return err
	})
	switch {
	case errors.Is(err, sql.ErrNoRows):
	case err != nil:
		failedPublish(ctx, uuid.Nil, EventSeenUpdated, err)
	default:
		h.d.Publish.send(ctx, cmd.CabalID, claimed.MessageID, EventSeenUpdated,
			SeenUpdated{MessageID: claimed.MessageID, Count: int(claimed.SeenCount)})
	}
}
