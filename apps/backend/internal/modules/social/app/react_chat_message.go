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

type ReactChatMessage struct {
	CabalID   ids.CabalID
	MessageID uuid.UUID
	Viewer    ids.UserID
	Emoji     string
}

type UnreactChatMessage = ReactChatMessage

type ReactChatMessageHandler struct{ d ChatDeps }

func NewReactChatMessageHandler(d ChatDeps) *ReactChatMessageHandler {
	return &ReactChatMessageHandler{d: d}
}

func (h *ReactChatMessageHandler) Handle(ctx context.Context, cmd ReactChatMessage) (ReactionSummary, error) {
	const op = "social.ReactChatMessage"
	emoji, err := checkChatReaction(ctx, h.d.Members, cmd)
	if err != nil {
		return ReactionSummary{}, err
	}
	changed := false
	err = h.d.UoW.Do(ctx, func(ctx context.Context, tx db.Tx) error {
		q := sqlc.New(tx.Queries())
		author, err := liveChatAuthor(ctx, q, cmd, op)
		if err != nil {
			return err
		}
		if err := refuseBlockedAuthor(ctx, tx, cmd.Viewer, author, op); err != nil {
			return err
		}
		_, err = q.AddChatReaction(ctx, sqlc.AddChatReactionParams{
			MessageID: cmd.MessageID, CabalID: cmd.CabalID.UUID(), UserID: cmd.Viewer.UUID(),
			Emoji: emoji.String(), CreatedAt: h.d.Clock.Now().UTC(),
		})
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		if err != nil {
			return errs.Wrap(err, errs.CodeInternal, op)
		}
		held, err := q.CountUserChatReactions(ctx, sqlc.CountUserChatReactionsParams{
			MessageID: cmd.MessageID, UserID: cmd.Viewer.UUID(),
		})
		if err != nil {
			return errs.Wrap(err, errs.CodeInternal, op)
		}
		if held > domain.MaxReactionsPerUser {
			return errs.New(errs.CodeInvalidInput, op, slog.String("reason", "reaction_limit"))
		}
		changed = true
		return tx.Events.Append(ctx, events.ChatReactionAdded{
			V: 1, MessageID: cmd.MessageID, CabalID: cmd.CabalID.UUID(), UserID: cmd.Viewer.UUID(),
			Emoji: emoji.String(), AuthorID: &author,
		})
	})
	if err != nil {
		return ReactionSummary{}, err
	}
	return h.d.settleReaction(ctx, cmd, changed)
}

type UnreactChatMessageHandler struct{ d ChatDeps }

func NewUnreactChatMessageHandler(d ChatDeps) *UnreactChatMessageHandler {
	return &UnreactChatMessageHandler{d: d}
}

func (h *UnreactChatMessageHandler) Handle(ctx context.Context, cmd UnreactChatMessage) (ReactionSummary, error) {
	const op = "social.UnreactChatMessage"
	emoji, err := checkChatReaction(ctx, h.d.Members, cmd)
	if err != nil {
		return ReactionSummary{}, err
	}
	changed := false
	err = h.d.UoW.Do(ctx, func(ctx context.Context, tx db.Tx) error {
		q := sqlc.New(tx.Queries())
		author, err := liveChatAuthor(ctx, q, cmd, op)
		if err != nil {
			return err
		}
		rows, err := q.RemoveChatReaction(ctx, sqlc.RemoveChatReactionParams{
			MessageID: cmd.MessageID, UserID: cmd.Viewer.UUID(), Emoji: emoji.String(),
		})
		if err != nil {
			return errs.Wrap(err, errs.CodeInternal, op)
		}
		if rows == 0 {
			return nil
		}
		changed = true
		return tx.Events.Append(ctx, events.ChatReactionRemoved{
			V: 1, MessageID: cmd.MessageID, CabalID: cmd.CabalID.UUID(), UserID: cmd.Viewer.UUID(),
			Emoji: emoji.String(), AuthorID: &author,
		})
	})
	if err != nil {
		return ReactionSummary{}, err
	}
	return h.d.settleReaction(ctx, cmd, changed)
}

func checkChatReaction(ctx context.Context, members Members, cmd ReactChatMessage) (domain.Emoji, error) {
	if err := requireMember(ctx, members, cmd.CabalID, cmd.Viewer); err != nil {
		return "", err
	}
	return domain.ParseEmoji(cmd.Emoji)
}

func liveChatAuthor(ctx context.Context, q *sqlc.Queries, cmd ReactChatMessage, op string) (uuid.UUID, error) {
	author, err := q.LiveChatMessageAuthor(ctx, sqlc.LiveChatMessageAuthorParams{
		ID: cmd.MessageID, CabalID: cmd.CabalID.UUID(),
	})
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return uuid.Nil, errs.New(errs.CodeChatMessageNotFound, op, slog.String("message_id", cmd.MessageID.String()))
	case err != nil:
		return uuid.Nil, errs.Wrap(err, errs.CodeInternal, op)
	}
	return author.Bytes, nil
}

func refuseBlockedAuthor(ctx context.Context, tx db.Tx, viewer ids.UserID, author uuid.UUID, op string) error {
	blocked, err := blockedEitherWay(ctx, tx, viewer, ids.UserIDFrom(author))
	if err != nil {
		return err
	}
	if blocked {
		return errs.New(errs.CodeChatMessageNotFound, op, slog.String("reason", "blocked"))
	}
	return nil
}

func (d ChatDeps) settleReaction(ctx context.Context, cmd ReactChatMessage, changed bool) (ReactionSummary, error) {
	summary, err := ChatReactionSummary(ctx, d.Reads, cmd.Viewer, cmd.MessageID)
	if err != nil {
		return ReactionSummary{}, err
	}
	if changed {
		d.Publish.reactionUpdated(ctx, cmd.CabalID, cmd.MessageID, summary)
	}
	return summary, nil
}

func ChatReactions(
	ctx context.Context, reads sqlc.DBTX, viewer ids.UserID, messageIDs []uuid.UUID,
) (map[uuid.UUID]ReactionSummary, error) {
	out := make(map[uuid.UUID]ReactionSummary, len(messageIDs))
	if len(messageIDs) == 0 {
		return out, nil
	}
	rows, err := sqlc.New(reads).ChatReactionSummaries(
		ctx, sqlc.ChatReactionSummariesParams{Viewer: viewer.UUID(), MessageIds: messageIDs},
	)
	if err != nil {
		return nil, errs.Wrap(err, errs.CodeInternal, "social.ChatReactions")
	}
	for _, row := range rows {
		summary := out[row.MessageID]
		summary.Reactions = append(
			summary.Reactions, Reaction{Emoji: domain.Emoji(row.Emoji), Count: row.Count, Mine: row.Mine},
		)
		summary.Total = row.Total
		out[row.MessageID] = summary
	}
	return out, nil
}

func ChatReactionSummary(
	ctx context.Context, reads sqlc.DBTX, viewer ids.UserID, id uuid.UUID,
) (ReactionSummary, error) {
	found, err := ChatReactions(ctx, reads, viewer, []uuid.UUID{id})
	if err != nil {
		return ReactionSummary{}, err
	}
	return found[id], nil
}
