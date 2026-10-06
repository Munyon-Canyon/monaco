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
	if e.ParentCommentID == nil || e.ParentAuthorID == nil || e.ParentDeleted {
		return nil, nil
	}
	return []ids.UserID{ids.UserIDFrom(*e.ParentAuthorID)}, nil
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
		CollapseID: "comment-" + e.ParentCommentID.String(),
	}, nil
}
