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
	Members ChatMembers
	Users   Users
	IDs     ids.Generator
	Clock   clock.Clock
	Publish ChatPublisher
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
	aud, err := h.audienceOf(ctx, cmd)
	if err != nil {
		return ChatMessage{}, err
	}
	var posted ChatMessage
	var thread *ThreadUpdated
	err = h.d.UoW.Do(ctx, func(ctx context.Context, tx db.Tx) error {
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
			bumped, err := q.BumpChatReplies(ctx, sqlc.BumpChatRepliesParams{At: now, ID: cmd.Reply.Parent})
			if err != nil {
				return errs.Wrap(err, errs.CodeInternal, op)
			}
			thread = &ThreadUpdated{
				MessageID: bumped.ID, ReplyCount: bumped.ReplyCount, LastReplyAt: bumped.LastReplyAt.Time.UTC(),
			}
			params.ParentID = cmd.Reply.Parent
			params.AlsoInChannel = cmd.Reply.AlsoInChannel
		}
		row, err := q.InsertChatMessage(ctx, params)
		if err != nil {
			return errs.Wrap(err, errs.CodeInternal, op)
		}
		posted = postedMessage(row)
		participants, err := aud.participants(ctx, q, cmd.Reply)
		if err != nil {
			return err
		}
		return tx.Events.Append(ctx, events.ChatMessagePosted{
			V:             1,
			MessageID:     row.ID,
			CabalID:       row.CabalID,
			AuthorID:      row.AuthorID,
			ParentID:      parentOf(row.ParentID.Bytes),
			AlsoInChannel: row.AlsoInChannel,
			CreatedAt:     row.CreatedAt.UTC(),

			MentionedUserIDs:     aud.mentioned,
			ThreadParticipantIDs: participants,
		})
	})
	if err != nil {
		return ChatMessage{}, err
	}
	h.d.Publish.created(ctx, posted, thread)
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

const maxThreadParticipants = 200

type audience struct {
	mentioned []uuid.UUID
	current   map[uuid.UUID]bool
}

func (h *PostChatMessageHandler) audienceOf(ctx context.Context, cmd PostChatMessage) (audience, error) {
	const op = "social.PostChatMessage.audienceOf"
	out := audience{mentioned: []uuid.UUID{}}
	handles := domain.ParseMentions(cmd.Body.String())
	if len(handles) == 0 && cmd.Reply == nil {
		return out, nil
	}
	members, err := h.d.Members.Members(ctx, cmd.CabalID)
	if err != nil {
		return audience{}, errs.Wrap(err, errs.CodeOf(err), op)
	}
	out.current = make(map[uuid.UUID]bool, len(members))
	for _, m := range members {
		out.current[m.UserID.UUID()] = m.UserID != cmd.Author
	}
	if len(handles) == 0 {
		return out, nil
	}
	byHandle, err := h.d.Users.UserIDsByHandles(ctx, handles)
	if err != nil {
		return audience{}, errs.Wrap(err, errs.CodeOf(err), op)
	}
	for _, handle := range handles {
		if id, ok := byHandle[handle]; ok && out.current[id.UUID()] {
			out.mentioned = append(out.mentioned, id.UUID())
		}
	}
	return out, nil
}

func (a audience) participants(ctx context.Context, q *sqlc.Queries, reply *domain.Reply) ([]uuid.UUID, error) {
	out := []uuid.UUID{}
	if reply == nil {
		return out, nil
	}
	authors, err := q.ThreadParticipants(ctx, reply.Parent)
	if err != nil {
		return nil, errs.Wrap(err, errs.CodeInternal, "social.PostChatMessage.participants")
	}
	for _, id := range authors {
		if a.current[id] && len(out) < maxThreadParticipants {
			out = append(out, id)
		}
	}
	return out, nil
}
