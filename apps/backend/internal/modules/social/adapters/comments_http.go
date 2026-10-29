package adapters

import (
	"context"
	"encoding/json"
	"log/slog"
	"math"
	"slices"
	"time"

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
		parsed, err := attachmentInput(*a)
		if err != nil {
			return nil, err
		}
		att = &parsed
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

func attachmentInput(a api.CommentAttachmentInput) (domain.AttachmentInput, error) {
	raw, _ := a.MarshalJSON()
	var in struct {
		Kind          string        `json:"kind"`
		Gif           *api.GifInput `json:"gif"`
		Start         *int          `json:"start"`
		End           *int          `json:"end"`
		AssetID       uuid.UUID     `json:"asset_id"`
		Question      string        `json:"question"`
		Options       []string      `json:"options"`
		ClosesInHours *int          `json:"closes_in_hours"`
	}
	if json.Unmarshal(raw, &in) != nil {
		return domain.AttachmentInput{}, errs.New(
			errs.CodeInvalidInput, "social.PostFeedComment", slog.String("reason", "invalid_attachment"),
		)
	}
	out := domain.AttachmentInput{Kind: in.Kind, Start: in.Start, End: in.End, AssetID: in.AssetID}
	if in.Gif != nil {
		gif := domain.GifInput{
			ID: in.Gif.Id, Title: in.Gif.Title, Still: mediaInput(in.Gif.Still), Preview: mediaInput(in.Gif.Preview),
			Original: mediaInput(in.Gif.Original),
		}
		out.GIF = &gif
	}
	if in.Kind == string(domain.AttachmentPoll) {
		poll := domain.PollInput{Question: in.Question, Options: in.Options}
		if in.ClosesInHours != nil {
			closesIn := time.Duration(*in.ClosesInHours) * time.Hour
			poll.ClosesIn = &closesIn
		}
		out.Poll = &poll
	}
	return out, nil
}

func wireAttachment(c app.Comment) *api.CommentAttachment {
	a := c.Attachment
	var out api.CommentAttachment
	switch a.Kind {
	case domain.AttachmentPoll:
		_ = out.FromPollAttachment(api.PollAttachment{
			Kind: api.PollAttachmentKindPoll, Question: a.Poll.Question, ClosesAt: c.Poll.ClosesAt,
			Closed: c.Poll.Closed, Options: wirePollOptions(c.Poll), Total: c.Poll.Total, MyPosition: c.Poll.MyPosition,
		})
	case domain.AttachmentThesisQuote:
		_ = out.FromThesisQuoteAttachment(api.ThesisQuoteAttachment{
			Kind: api.ThesisQuoteAttachmentKindThesisQuote, ProposalId: a.Thesis.ProposalID, Text: a.Thesis.Text,
			ProposerName: a.Thesis.ProposerName,
		})
	case domain.AttachmentStock:
		_ = out.FromStockAttachment(wireStock(a.Stock, c.Stock))
	case domain.AttachmentGIF:
		_ = out.FromGifAttachment(api.GifAttachment{
			Kind: api.GifAttachmentKindGif, Id: a.GIF.ID, Title: a.GIF.Title, Still: wireMedia(a.GIF.Still),
			Preview: wireMedia(a.GIF.Preview), Original: wireMedia(a.GIF.Original),
		})
	}
	return &out
}

func wireStock(snap *domain.StockSnapshot, live *app.StockCard) api.StockAttachment {
	out := api.StockAttachment{
		Kind: api.StockAttachmentKindStock, AssetId: snap.AssetID, Symbol: snap.Symbol, Name: snap.Name,
		LogoUrl: snap.LogoURL, IsPreIpo: snap.IsPreIPO,
	}
	if live != nil {
		price := int64(min(live.PriceMicros.Uint64(), math.MaxInt64))
		out.PriceMicros, out.ChangeBps = &price, live.ChangeBps
	}
	return out
}

func wirePollOptions(p *app.PollView) []api.PollOption {
	options := make([]api.PollOption, len(p.Options))
	for i, o := range p.Options {
		options[i] = api.PollOption{Position: o.Position, Label: o.Label, Count: o.Count}
	}
	return options
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
		Reactions:       wireReactions(c.Reactions),
		ReactionCount:   c.ReactionCount,
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
	out.Attachment = wireAttachment(c)
	if c.Body == "" {
		out.BodyDisplay = string(c.Attachment.Display())
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
	q := app.CommentsQuery{
		FeedObjectID: item,
		Viewer:       me,
		Limit:        app.CommentPageDefault,
		Stocks:       h.Stocks,
		Now:          h.Clock.Now(),
	}
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

func (h HTTP) PutCommentPollVote(
	ctx context.Context, req api.PutCommentPollVoteRequestObject,
) (api.PutCommentPollVoteResponseObject, error) {
	me, err := caller(ctx)
	if err != nil {
		return nil, err
	}
	if req.Body == nil {
		return nil, errs.New(errs.CodeInvalidInput, "social.PutCommentPollVote")
	}
	state, err := h.VoteCommentPoll.Handle(ctx, app.VoteCommentPoll{
		CommentID: req.CommentId, Viewer: me, Position: req.Body.Position,
	})
	if err != nil {
		return nil, err
	}
	return api.PutCommentPollVote200JSONResponse(wirePollState(state)), nil
}

func (h HTTP) DeleteCommentPollVote(
	ctx context.Context, req api.DeleteCommentPollVoteRequestObject,
) (api.DeleteCommentPollVoteResponseObject, error) {
	me, err := caller(ctx)
	if err != nil {
		return nil, err
	}
	state, err := h.WithdrawCommentPollVote.Handle(ctx, app.WithdrawCommentPollVote{
		CommentID: req.CommentId, Viewer: me,
	})
	if err != nil {
		return nil, err
	}
	return api.DeleteCommentPollVote200JSONResponse(wirePollState(state)), nil
}

func wirePollState(p app.PollView) api.PollState {
	return api.PollState{
		ClosesAt: p.ClosesAt, Closed: p.Closed, Options: wirePollOptions(&p), Total: p.Total, MyPosition: p.MyPosition,
	}
}
