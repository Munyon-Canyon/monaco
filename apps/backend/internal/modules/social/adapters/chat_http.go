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

func (h HTTP) GetChatMessages(
	ctx context.Context, req api.GetChatMessagesRequestObject,
) (api.GetChatMessagesResponseObject, error) {
	me, err := caller(ctx)
	if err != nil {
		return nil, err
	}
	q := app.ChatChannelQuery{
		CabalID: ids.CabalIDFrom(req.Id), Viewer: me, Before: req.Params.Before, After: req.Params.After,
		Limit: limitOr(req.Params.Limit),
	}
	messages, err := app.ListChatChannel(ctx, h.Reads, h.Members, q)
	if err != nil {
		return nil, err
	}
	wire, err := h.wireChat(ctx, messages)
	if err != nil {
		return nil, err
	}
	return api.GetChatMessages200JSONResponse(api.ChatChannelPage{Messages: wire}), nil
}

func (h HTTP) GetChatThread(
	ctx context.Context, req api.GetChatThreadRequestObject,
) (api.GetChatThreadResponseObject, error) {
	me, err := caller(ctx)
	if err != nil {
		return nil, err
	}
	thread, err := app.GetChatThread(ctx, h.Reads, h.Members, app.ChatThreadQuery{
		CabalID: ids.CabalIDFrom(req.Id), Viewer: me, ParentID: req.MessageId, Before: req.Params.Before,
		Limit: limitOr(req.Params.Limit),
	})
	if err != nil {
		return nil, err
	}
	wire, err := h.wireChat(ctx, append([]app.ChatMessage{thread.Parent}, thread.Replies...))
	if err != nil {
		return nil, err
	}
	return api.GetChatThread200JSONResponse(api.ChatThread{Parent: wire[0], Replies: wire[1:]}), nil
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

func limitOr(limit *int) int {
	if limit == nil {
		return app.ChatPageDefault
	}
	return *limit
}

func (h HTTP) wireChat(ctx context.Context, messages []app.ChatMessage) ([]api.ChatMessage, error) {
	return wireChat(ctx, h.Users, messages)
}

func ChatWire(users app.Users) app.ChatWire {
	return func(ctx context.Context, m app.ChatMessage) (any, error) {
		wire, err := wireChat(ctx, users, []app.ChatMessage{m})
		if err != nil {
			return nil, err
		}
		return wire[0], nil
	}
}

func wireChat(ctx context.Context, users app.Users, messages []app.ChatMessage) ([]api.ChatMessage, error) {
	authors := make([]ids.UserID, 0, len(messages))
	for _, m := range messages {
		if !slices.Contains(authors, m.AuthorID) {
			authors = append(authors, m.AuthorID)
		}
	}
	cards, err := users.UsersByID(ctx, authors)
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
