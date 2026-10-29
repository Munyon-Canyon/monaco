package adapters

import (
	"context"
	"math"

	"github.com/google/uuid"
	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/monaco/monaco/apps/backend/internal/modules/notify/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/notify/domain"
	baseapi "github.com/monaco/monaco/apps/backend/internal/platform/httpx/api"
	api "github.com/monaco/monaco/apps/backend/internal/platform/httpx/api/notifyapi"
)

func (h HTTP) GetMyCatchUp(
	ctx context.Context,
	req api.GetMyCatchUpRequestObject,
) (api.GetMyCatchUpResponseObject, error) {
	user, err := caller(ctx)
	if err != nil {
		return nil, err
	}
	since := h.Clock.Now().Add(-domain.CatchUpDay)
	if req.Params.Since != nil {
		since = *req.Params.Since
	}
	got, err := h.CatchUp.Read(ctx, user, since)
	if err != nil {
		return nil, err
	}
	return api.GetMyCatchUp200JSONResponse{
		Count: got.Count, Cabals: wireCabals(got.Cabals), Waiting: wireItems(got.Waiting),
		Replies: wireItems(got.Replies), Invites: wireItems(got.Invites), Results: wireItems(got.Results),
		Cleared: wireItems(got.Cleared),
	}, nil
}

func (h HTTP) GetMyCatchUpCount(
	ctx context.Context, _ api.GetMyCatchUpCountRequestObject,
) (api.GetMyCatchUpCountResponseObject, error) {
	user, err := caller(ctx)
	if err != nil {
		return nil, err
	}
	count, err := h.CatchUp.Count(ctx, user)
	if err != nil {
		return nil, err
	}
	return api.GetMyCatchUpCount200JSONResponse{Count: count}, nil
}

func (h HTTP) ReadCatchUpItem(
	ctx context.Context, req api.ReadCatchUpItemRequestObject,
) (api.ReadCatchUpItemResponseObject, error) {
	user, err := caller(ctx)
	if err != nil {
		return nil, err
	}
	if err := h.CatchUpWrites.Read(ctx, app.ReadItem{User: user, Key: req.Key}); err != nil {
		return nil, err
	}
	return api.ReadCatchUpItem204Response{}, nil
}

func (h HTTP) ClearCatchUpItem(
	ctx context.Context, req api.ClearCatchUpItemRequestObject,
) (api.ClearCatchUpItemResponseObject, error) {
	user, err := caller(ctx)
	if err != nil {
		return nil, err
	}
	if err := h.CatchUpWrites.Clear(ctx, app.ClearItem{User: user, Key: req.Key}); err != nil {
		return nil, err
	}
	return api.ClearCatchUpItem204Response{}, nil
}

func (h HTTP) KeepCatchUpItem(
	ctx context.Context, req api.KeepCatchUpItemRequestObject,
) (api.KeepCatchUpItemResponseObject, error) {
	user, err := caller(ctx)
	if err != nil {
		return nil, err
	}
	if err := h.CatchUpWrites.Keep(ctx, app.KeepItem{User: user, Key: req.Key}); err != nil {
		return nil, err
	}
	return api.KeepCatchUpItem204Response{}, nil
}

func wireCabals(in []domain.CatchUpCabal) []api.CatchUpCabal {
	out := make([]api.CatchUpCabal, len(in))
	for i, c := range in {
		out[i] = wireCabal(c)
	}
	return out
}

func wireCabal(c domain.CatchUpCabal) api.CatchUpCabal {
	return api.CatchUpCabal{Id: uid(c.ID), Name: c.Name, PictureUrl: present(c.PictureURL)}
}

func wirePerson(p domain.CatchUpPerson) api.CatchUpPerson {
	return api.CatchUpPerson{Id: uid(p.ID), Name: p.Name, PhotoUrl: present(p.PhotoURL)}
}

func wireItems(in []domain.CatchUpItem) []api.CatchUpItem {
	out := make([]api.CatchUpItem, len(in))
	for i, item := range in {
		out[i] = wireItem(item)
	}
	return out
}

