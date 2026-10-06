package adapters

import (
	"context"
	"log/slog"
	"slices"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/domain"
	api "github.com/monaco/monaco/apps/backend/internal/platform/httpx/api/socialapi"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

func (h HTTP) PostChatMessage(
	ctx context.Context, req api.PostChatMessageRequestObject,
) (api.PostChatMessageResponseObject, error) {
	me, err := caller(ctx)
	if err != nil {
		return nil, err
	}
	cmd, err := postChatCommand(ids.CabalIDFrom(req.Id), me, req.Body)
	if err != nil {
		return nil, err
	}
	posted, err := h.PostChat.Handle(ctx, cmd)
	if err != nil {
		return nil, err
	}
	wire, err := h.wireChat(ctx, []app.ChatMessage{posted})
	if err != nil {
		return nil, err
	}
	return api.PostChatMessage201JSONResponse(wire[0]), nil
}

func (h HTTP) DeleteChatMessage(
	ctx context.Context, req api.DeleteChatMessageRequestObject,
) (api.DeleteChatMessageResponseObject, error) {
	me, err := caller(ctx)
	if err != nil {
		return nil, err
	}
	cmd := app.DeleteChatMessage{CabalID: ids.CabalIDFrom(req.Id), MessageID: req.MessageId, Caller: me}
	if err := h.DeleteChat.Handle(ctx, cmd); err != nil {
		return nil, err
	}
	return api.DeleteChatMessage204Response{}, nil
}

func postChatCommand(cabal ids.CabalID, me ids.UserID, body *api.PostChatMessageRequest) (app.PostChatMessage, error) {
	const op = "social.PostChatMessage"
	if body == nil {
		return app.PostChatMessage{}, errs.New(errs.CodeInvalidInput, op)
	}
	text, err := domain.ParseChatBody(body.Body)
	if err != nil {
		return app.PostChatMessage{}, err
	}
	cmd := app.PostChatMessage{CabalID: cabal, Author: me, Body: text}
	also := body.AlsoInChannel != nil && *body.AlsoInChannel
	switch {
	case body.ParentId != nil:
		cmd.Reply = &domain.Reply{Parent: *body.ParentId, AlsoInChannel: also}
	case also:
		return app.PostChatMessage{}, errs.New(errs.CodeInvalidInput, op, slog.String("also_in_channel", "no parent"))
	}
	return cmd, nil
}

func (h HTTP) wireChat(ctx context.Context, messages []app.ChatMessage) ([]api.ChatMessage, error) {
	authors := make([]ids.UserID, 0, len(messages))
	for _, m := range messages {
		if !slices.Contains(authors, m.AuthorID) {
			authors = append(authors, m.AuthorID)
		}
	}
	cards, err := h.Users.UsersByID(ctx, authors)
	if err != nil {
		return nil, errs.Wrap(err, errs.CodeOf(err), "social.wireChat")
	}
	out := make([]api.ChatMessage, len(messages))
	for i, m := range messages {
		author := api.ChatAuthor{Id: m.AuthorID.UUID()}
		if card, ok := cards[m.AuthorID]; ok && !card.Deleted {
			author.Handle, author.DisplayName = optionalWireText(card.Handle), card.DisplayName
			author.PhotoUrl = optionalWireText(card.PhotoURL)
		}
		out[i] = api.ChatMessage{
			Id: m.ID, Author: author, CreatedAt: m.CreatedAt, ParentId: optionalWireID(m.ParentID),
			AlsoInChannel: m.AlsoInChannel, ReplyCount: int(m.ReplyCount), LastReplyAt: m.LastReplyAt,
			ProposalId: optionalWireID(m.ProposalID), Deleted: m.Deleted,
		}
		if !m.Deleted {
			out[i].Body = &m.Body
		}
	}
	return out, nil
}
