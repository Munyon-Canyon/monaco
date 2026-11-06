package adapters

import (
	"context"
	"log/slog"
	"slices"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/domain"
	baseapi "github.com/monaco/monaco/apps/backend/internal/platform/httpx/api"
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
	wire, err := h.wireChat(ctx, me, []app.ChatMessage{posted})
	if err != nil {
		observability.Degraded(ctx, observability.SocialChatAuthorUnreadable,
			slog.String("message_id", posted.ID.String()), slog.String("cause", err.Error()))
		wire = wireChatWith(nil, nil, []app.ChatMessage{posted})
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
		Limit: limitOr(req.Params.Limit), Moments: req.Params.Moments != nil && *req.Params.Moments,
	}
	messages, err := app.ListChatChannel(ctx, h.Reads, h.Members, q)
	if err != nil {
		return nil, err
	}
	wire, err := h.wireChat(ctx, me, messages)
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
		Limit: limitOr(req.Params.Limit), Moments: req.Params.Moments != nil && *req.Params.Moments,
	})
	if err != nil {
		return nil, err
	}
	wire, err := h.wireChat(ctx, me, append([]app.ChatMessage{thread.Parent}, thread.Replies...))
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
	var att *domain.AttachmentInput
	if a := body.Attachment; a != nil {
		parsed, err := attachmentInput(a)
		if err != nil {
			return app.PostChatMessage{}, err
		}
		att = &parsed
	}
	input, err := domain.ParseChatInput(body.Body, att)
	if err != nil {
		return app.PostChatMessage{}, err
	}
	cmd := app.PostChatMessage{CabalID: cabal, Author: me, Input: input}
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

func (h HTTP) wireChat(ctx context.Context, viewer ids.UserID, messages []app.ChatMessage) ([]api.ChatMessage, error) {
	reactions, err := app.ChatMessageReactions(ctx, h.Reads, viewer, messages)
	if err != nil {
		return nil, err
	}
	if err := app.LoadCommentViews(ctx, h.Reads, h.Stocks, viewer, messages, h.Clock.Now()); err != nil {
		return nil, err
	}
	if err := app.LoadChatAttachments(ctx, h.Reads, h.Stocks, messages); err != nil {
		return nil, err
	}
	if err := app.LoadChatPolls(ctx, h.Reads, viewer, messages, h.Clock.Now()); err != nil {
		return nil, err
	}
	return wireChat(ctx, h.Users, reactions, messages)
}

func (h HTTP) VoteChatPoll(
	ctx context.Context, req api.VoteChatPollRequestObject,
) (api.VoteChatPollResponseObject, error) {
	me, err := caller(ctx)
	if err != nil {
		return nil, err
	}
	if req.Body == nil {
		return nil, errs.New(errs.CodeInvalidInput, "social.VoteChatPoll")
	}
	state, err := h.VoteChatPollCmd.Handle(ctx, app.VoteChatPoll{
		CabalID: ids.CabalIDFrom(req.Id), MessageID: req.MessageId, Viewer: me, Position: req.Body.Position,
	})
	if err != nil {
		return nil, err
	}
	return api.VoteChatPoll200JSONResponse(wirePollState(state)), nil
}

func (h HTTP) UnvoteChatPoll(
	ctx context.Context, req api.UnvoteChatPollRequestObject,
) (api.UnvoteChatPollResponseObject, error) {
	me, err := caller(ctx)
	if err != nil {
		return nil, err
	}
	state, err := h.UnvoteChatPollCmd.Handle(ctx, app.UnvoteChatPoll{
		CabalID: ids.CabalIDFrom(req.Id), MessageID: req.MessageId, Viewer: me,
	})
	if err != nil {
		return nil, err
	}
	return api.UnvoteChatPoll200JSONResponse(wirePollState(state)), nil
}

func (h HTTP) PinChatMessage(
	ctx context.Context, req api.PinChatMessageRequestObject,
) (api.PinChatMessageResponseObject, error) {
	me, err := caller(ctx)
	if err != nil {
		return nil, err
	}
	pinned, err := h.PinChat.Handle(ctx, app.PinChatMessage{
		CabalID: ids.CabalIDFrom(req.Id), MessageID: req.MessageId, Caller: me,
	})
	if err != nil {
		return nil, err
	}
	card := []app.ChatMessage{pinned}
	if err := app.WithReplies(ctx, h.Reads, me, card); err != nil {
		return nil, err
	}
	wire, err := h.wireChat(ctx, me, card)
	if err != nil {
		return nil, err
	}
	return api.PinChatMessage200JSONResponse(wire[0]), nil
}

