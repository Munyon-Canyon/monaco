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
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
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
		observability.Degraded(ctx, observability.SocialChatAuthorUnreadable,
			slog.String("message_id", posted.ID.String()), slog.String("cause", err.Error()))
		wire = wireChatWith(nil, []app.ChatMessage{posted})
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

func (h HTTP) MarkChatSeen(
	ctx context.Context, req api.MarkChatSeenRequestObject,
) (api.MarkChatSeenResponseObject, error) {
	me, err := caller(ctx)
	if err != nil {
		return nil, err
	}
	seen, err := h.MarkSeen.Handle(ctx, app.MarkChatSeen{CabalID: ids.CabalIDFrom(req.Id), Caller: me})
	if err != nil {
		return nil, err
	}
	return api.MarkChatSeen200JSONResponse{LastSeenAt: seen}, nil
}

func (h HTTP) GetChatSeenBy(
	ctx context.Context, req api.GetChatSeenByRequestObject,
) (api.GetChatSeenByResponseObject, error) {
	me, err := caller(ctx)
	if err != nil {
		return nil, err
	}
	seen, err := app.GetChatSeenBy(ctx, h.Reads, h.Members, app.ChatSeenByQuery{
		CabalID: ids.CabalIDFrom(req.Id), Viewer: me, MessageID: req.Params.MessageId,
	})
	if err != nil {
		return nil, err
	}
	cards, err := h.Users.UsersByID(ctx, seen.Members)
	if err != nil {
		return nil, errs.Wrap(err, errs.CodeOf(err), "social.GetChatSeenBy")
	}
	members := make([]api.ChatSeenMember, len(seen.Members))
	for i, id := range seen.Members {
		members[i] = api.ChatSeenMember{UserId: id.UUID()}
		if card, ok := cards[id]; ok && !card.Deleted {
			members[i].Handle, members[i].DisplayName = optionalWireText(card.Handle), card.DisplayName
			members[i].PhotoUrl = optionalWireText(card.PhotoURL)
		}
	}
	return api.GetChatSeenBy200JSONResponse{Count: len(members), Members: members}, nil
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
	return wireChatWith(cards, messages), nil
}

func wireChatWith(cards map[ids.UserID]app.UserCard, messages []app.ChatMessage) []api.ChatMessage {
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
			ProposalId: optionalWireID(m.ProposalID), Deleted: m.Deleted, SeenCount: m.SeenCount,
		}
		if !m.Deleted {
			out[i].Body = &m.Body
		}
	}
	return out
}

func (h HTTP) CreateRealtimeToken(
	ctx context.Context, _ api.CreateRealtimeTokenRequestObject,
) (api.CreateRealtimeTokenResponseObject, error) {
	me, err := caller(ctx)
	if err != nil {
		return nil, err
	}
	token, err := h.Token.Handle(ctx, me)
	if err != nil {
		return nil, err
	}
	return api.CreateRealtimeToken200JSONResponse{
		KeyName: token.KeyName, ClientId: token.ClientID, Capability: token.Capability,
		Timestamp: token.Timestamp, Ttl: token.TTL, Nonce: token.Nonce, Mac: token.MAC,
	}, nil
}
