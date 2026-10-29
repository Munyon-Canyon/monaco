package app

import (
	"context"
	"slices"

	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/notify/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

type CommentMention struct {
	Users  Users
	Cabals Cabals
	Blocks Blocks
}

func (CommentMention) Name() string { return "comment_mention" }

func (k CommentMention) Recipients(ctx context.Context, e events.CommentCreated) ([]ids.UserID, error) {
	out := make([]ids.UserID, 0, len(e.MentionedUserIDs))
	for _, mentioned := range e.MentionedUserIDs {
		to, err := unblocked(ctx, k.Blocks, e, ids.UserIDFrom(mentioned))
		if err != nil {
			return nil, err
		}
		out = append(out, to...)
	}
	return out, nil
}

func (CommentMention) ItemKey(e events.CommentCreated) string {
	return domain.ItemKeyCommentPrefix + e.CommentID.String()
}

func (k CommentMention) Render(ctx context.Context, e events.CommentCreated, _ ids.UserID) (Message, error) {
	card, err := replierCard(ctx, k.Users, ids.UserIDFrom(e.AuthorID))
	if err != nil {
		return Message{}, err
	}
	pres, err := replyPresentation(ctx, k.Cabals, e, card)
	if err != nil {
		return Message{}, err
	}
	body := replyBody(e)
	if body != "" {
		body += " "
	}
	return Message{
		Title:        PersonSender(card).Name + " mentioned you",
		Body:         body + "On " + e.ItemLabel + ".",
		Data:         replyData(k.Name(), e),
		CollapseID:   collapseID(e),
		Presentation: pres,
	}, nil
}

func mentioned(e events.CommentCreated, user ids.UserID) bool {
	return slices.Contains(e.MentionedUserIDs, user.UUID())
}