func (h HTTP) UnpinChatMessage(
	ctx context.Context, req api.UnpinChatMessageRequestObject,
) (api.UnpinChatMessageResponseObject, error) {
	me, err := caller(ctx)
	if err != nil {
		return nil, err
	}
	unpinned, err := h.UnpinChat.Handle(ctx, app.UnpinChatMessage{
		CabalID: ids.CabalIDFrom(req.Id), MessageID: req.MessageId, Caller: me,
	})
	if err != nil {
		return nil, err
	}
	card := []app.ChatMessage{unpinned}
	if err := app.WithReplies(ctx, h.Reads, me, card); err != nil {
		return nil, err
	}
	wire, err := h.wireChat(ctx, me, card)
	if err != nil {
		return nil, err
	}
	return api.UnpinChatMessage200JSONResponse(wire[0]), nil
}

func (h HTTP) GetChatPins(
	ctx context.Context, req api.GetChatPinsRequestObject,
) (api.GetChatPinsResponseObject, error) {
	me, err := caller(ctx)
	if err != nil {
		return nil, err
	}
	pins, err := app.ListChatPins(ctx, h.Reads, h.Members, ids.CabalIDFrom(req.Id), me)
	if err != nil {
		return nil, err
	}
	if err := app.WithReplies(ctx, h.Reads, me, pins); err != nil {
		return nil, err
	}
	wire, err := h.wireChat(ctx, me, pins)
	if err != nil {
		return nil, err
	}
	return api.GetChatPins200JSONResponse(api.ChatPins{Messages: wire}), nil
}

func (h HTTP) ReactToChatMessage(
	ctx context.Context, req api.ReactToChatMessageRequestObject,
) (api.ReactToChatMessageResponseObject, error) {
	me, err := caller(ctx)
	if err != nil {
		return nil, err
	}
	summary, err := h.ReactChat.Handle(ctx, app.ReactChatMessage{
		CabalID: ids.CabalIDFrom(req.Id), MessageID: req.MessageId, Viewer: me, Emoji: req.Emoji,
	})
	if err != nil {
		return nil, err
	}
	return api.ReactToChatMessage200JSONResponse(wireReactionSummary(summary)), nil
}

func (h HTTP) UnreactToChatMessage(
	ctx context.Context, req api.UnreactToChatMessageRequestObject,
) (api.UnreactToChatMessageResponseObject, error) {
	me, err := caller(ctx)
	if err != nil {
		return nil, err
	}
	summary, err := h.UnreactChat.Handle(ctx, app.UnreactChatMessage{
		CabalID: ids.CabalIDFrom(req.Id), MessageID: req.MessageId, Viewer: me, Emoji: req.Emoji,
	})
	if err != nil {
		return nil, err
	}
	return api.UnreactToChatMessage200JSONResponse(wireReactionSummary(summary)), nil
}

func ChatWire(users app.Users) app.ChatWire {
	return func(ctx context.Context, m app.ChatMessage) (any, error) {
		wire, err := wireChat(ctx, users, nil, []app.ChatMessage{m})
		if err != nil {
			return nil, err
		}
		return wire[0], nil
	}
}

func wireChat(
	ctx context.Context, users app.Users, reactions map[uuid.UUID]app.ReactionSummary, messages []app.ChatMessage,
) ([]api.ChatMessage, error) {
	authors := make([]ids.UserID, 0, len(messages))
	for _, m := range messages {
		for _, id := range cardsNeeded(m) {
			if !slices.Contains(authors, id) {
				authors = append(authors, id)
			}
		}
	}
	cards, err := users.UsersByID(ctx, authors)
	if err != nil {
		return nil, errs.Wrap(err, errs.CodeOf(err), "social.wireChat")
	}
	return wireChatWith(cards, reactions, messages), nil
}

func wireChatWith(
	cards map[ids.UserID]app.UserCard, reactions map[uuid.UUID]app.ReactionSummary, messages []app.ChatMessage,
) []api.ChatMessage {
	out := make([]api.ChatMessage, len(messages))
	for i, m := range messages {
		out[i] = api.ChatMessage{
			Id:            m.ID,
			Kind:          api.ChatMessageKind(m.Kind),
			Author:        wireChatAuthor(cards, m.AuthorID),
			CreatedAt:     m.CreatedAt,
			ParentId:      optionalWireID(m.ParentID),
			AlsoInChannel: m.AlsoInChannel,
			ReplyCount:    int(m.ReplyCount),
			LastReplyAt:   m.LastReplyAt,
			ProposalId:    optionalWireID(m.ProposalID),
			Deleted:       m.Deleted,
			SeenCount:     m.SeenCount,
			Reactions:     []baseapi.Reaction{},
			LastRepliers:  wireLastRepliers(cards, m.LastRepliers),
			PinnedAt:      m.PinnedAt,
			CommentId:     optionalWireID(m.CommentID),
		}
		if m.ReplyTo != nil {
			out[i].ReplyTo = wireQuote(m.ReplyTo, people{cards: cards})
		}
		if m.PinnedAt != nil {
			pinner := wireChatAuthor(cards, m.PinnedBy)
			out[i].PinnedBy = &pinner
		}
		switch {
		case m.Kind == domain.ChatKindSystem:
			out[i].Moment = wireMoment(cards, m.Moment)
		case !m.Deleted:
			out[i].Body, out[i].Attachment = wireChatBody(m)
		}
		if m.Kind == domain.ChatKindSystem || !m.Deleted {
			summary := reactions[m.ID]
			out[i].Reactions, out[i].ReactionCount = wireReactions(summary.Reactions), summary.Total
		}
	}
	return out
}

