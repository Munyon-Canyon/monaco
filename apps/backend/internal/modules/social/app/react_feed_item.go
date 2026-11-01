package app

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

type ReactFeedItem struct {
	FeedObjectID uuid.UUID
	Viewer       ids.UserID
	Emoji        string
}

type UnreactFeedItem = ReactFeedItem

type ReactionSummary struct {
	Reactions []Reaction
	Total     int32
}

type ReactionDeps struct {
	UoW     *db.UnitOfWork
	Reads   sqlc.DBTX
	Members Members
	Clock   clock.Clock
	Hints   Hints
}

type ReactFeedItemHandler struct{ d ReactionDeps }

func NewReactFeedItemHandler(d ReactionDeps) *ReactFeedItemHandler {
	return &ReactFeedItemHandler{d: d}
}

func (h *ReactFeedItemHandler) Handle(ctx context.Context, cmd ReactFeedItem) (ReactionSummary, error) {
	const op = "social.ReactFeedItem"
	item, err := CanReact(ctx, h.d.Reads, h.d.Members, cmd.Viewer, cmd.FeedObjectID)
	if err != nil {
		return ReactionSummary{}, err
	}
	emoji, err := domain.ParseEmoji(cmd.Emoji)
	if err != nil {
		return ReactionSummary{}, err
	}
	err = h.d.UoW.Do(ctx, func(ctx context.Context, tx db.Tx) error {
		q := sqlc.New(tx.Queries())
		_, err := q.AddReaction(ctx, sqlc.AddReactionParams{
			FeedObjectID: cmd.FeedObjectID, UserID: cmd.Viewer.UUID(), Emoji: emoji.String(),
			CreatedAt: h.d.Clock.Now().UTC(),
		})
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		if err != nil {
			return errs.Wrap(err, errs.CodeInternal, op)
		}
		held, err := q.CountUserReactionsOnItem(ctx, sqlc.CountUserReactionsOnItemParams{
			FeedObjectID: cmd.FeedObjectID, UserID: cmd.Viewer.UUID(),
		})
		if err != nil {
			return errs.Wrap(err, errs.CodeInternal, op)
		}
		if held > domain.MaxReactionsPerUser {
			return errs.New(errs.CodeInvalidInput, op, slog.String("reason", "reaction_limit"))
		}
		tx.AfterCommit(func(ctx context.Context) { h.d.Hints.PublishHint(ctx, FeedHint, nil) })
		return tx.Events.Append(ctx, events.ReactionAdded{
			V: 1, FeedObjectID: cmd.FeedObjectID, UserID: cmd.Viewer.UUID(), Emoji: emoji.String(),
			ItemActorID: parentOf(item.ActorID.UUID()), CabalID: parentOf(item.CabalID.UUID()),
		})
	})
	if err != nil {
		return ReactionSummary{}, err
	}
	return reactionSummaryOf(ctx, h.d.Reads, cmd.Viewer, cmd.FeedObjectID)
}

type UnreactFeedItemHandler struct{ d ReactionDeps }

func NewUnreactFeedItemHandler(d ReactionDeps) *UnreactFeedItemHandler {
	return &UnreactFeedItemHandler{d: d}
}

func (h *UnreactFeedItemHandler) Handle(ctx context.Context, cmd UnreactFeedItem) (ReactionSummary, error) {
	const op = "social.UnreactFeedItem"
	item, err := requireVisibleItem(ctx, h.d.Reads, cmd.Viewer, cmd.FeedObjectID)
	if err != nil {
		return ReactionSummary{}, err
	}
	emoji, err := domain.ParseEmoji(cmd.Emoji)
	if err != nil {
		return ReactionSummary{}, err
	}
	err = h.d.UoW.Do(ctx, func(ctx context.Context, tx db.Tx) error {
		q := sqlc.New(tx.Queries())
		rows, err := q.RemoveReaction(ctx, sqlc.RemoveReactionParams{
			FeedObjectID: cmd.FeedObjectID, UserID: cmd.Viewer.UUID(), Emoji: emoji.String(),
		})
		if err != nil {
			return errs.Wrap(err, errs.CodeInternal, op)
		}
		if rows == 0 {
			return nil
		}
		tx.AfterCommit(func(ctx context.Context) { h.d.Hints.PublishHint(ctx, FeedHint, nil) })
		return tx.Events.Append(ctx, events.ReactionRemoved{
			V: 1, FeedObjectID: cmd.FeedObjectID, UserID: cmd.Viewer.UUID(), Emoji: emoji.String(),
			ItemActorID: parentOf(item.ActorID.UUID()), CabalID: parentOf(item.CabalID.UUID()),
		})
	})
	if err != nil {
		return ReactionSummary{}, err
	}
	return reactionSummaryOf(ctx, h.d.Reads, cmd.Viewer, cmd.FeedObjectID)
}

func reactionSummaryOf(
	ctx context.Context, reads sqlc.DBTX, viewer ids.UserID, id uuid.UUID,
) (ReactionSummary, error) {
	items := []FeedItem{{ID: id}}
	if err := attachReactions(ctx, sqlc.New(reads), viewer, items); err != nil {
		return ReactionSummary{}, errs.Wrap(err, errs.CodeOf(err), "social.reactionSummaryOf")
	}
	return ReactionSummary{Reactions: items[0].Reactions, Total: items[0].ReactionCount}, nil
}
