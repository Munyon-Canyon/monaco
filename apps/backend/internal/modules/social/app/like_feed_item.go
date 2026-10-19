package app

import (
	"context"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

type LikeFeedItem struct {
	FeedObjectID uuid.UUID
	Viewer       ids.UserID
}

type LikeDeps struct {
	UoW   *db.UnitOfWork
	Reads sqlc.DBTX
	Clock clock.Clock
	Hints Hints
}

type LikeFeedItemHandler struct{ d LikeDeps }

func NewLikeFeedItemHandler(d LikeDeps) *LikeFeedItemHandler { return &LikeFeedItemHandler{d: d} }

func (h *LikeFeedItemHandler) Handle(ctx context.Context, cmd LikeFeedItem) error {
	const op = "social.LikeFeedItem"
	if err := requireVisibleItem(ctx, h.d.Reads, cmd.Viewer, cmd.FeedObjectID); err != nil {
		return err
	}
	return h.d.UoW.Do(ctx, func(ctx context.Context, tx db.Tx) error {
		rows, err := sqlc.New(tx.Queries()).LikeFeedItem(ctx, sqlc.LikeFeedItemParams{
			FeedObjectID: cmd.FeedObjectID, UserID: cmd.Viewer.UUID(), CreatedAt: h.d.Clock.Now().UTC(),
		})
		if err != nil {
			return errs.Wrap(err, errs.CodeInternal, op)
		}
		if rows > 0 {
			tx.AfterCommit(func(ctx context.Context) { h.d.Hints.PublishHint(ctx, FeedHint, nil) })
		}
		return nil
	})
}

func requireVisibleItem(ctx context.Context, reads sqlc.DBTX, viewer ids.UserID, id uuid.UUID) error {
	view, err := GetFeedItem(ctx, reads, viewer, id, FeedFilter{})
	if err != nil {
		return err
	}
	if !view.Visible {
		return errs.New(errs.CodeFeedItemNotFound, "social.requireVisibleItem")
	}
	return nil
}
