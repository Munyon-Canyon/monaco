package app

import (
	"context"
	"unicode/utf8"

	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/notify/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

const commentExcerptMax = 120

type CommentReply struct{ Users Users }

func (CommentReply) Name() string { return "comment_reply" }

func (CommentReply) Recipients(_ context.Context, e events.CommentCreated) ([]ids.UserID, error) {
	switch {
	case e.ReplyToAuthorID != nil:
		if e.ReplyToDeleted {
			return nil, nil
		}
		return []ids.UserID{ids.UserIDFrom(*e.ReplyToAuthorID)}, nil
	case e.ParentAuthorID != nil && !e.ParentDeleted:
		return []ids.UserID{ids.UserIDFrom(*e.ParentAuthorID)}, nil
	}
	return nil, nil
}

func collapseID(e events.CommentCreated) string {
	root := e.ParentCommentID
	if e.ReplyToRootID != nil {
		root = e.ReplyToRootID
	}
	return "comment-" + root.String()
}

func (CommentReply) ItemKey(e events.CommentCreated) string {
	return domain.ItemKeyCommentPrefix + e.CommentID.String()
}

func (k CommentReply) Render(ctx context.Context, e events.CommentCreated, _ ids.UserID) (Message, error) {
	replier, err := proposerName(ctx, k.Users, ids.UserIDFrom(e.AuthorID))
	if err != nil {
		return Message{}, err
	}
	body := e.Excerpt
	if utf8.RuneCountInString(body) >= commentExcerptMax {
		body += "…"
	}
	data := map[string]string{"kind": k.Name(), "feed_item_id": e.FeedObjectID.String()}
	if e.ProposalID != nil && e.CabalID != nil {
		data["proposal_id"], data["cabal_id"] = e.ProposalID.String(), e.CabalID.String()
	}
	if e.ProposalID == nil {
		data["item_kind"], data["item_label"], data["item_actor_name"] = e.ItemKind, e.ItemLabel, e.ItemActorName
	}
	return Message{
		Title:      replier + " replied to your comment",
		Body:       body,
		Data:       data,
		CollapseID: collapseID(e),
	}, nil
}
