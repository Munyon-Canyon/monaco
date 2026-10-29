package app

import (
	"context"
	"unicode/utf8"

	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

const commentExcerptMax = 120

type CommentReply struct{ Users Users }

func (CommentReply) Name() string { return "comment_reply" }

func (CommentReply) Recipients(_ context.Context, e events.CommentCreated) ([]ids.UserID, error) {
	answered, deleted := e.ReplyToAuthorID, e.ReplyToDeleted
	if e.ReplyToCommentID == nil {
		answered, deleted = e.ParentAuthorID, e.ParentDeleted
	}
	if answered == nil || deleted {
		return nil, nil
	}
	return []ids.UserID{ids.UserIDFrom(*answered)}, nil
}

func collapseRoot(e events.CommentCreated) string {
	switch {
	case e.ReplyToRootID != nil:
		return e.ReplyToRootID.String()
	case e.ParentCommentID != nil:
		return e.ParentCommentID.String()
	}
	return e.CommentID.String()
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
	return Message{
		Title:      replier + " replied to your comment",
		Body:       body,
		Data:       data,
		CollapseID: "comment-" + collapseRoot(e),
	}, nil
}
