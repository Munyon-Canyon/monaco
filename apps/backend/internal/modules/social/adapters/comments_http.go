package adapters

import (
	"context"
	"slices"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/domain"
	api "github.com/monaco/monaco/apps/backend/internal/platform/httpx/api/socialapi"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

const (
	commentDeletedDisplay = "Comment deleted"
	commentRemovedDisplay = "Removed by Monaco"
	quoteAuthorDeleted    = "Someone"
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
	var att *domain.AttachmentInput
	if a := req.Body.Attachment; a != nil {
		gif := domain.GifInput{
			ID: a.Gif.Id, Title: a.Gif.Title, Still: mediaInput(a.Gif.Still), Preview: mediaInput(a.Gif.Preview),
			Original: mediaInput(a.Gif.Original),
		}
		att = &domain.AttachmentInput{Kind: string(a.Kind), GIF: &gif}
	}
	input, err := domain.ParseCommentInput(req.Body.Body, att)
	if err != nil {
		return nil, err
	}
	cmd := app.CreateComment{FeedObjectID: req.Id, Author: me, Input: input}
	if req.Body.ParentCommentId != nil {
		cmd.ParentID = *req.Body.ParentCommentId
	}
	created, err := h.CreateComment.Handle(ctx, cmd)
	if err != nil {
		return nil, err
	}
	wire, err := wireComments(ctx, h.Users, h.Returns, me, []app.Comment{created})
	if err != nil {
		return nil, err
	}
	return api.PostFeedComment201JSONResponse(wire[0]), nil
}

func mediaInput(m api.GifMedia) domain.Media {
	return domain.Media{URL: m.Url, Width: m.Width, Height: m.Height}
}

func wireMedia(m domain.Media) api.GifMedia {
	return api.GifMedia{Url: m.URL, Width: m.Width, Height: m.Height}
}

func wireAttachment(a *domain.Attachment) *api.GifAttachment {
	return &api.GifAttachment{
		Kind: api.GifAttachmentKindGif, Id: a.GIF.ID, Title: a.GIF.Title, Still: wireMedia(a.GIF.Still),
		Preview: wireMedia(a.GIF.Preview), Original: wireMedia(a.GIF.Original),
	}
}

func wireComments(
	ctx context.Context, users app.Users, returns app.Returns, viewer ids.UserID, comments []app.Comment,
) ([]api.Comment, error) {
	who, err := loadPeople(ctx, users, returns, commentPeople(comments), "social.wireComments")
	if err != nil {
		return nil, err
	}
	out := make([]api.Comment, len(comments))
	for i, c := range comments {
		out[i] = wireComment(c, viewer, who)
	}
	return out, nil
}

func commentPeople(comments []app.Comment) []ids.UserID {
	var people []ids.UserID
	for _, c := range comments {
		for _, id := range commentPerson(c) {
			if id.UUID() != uuid.Nil && !slices.Contains(people, id) {
				people = append(people, id)
			}
		}
	}
	return people
}

func commentPerson(c app.Comment) []ids.UserID {
	people := []ids.UserID{c.AuthorID, c.ReplyToUserID}
	if c.ReplyTo != nil {
		people = append(people, c.ReplyTo.AuthorID)
	}
	return people
}

func wireQuote(q *app.CommentQuote, who people) *api.CommentQuote {
	name := quoteAuthorDeleted
	if card, ok := who.cards[q.AuthorID]; ok && !card.Deleted {
		name = card.DisplayName
	}
	return &api.CommentQuote{
		CommentId: q.CommentID, RootCommentId: q.RootID, AuthorDisplayName: name, Excerpt: q.Excerpt,
		IsDeleted: q.Deleted, IsFolded: q.Folded, HasAttachment: q.HasAttachment,
	}
}

func wireComment(c app.Comment, viewer ids.UserID, who people) api.Comment {
	author := api.CommentAuthor{Id: c.AuthorID.UUID()}
	if card, ok := who.cards[c.AuthorID]; ok && !card.Deleted {
		author.Handle, author.DisplayName = optionalWireText(card.Handle), card.DisplayName
		author.PhotoUrl = optionalWireText(card.PhotoURL)
	}
	author.Return1mBps, author.ReturnPrivate = who.authorReturn(c.AuthorID)
	out := api.Comment{
		Id:              c.ID,
		ParentCommentId: optionalWireID(c.ParentID),
		RootCommentId:   optionalWireID(c.ParentID),
		Author:          author,
		IsMine:          c.AuthorID == viewer,
		CreatedAt:       c.CreatedAt,
		BodyDisplay:     c.Body,
	}
	if card, ok := who.cards[c.ReplyToUserID]; ok && !card.Deleted {
		out.ReplyToHandle = optionalWireText(card.Handle)
	}
	if c.ReplyTo != nil {
		out.ReplyTo = wireQuote(c.ReplyTo, who)
	}
	switch {
	case c.Deleted && c.RemovedByAdmin:
		out.BodyDisplay = commentRemovedDisplay
	case c.Deleted:
		out.BodyDisplay = commentDeletedDisplay
	default:
		wireLiveBody(&out, c)
	}
	return out
}

func wireLiveBody(out *api.Comment, c app.Comment) {
	if c.Body != "" {
		out.Body = &c.Body
	}
	if c.Attachment == nil {
		return
	}
	out.Attachment = wireAttachment(c.Attachment)
	if c.Body == "" {
		out.BodyDisplay = string(domain.GifDisplay)
	}
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
	q := app.CommentsQuery{FeedObjectID: item, Viewer: me, Limit: app.CommentPageDefault}
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
	wire, err := wireComments(ctx, h.Users, h.Returns, me, flat)
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

func (h HTTP) RemoveComment(
	ctx context.Context, req api.RemoveCommentRequestObject,
) (api.RemoveCommentResponseObject, error) {
	admin, err := adminCaller(ctx)
	if err != nil {
		return nil, err
	}
	reason, err := events.NewReason(req.Body.Reason)
	if err != nil {
		return nil, err
	}
	if err := h.Removal.Handle(ctx, app.RemoveComment{CommentID: req.Id, Reason: reason, Admin: admin}); err != nil {
		return nil, err
	}
	return api.RemoveComment204Response{}, nil
}