func wireItem(in domain.CatchUpItem) api.CatchUpItem {
	out := api.CatchUpItem{Key: in.Key, Kind: api.CatchUpItemKind(in.Kind), Unread: in.Unread, At: in.At}
	if in.ClearedReason != "" {
		reason := api.CatchUpItemClearedReason(in.ClearedReason)
		out.ClearedReason = &reason
	}
	if in.Actor != nil {
		p := wirePerson(*in.Actor)
		out.Actor = &p
	}
	if in.Cabal != nil {
		c := wireCabal(*in.Cabal)
		out.Cabal = &c
	}
	if in.Proposal != nil {
		p := wireProposal(*in.Proposal)
		out.Proposal = &p
	}
	if in.Post != nil {
		out.Post = &api.CatchUpPost{
			FeedItemId: uid(in.Post.FeedItemID), Kind: in.Post.Kind, Label: in.Post.Label, ActorName: in.Post.ActorName,
		}
	}
	if in.Comment != nil {
		c := wireComment(*in.Comment)
		out.Comment = &c
	}
	if in.Quote != nil {
		out.Quote = &api.CatchUpQuote{AuthorName: in.Quote.AuthorName, IsYou: in.Quote.IsYou, Text: in.Quote.Text}
	}
	if in.Invite != nil {
		out.Invite = &api.CatchUpInvite{
			Id: uid(
				in.Invite.ID,
			),
			MemberCount:    in.Invite.MemberCount,
			TopHoldingName: present(in.Invite.TopHoldingName),
		}
	}
	return out
}

func wireProposal(p domain.CatchUpProposal) api.CatchUpProposal {
	out := api.CatchUpProposal{
		Id: uid(p.ID), Kind: p.Kind, Symbol: p.Symbol, AssetName: p.AssetName, AssetLogoUrl: present(p.AssetLogoURL),
		Proposer: wirePerson(p.Proposer), CreatedAt: p.CreatedAt, ExpiresAt: p.ExpiresAt, Status: p.Status,
		Yes: p.Yes, No: p.No, Voted: p.Voted, Eligible: p.Eligible, ReplyCount: p.ReplyCount, Filled: p.Filled,
	}
	out.UsdcMicros, out.TokenAmount = amount(p.USDCMicros), amount(p.TokenAmount)
	if p.Outcome != "" {
		o := api.CatchUpProposalOutcome(p.Outcome)
		out.Outcome = &o
	}
	if p.YourChoice != "" {
		c := api.CatchUpProposalYourChoice(p.YourChoice)
		out.YourChoice = &c
	}
	return out
}

func amount(n *uint64) *int64 {
	if n == nil || *n > math.MaxInt64 {
		return nil
	}
	v := int64(*n)
	return &v
}

func wireComment(c domain.CatchUpComment) api.CatchUpComment {
	return api.CatchUpComment{
		FeedItemId: uid(c.FeedItemID), CommentId: uid(c.CommentID), ParentCommentId: uidPtr(c.ParentCommentID),
		RootCommentId: uid(c.RootCommentID), ProposalId: uidPtr(c.ProposalID), CabalId: uidPtr(c.CabalID),
		Author: wirePerson(c.Author), Body: c.Body, CreatedAt: c.CreatedAt,
		Reactions: wireReactions(c.Reactions), ReactionCount: c.ReactionCount,
	}
}

func uid(s string) openapi_types.UUID {
	id, _ := uuid.Parse(s)
	return id
}

func uidPtr(s string) *openapi_types.UUID {
	if s == "" {
		return nil
	}
	id := uid(s)
	return &id
}

func present(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func wireReactions(in []domain.CatchUpReaction) []baseapi.Reaction {
	out := make([]baseapi.Reaction, len(in))
	for i, r := range in {
		out[i] = baseapi.Reaction{Emoji: r.Emoji, Count: r.Count, Mine: r.Mine}
	}
	return out
}
