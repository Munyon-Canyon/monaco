package adapters

import (
	"context"
	"slices"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/domain"
	api "github.com/monaco/monaco/apps/backend/internal/platform/httpx/api/socialapi"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

const commentDeletedDisplay = "Comment deleted"

func (h HTTP) PostFeedComment(
	ctx context.Context, req api.PostFeedCommentRequestObject,
) (api.PostFeedCommentResponseObject, error) {
	me, err := caller(ctx)
	if err != nil {
		return nil, err
	}
	if req.Body == nil {
		return nil, errs.New(errs.CodeInvalidInput, "social.PostFeedComment")
	}
	body, err := domain.ParseCommentBody(req.Body.Body)
	if err != nil {
		return nil, err
	}
	cmd := app.CreateComment{FeedObjectID: req.Id, Author: me, Body: body}
	if req.Body.ParentCommentId != nil {
		cmd.ParentID = *req.Body.ParentCommentId
	}
	created, err := h.CreateComment.Handle(ctx, cmd)
	if err != nil {
		return nil, err
	}
	wire, err := wireComments(ctx, h.Users, me, []app.Comment{created})
	if err != nil {
		return nil, err
	}
	return api.PostFeedComment201JSONResponse(wire[0]), nil
}

func wireComments(
	ctx context.Context, users app.Users, viewer ids.UserID, comments []app.Comment,
) ([]api.Comment, error) {
	cards, err := users.UsersByID(ctx, commentPeople(comments))
	if err != nil {
		return nil, errs.Wrap(err, errs.CodeOf(err), "social.wireComments")
	}
	out := make([]api.Comment, len(comments))
	for i, c := range comments {
		out[i] = wireComment(c, viewer, cards)
	}
	return out, nil
}

func commentPeople(comments []app.Comment) []ids.UserID {
	var people []ids.UserID
	for _, c := range comments {
		for _, id := range []ids.UserID{c.AuthorID, c.ReplyToUserID} {
			if id.UUID() != uuid.Nil && !slices.Contains(people, id) {
				people = append(people, id)
			}
		}
	}
	return people
}

func wireComment(c app.Comment, viewer ids.UserID, cards map[ids.UserID]app.UserCard) api.Comment {
	author := api.CommentAuthor{Id: c.AuthorID.UUID()}
	if card, ok := cards[c.AuthorID]; ok && !card.Deleted {
		author.Handle, author.DisplayName = optionalWireText(card.Handle), card.DisplayName
		author.PhotoUrl = optionalWireText(card.PhotoURL)
	}
	out := api.Comment{
		Id: c.ID, ParentCommentId: optionalWireID(c.ParentID), Author: author, IsMine: c.AuthorID == viewer,
		CreatedAt: c.CreatedAt, BodyDisplay: c.Body,
	}
	if card, ok := cards[c.ReplyToUserID]; ok && !card.Deleted {
		out.ReplyToHandle = optionalWireText(card.Handle)
	}
	if c.Deleted {
		out.BodyDisplay = commentDeletedDisplay
	} else {
		out.Body = &c.Body
	}
	return out
}

func (h HTTP) GetFeedComments(
	ctx context.Context, req api.GetFeedCommentsRequestObject,
) (api.GetFeedCommentsResponseObject, error) {
	page, err := h.commentPage(ctx, req.Id, req.Params.Cursor, req.Params.Limit)
	if err != nil {
		return nil, err
	}
	return api.GetFeedComments200JSONResponse(page), nil
}

func (h HTTP) GetProposalComments(
	ctx context.Context, req api.GetProposalCommentsRequestObject,
) (api.GetProposalCommentsResponseObject, error) {
	item, err := app.ProposalFeedItem(ctx, h.Reads, req.Id)
	if err != nil {
		return nil, err
	}
	page, err := h.commentPage(ctx, item, req.Params.Cursor, req.Params.Limit)
	if err != nil {
		return nil, err
	}
	return api.GetProposalComments200JSONResponse{
		FeedObjectId: item, Items: page.Items, NextCursor: page.NextCursor,
	}, nil
}

func (h HTTP) PostProposalComment(
	ctx context.Context, req api.PostProposalCommentRequestObject,
) (api.PostProposalCommentResponseObject, error) {
	item, err := app.ProposalFeedItem(ctx, h.Reads, req.Id)
	if err != nil {
		return nil, err
	}
	res, err := h.PostFeedComment(ctx, api.PostFeedCommentRequestObject{Id: item, Body: req.Body})
	if err != nil {
		return nil, err
	}
	return api.PostProposalComment201JSONResponse(res.(api.PostFeedComment201JSONResponse)), nil
}

func (h HTTP) commentPage(
	ctx context.Context, item uuid.UUID, cursor *string, limit *int,
) (api.CommentPage, error) {
	me, err := caller(ctx)
	if err != nil {
		return api.CommentPage{}, err
	}
	q := app.CommentsQuery{FeedObjectID: item, Limit: app.CommentPageDefault}
	if limit != nil {
		q.Limit = *limit
	}
	if cursor != nil {
		after, err := domain.ParseKeyset(*cursor)
		if err != nil {
			return api.CommentPage{}, err
		}
		q.After = &after
	}
	page, err := app.ListComments(ctx, h.Reads, q)
	if err != nil {
		return api.CommentPage{}, err
	}
	var flat []app.Comment
	for _, thread := range page.Threads {
		flat = append(flat, thread.Comment)
		flat = append(flat, thread.Replies...)
	}
	wire, err := wireComments(ctx, h.Users, me, flat)
	if err != nil {
		return api.CommentPage{}, err
	}
	body := api.CommentPage{Items: make([]api.CommentThread, len(page.Threads))}
	for i, thread := range page.Threads {
		body.Items[i] = api.CommentThread{Comment: wire[0], Replies: wire[1 : 1+len(thread.Replies)]}
		wire = wire[1+len(thread.Replies):]
	}
	if page.Next != nil {
		next := page.Next.Encode()
		body.NextCursor = &next
	}
	return body, nil
}

func (h HTTP) DeleteFeedComment(
	ctx context.Context, req api.DeleteFeedCommentRequestObject,
) (api.DeleteFeedCommentResponseObject, error) {
	me, err := caller(ctx)
	if err != nil {
		return nil, err
	}
	if err := h.DeleteComment.Handle(ctx, app.DeleteComment{CommentID: req.CommentId, Caller: me}); err != nil {
		return nil, err
	}
	return api.DeleteFeedComment204Response{}, nil
}
