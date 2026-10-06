package app

import (
	"context"
	"slices"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

type ChatMention struct {
	Cabals Cabals
	Users  Users
}

func (ChatMention) Name() string { return "chat_mention" }

func (k ChatMention) Recipients(ctx context.Context, e events.ChatMessagePosted) ([]ids.UserID, error) {
	return chatRecipients(ctx, k.Cabals, e.CabalID, e.MentionedUserIDs, nil)
}

func (k ChatMention) Render(ctx context.Context, e events.ChatMessagePosted, _ ids.UserID) (Message, error) {
	return chatMessage(ctx, k.Cabals, k.Users, k.Name(), e, " mentioned you")
}

type ChatThreadReply struct {
	Cabals Cabals
	Users  Users
}

func (ChatThreadReply) Name() string { return "chat_thread_reply" }

func (k ChatThreadReply) Recipients(ctx context.Context, e events.ChatMessagePosted) ([]ids.UserID, error) {
	return chatRecipients(ctx, k.Cabals, e.CabalID, e.ThreadParticipantIDs, e.MentionedUserIDs)
}

func (k ChatThreadReply) Render(ctx context.Context, e events.ChatMessagePosted, _ ids.UserID) (Message, error) {
	return chatMessage(ctx, k.Cabals, k.Users, k.Name(), e, " replied in a thread you're in")
}

func chatRecipients(
	ctx context.Context, cabals Cabals, cabalID uuid.UUID, listed, except []uuid.UUID,
) ([]ids.UserID, error) {
	if len(listed) == 0 {
		return nil, nil
	}
	members, err := memberIDs(ctx, cabals, &cabalID)
	if err != nil {
		return nil, err
	}
	return slices.DeleteFunc(members, func(user ids.UserID) bool {
		return !slices.Contains(listed, user.UUID()) || slices.Contains(except, user.UUID())
	}), nil
}

func chatMessage(
	ctx context.Context, cabals Cabals, users Users, kind string, e events.ChatMessagePosted, action string,
) (Message, error) {
	view, err := cabals.Cabal(ctx, ids.CabalIDFrom(e.CabalID))
	if err != nil {
		return Message{}, err
	}
	poster, err := proposerName(ctx, users, ids.UserIDFrom(e.AuthorID))
	if err != nil {
		return Message{}, err
	}
	thread := e.MessageID
	if e.ParentID != nil {
		thread = *e.ParentID
	}
	return Message{
		Title: view.Name,
		Body:  poster + action,
		Data: map[string]string{
			"kind": kind, "cabal_id": e.CabalID.String(), "message_id": e.MessageID.String(),
		},
		CollapseID: "chat-" + thread.String(),
	}, nil
}
