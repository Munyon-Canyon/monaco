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
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

type ReactComment struct {
	CommentID uuid.UUID
	Viewer    ids.UserID
	Emoji     string
}

type UnreactComment = ReactComment

type ReactCommentHandler struct{ d ReactionDeps }

func NewReactCommentHandler(d ReactionDeps) *ReactCommentHandler { return &ReactCommentHandler{d: d} }

func (h *ReactCommentHandler) Handle(ctx context.Context, cmd ReactComment) (ReactionSummary, error) {
	const op = "social.ReactComment"
	ref, err := liveComment(ctx, h.d.Reads, cmd.CommentID, op)
	if err != nil {
		return ReactionSummary{}, err
	}
	if err := refuseSystemReply(ref.AuthorKind, op, ref.ID); err != nil {
		return ReactionSummary{}, err
	}
	item, err := CanReact(ctx, h.d.Reads, h.d.Members, cmd.Viewer, ref.FeedObjectID)
	if err != nil {
		return ReactionSummary{}, err
	}
	emoji, err := domain.ParseEmoji(cmd.Emoji)
	if err != nil {
		return ReactionSummary{}, err
	}
	err = h.d.UoW.Do(ctx, func(ctx context.Context, tx db.Tx) error {
		q := sqlc.New(tx.Queries())
		_, err := q.AddCommentReaction(ctx, sqlc.AddCommentReactionParams{
			CommentID: cmd.CommentID, FeedObjectID: ref.FeedObjectID, UserID: cmd.Viewer.UUID(),
			Emoji: emoji.String(), CreatedAt: h.d.Clock.Now().UTC(),
		})
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		if err != nil {
			return errs.Wrap(err, errs.CodeInternal, op)
		}
		held, err := q.CountUserCommentReactions(ctx, sqlc.CountUserCommentReactionsParams{
			CommentID: cmd.CommentID, UserID: cmd.Viewer.UUID(),
		})
		if err != nil {
			return errs.Wrap(err, errs.CodeInternal, op)
		}
		if held > domain.MaxReactionsPerUser {
			return errs.New(errs.CodeInvalidInput, op, slog.String("reason", "reaction_limit"))
		}
		tx.AfterCommit(func(ctx context.Context) { h.d.Hints.PublishHint(ctx, itemChangedKey(item.CabalID), nil) })
		return tx.Events.Append(ctx, events.ReactionAdded{
			V: 1, FeedObjectID: ref.FeedObjectID, CommentID: &cmd.CommentID, UserID: cmd.Viewer.UUID(),
			Emoji: emoji.String(), ItemActorID: parentOf(item.ActorID.UUID()), CabalID: parentOf(item.CabalID.UUID()),
		})
	})
	if err != nil {
		return ReactionSummary{}, err
	}
	return commentReactionSummary(ctx, h.d.Reads, cmd.Viewer, cmd.CommentID)
}

type UnreactCommentHandler struct{ d ReactionDeps }

func NewUnreactCommentHandler(d ReactionDeps) *UnreactCommentHandler {
	return &UnreactCommentHandler{d: d}
}

func (h *UnreactCommentHandler) Handle(ctx context.Context, cmd UnreactComment) (ReactionSummary, error) {
	const op = "social.UnreactComment"
	ref, err := liveComment(ctx, h.d.Reads, cmd.CommentID, op)
	if err != nil {
		return ReactionSummary{}, err
	}
	item, err := requireVisibleItem(ctx, h.d.Reads, cmd.Viewer, ref.FeedObjectID)
	if err != nil {
		return ReactionSummary{}, err
	}
	emoji, err := domain.ParseEmoji(cmd.Emoji)
	if err != nil {
		return ReactionSummary{}, err
	}
	err = h.d.UoW.Do(ctx, func(ctx context.Context, tx db.Tx) error {
		q := sqlc.New(tx.Queries())
		rows, err := q.RemoveCommentReaction(ctx, sqlc.RemoveCommentReactionParams{
			CommentID: cmd.CommentID, UserID: cmd.Viewer.UUID(), Emoji: emoji.String(),
		})
		if err != nil {
			return errs.Wrap(err, errs.CodeInternal, op)
		}
		if rows == 0 {
			return nil
		}
		tx.AfterCommit(func(ctx context.Context) { h.d.Hints.PublishHint(ctx, itemChangedKey(item.CabalID), nil) })
		return tx.Events.Append(ctx, events.ReactionRemoved{
			V: 1, FeedObjectID: ref.FeedObjectID, CommentID: &cmd.CommentID, UserID: cmd.Viewer.UUID(),
			Emoji: emoji.String(), ItemActorID: parentOf(item.ActorID.UUID()), CabalID: parentOf(item.CabalID.UUID()),
		})
	})
	if err != nil {
		return ReactionSummary{}, err
	}
	return commentReactionSummary(ctx, h.d.Reads, cmd.Viewer, cmd.CommentID)
}

func liveComment(ctx context.Context, reads sqlc.DBTX, id uuid.UUID, op string) (sqlc.GetCommentRefRow, error) {
	ref, err := sqlc.New(reads).GetCommentRef(ctx, id)
	switch {
	case errors.Is(err, sql.ErrNoRows), err == nil && ref.Deleted:
		return sqlc.GetCommentRefRow{}, errs.New(errs.CodeCommentNotFound, op, slog.String("comment_id", id.String()))
	case err != nil:
		return sqlc.GetCommentRefRow{}, errs.Wrap(err, errs.CodeInternal, op)
	}
	return ref, nil
}

func commentReactions(
	ctx context.Context, reads *sqlc.Queries, viewer ids.UserID, commentIDs []uuid.UUID,
) (map[uuid.UUID]ReactionSummary, error) {
	out := make(map[uuid.UUID]ReactionSummary, len(commentIDs))
	if len(commentIDs) == 0 {
		return out, nil
	}
	rows, err := reads.CommentReactionSummaries(
		ctx, sqlc.CommentReactionSummariesParams{Viewer: viewer.UUID(), CommentIds: commentIDs},
	)
	if err != nil {
		return nil, errs.Wrap(err, errs.CodeInternal, "social.commentReactions")
	}
	for _, row := range rows {
		summary := out[row.CommentID]
		summary.Reactions = append(
			summary.Reactions, Reaction{Emoji: domain.Emoji(row.Emoji), Count: row.Count, Mine: row.Mine},
		)
		summary.Total = row.Total
		out[row.CommentID] = summary
	}
	return out, nil
}

func commentReactionSummary(
	ctx context.Context, reads sqlc.DBTX, viewer ids.UserID, id uuid.UUID,
) (ReactionSummary, error) {
	found, err := commentReactions(ctx, sqlc.New(reads), viewer, []uuid.UUID{id})
	if err != nil {
		return ReactionSummary{}, err
	}
	return found[id], nil
}
