package app

import (
	"context"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

type PostChatMessage struct {
	CabalID ids.CabalID
	Author  ids.UserID
	Body    domain.ChatBody
	Reply   *domain.Reply
}

type ChatDeps struct {
	UoW     *db.UnitOfWork
	Members Members
	IDs     ids.Generator
	Clock   clock.Clock
}

type PostChatMessageHandler struct {
	d ChatDeps
}

func NewPostChatMessageHandler(d ChatDeps) *PostChatMessageHandler {
	return &PostChatMessageHandler{d: d}
}

func (h *PostChatMessageHandler) Handle(ctx context.Context, cmd PostChatMessage) (ChatMessage, error) {
	const op = "social.PostChatMessage"
	if err := requireMember(ctx, h.d.Members, cmd.CabalID, cmd.Author); err != nil {
		return ChatMessage{}, err
	}
	var posted ChatMessage
	err := h.d.UoW.Do(ctx, func(ctx context.Context, tx db.Tx) error {
		q := sqlc.New(tx.Queries())
		now := h.d.Clock.Now().UTC()
		params := sqlc.InsertChatMessageParams{
			ID: h.d.IDs.NewV7(), CabalID: cmd.CabalID.UUID(), AuthorID: cmd.Author.UUID(), Body: cmd.Body.String(),
			CreatedAt: now,
		}
		if cmd.Reply != nil {
			if err := lockParent(ctx, q, cmd.CabalID, cmd.Reply.Parent); err != nil {
				return err
			}
			if _, err := q.BumpChatReplies(ctx, sqlc.BumpChatRepliesParams{At: now, ID: cmd.Reply.Parent}); err != nil {
				return errs.Wrap(err, errs.CodeInternal, op)
			}
			params.ParentID = cmd.Reply.Parent
			params.AlsoInChannel = cmd.Reply.AlsoInChannel
		}
		row, err := q.InsertChatMessage(ctx, params)
		if err != nil {
			return errs.Wrap(err, errs.CodeInternal, op)
		}
		posted = postedMessage(row)
		return tx.Events.Append(ctx, events.ChatMessagePosted{
			V:             1,
			MessageID:     row.ID,
			CabalID:       row.CabalID,
			AuthorID:      row.AuthorID,
			ParentID:      parentOf(row.ParentID.Bytes),
			AlsoInChannel: row.AlsoInChannel,
			CreatedAt:     row.CreatedAt.UTC(),
		})
	})
	if err != nil {
		return ChatMessage{}, err
	}
	return posted, nil
}

func postedMessage(row sqlc.InsertChatMessageRow) ChatMessage {
	return ChatMessage{
		ID: row.ID, CabalID: ids.CabalIDFrom(row.CabalID), AuthorID: ids.UserIDFrom(row.AuthorID), Body: row.Body,
		CreatedAt: row.CreatedAt.UTC(), ParentID: row.ParentID.Bytes, AlsoInChannel: row.AlsoInChannel,
	}
}

func parentOf(id uuid.UUID) *uuid.UUID {
	if id == uuid.Nil {
		return nil
	}
	return &id
}