func wireLastRepliers(cards map[ids.UserID]app.UserCard, repliers []ids.UserID) []api.ChatAuthor {
	out := make([]api.ChatAuthor, len(repliers))
	for i, id := range repliers {
		out[i] = wireChatAuthor(cards, id)
	}
	return out
}

func wireChatBody(m app.ChatMessage) (*string, *api.CommentAttachment) {
	body := m.Body
	if m.Kind == domain.ChatKindProposal && body == "" && m.Attachment == nil {
		return nil, nil
	}
	if m.Attachment == nil {
		return &body, nil
	}
	if body == "" {
		body = m.Attachment.Display().String()
	}
	return &body, wireAttachment(m.Attachment, m.Stock, m.Poll)
}

func cardsNeeded(m app.ChatMessage) []ids.UserID {
	need := append([]ids.UserID{m.AuthorID}, m.LastRepliers...)
	if m.ReplyTo != nil {
		need = append(need, m.ReplyTo.AuthorID)
	}
	if m.PinnedAt != nil {
		need = append(need, m.PinnedBy)
	}
	if m.Moment != nil {
		need = append(need, momentPeople(m.Moment)...)
	}
	return need
}

func momentPeople(m *domain.ChatMoment) []ids.UserID {
	switch m.Kind() {
	case domain.MomentMemberJoined:
		return []ids.UserID{m.Member()}
	case domain.MomentTradeFilled:
		return []ids.UserID{m.Trade().ProposerID}
	case domain.MomentProposalSettled:
		return []ids.UserID{m.Settled().ProposerID}
	case domain.MomentCabalCreated:
		return nil
	default:
		return nil
	}
}

func wireChatAuthor(cards map[ids.UserID]app.UserCard, id ids.UserID) api.ChatAuthor {
	author := api.ChatAuthor{Id: id.UUID()}
	if card, ok := cards[id]; ok && !card.Deleted {
		author.Handle, author.DisplayName = optionalWireText(card.Handle), card.DisplayName
		author.PhotoUrl = optionalWireText(card.PhotoURL)
	}
	return author
}

func wireMoment(cards map[ids.UserID]app.UserCard, m *domain.ChatMoment) *api.ChatMoment {
	var out api.ChatMoment
	switch m.Kind() {
	case domain.MomentCabalCreated:
		_ = out.FromCabalCreatedMoment(api.CabalCreatedMoment{Kind: api.CabalCreated})
	case domain.MomentMemberJoined:
		_ = out.FromMemberJoinedMoment(api.MemberJoinedMoment{
			Kind: api.MemberJoined, Member: wireChatAuthor(cards, m.Member()),
		})
	case domain.MomentTradeFilled:
		t := m.Trade()
		trade := api.TradeFilledMoment{
			Kind: api.TradeFilled, Side: api.TradeFilledMomentSide(t.Side), Symbol: t.Symbol,
			AssetId: t.AssetID, PriceMicros: t.PriceMicros, UsdcMicros: t.USDCMicros, FilledAt: t.FilledAt,
			SwapId: t.SwapID, ProposalId: optionalWireID(t.ProposalID),
		}
		if t.Tallied {
			trade.Yes, trade.No = &t.Yes, &t.No
		}
		if t.ProposerID.UUID() != uuid.Nil {
			proposer := wireChatAuthor(cards, t.ProposerID)
			trade.Proposer = &proposer
		}
		_ = out.FromTradeFilledMoment(trade)
	case domain.MomentProposalSettled:
		p := m.Settled()
		settled := api.ProposalSettledMoment{
			Kind: api.ProposalSettled, Outcome: api.ProposalSettledMomentOutcome(p.Outcome), ProposalId: p.ProposalID,
			Side: api.ProposalSettledMomentSide(p.Side), Symbol: p.Symbol, UsdcMicros: p.USDCMicros,
			Yes: p.Yes, No: p.No, Voters: p.Voters, Proposer: wireChatAuthor(cards, p.ProposerID),
		}
		if p.TokenAmount > 0 {
			settled.TokenAmount = &p.TokenAmount
		}
		_ = out.FromProposalSettledMoment(settled)
	}
	return &out
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
