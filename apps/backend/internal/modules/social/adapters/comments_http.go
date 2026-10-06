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
	var people []ids.UserID
	for _, c := range comments {
		for _, id := range []ids.UserID{c.AuthorID, c.ReplyToUserID} {
			if id.UUID() != uuid.Nil && !slices.Contains(people, id) {
				people = append(people, id)
			}
		}
	}
	cards, err := users.UsersByID(ctx, people)
	if err != nil {
		return nil, errs.Wrap(err, errs.CodeOf(err), "social.wireComments")
	}
	out := make([]api.Comment, len(comments))
	for i, c := range comments {
		author := api.CommentAuthor{Id: c.AuthorID.UUID()}
		if card, ok := cards[c.AuthorID]; ok && !card.Deleted {
			author.Handle, author.DisplayName = optionalWireText(card.Handle), card.DisplayName
			author.PhotoUrl = optionalWireText(card.PhotoURL)
		}
		out[i] = api.Comment{
			Id: c.ID, ParentCommentId: optionalWireID(c.ParentID), Author: author, IsMine: c.AuthorID == viewer,
			CreatedAt: c.CreatedAt, BodyDisplay: c.Body,
		}
		if card, ok := cards[c.ReplyToUserID]; ok && !card.Deleted {
			out[i].ReplyToHandle = optionalWireText(card.Handle)
		}
		out[i].Body = &c.Body
	}
	return out, nil
}
