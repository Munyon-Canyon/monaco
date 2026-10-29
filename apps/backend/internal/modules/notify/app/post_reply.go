package app

import (
	"context"
	"slices"

	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/notify/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

func postItemKinds() []string {
	return []string{"proposal", "result", "trade", "member_joined"}
}

type PostReply struct {
	Users  Users
	Cabals Cabals
	Blocks Blocks
}

func (PostReply) Name() string { return "post_reply" }

func (k PostReply) Recipients(ctx context.Context, e events.CommentCreated) ([]ids.UserID, error) {
	if e.ItemActorID == nil || !slices.Contains(postItemKinds(), e.ItemKind) {
		return nil, nil
	}
	owner := ids.UserIDFrom(*e.ItemActorID)
	if owner == ids.UserIDFrom(e.AuthorID) {
		return nil, nil
	}
	if mentioned(e, owner) {
		return nil, nil
	}
	if answered, ok := answeredAuthor(e); ok && answered == owner {
		return nil, nil
	}
	return unblocked(ctx, k.Blocks, e, owner)
}

func (PostReply) ItemKey(e events.CommentCreated) string {
	return domain.ItemKeyCommentPrefix + e.CommentID.String()
}

func (k PostReply) Render(ctx context.Context, e events.CommentCreated, _ ids.UserID) (Message, error) {
	card, err := replierCard(ctx, k.Users, ids.UserIDFrom(e.AuthorID))
	if err != nil {
		return Message{}, err
	}
	pres, err := replyPresentation(ctx, k.Cabals, e, card)
	if err != nil {
		return Message{}, err
	}
	return Message{
		Title:        PersonSender(card).Name + " replied to your " + e.ItemNoun,
		Body:         replyBody(e),
		Data:         replyData(k.Name(), e),
		CollapseID:   "post-" + e.ThreadFeedObjectID.String(),
		Presentation: pres,
	}, nil
}
