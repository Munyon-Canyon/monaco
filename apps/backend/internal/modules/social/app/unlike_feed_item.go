package app

import (
	"context"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
)

type UnlikeFeedItem = LikeFeedItem

type UnlikeFeedItemHandler struct{ d LikeDeps }

func NewUnlikeFeedItemHandler(d LikeDeps) *UnlikeFeedItemHandler { return &UnlikeFeedItemHandler{d: d} }

func (h *UnlikeFeedItemHandler) Handle(ctx context.Context, cmd UnlikeFeedItem) error {
	const op = "social.UnlikeFeedItem"
	if err := requireVisibleItem(ctx, h.d.Reads, cmd.Viewer, cmd.FeedObjectID); err != nil {
		return err
	}
	return h.d.UoW.Do(ctx, func(ctx context.Context, tx db.Tx) error {
		rows, err := sqlc.New(tx.Queries()).UnlikeFeedItem(ctx, sqlc.UnlikeFeedItemParams{
			FeedObjectID: cmd.FeedObjectID, UserID: cmd.Viewer.UUID(),
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
