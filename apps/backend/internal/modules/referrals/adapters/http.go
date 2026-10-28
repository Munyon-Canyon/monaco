package adapters

import (
	"context"
	"log/slog"
	"math"
	"strconv"

	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/referrals/app"
	"github.com/monaco/monaco/apps/backend/internal/platform/auth"
	api "github.com/monaco/monaco/apps/backend/internal/platform/httpx/api/referralsapi"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

type HTTP struct {
	Codes  app.Resolver
	Attach *app.AttachReferralHandler
	Click  *app.RecordReferralClickHandler
	Cabals *app.InvitePreviewQuery
}

var _ api.StrictServerInterface = HTTP{}

func (h HTTP) GetMyReferralCode(
	ctx context.Context, _ api.GetMyReferralCodeRequestObject,
) (api.GetMyReferralCodeResponseObject, error) {
	user, err := caller(ctx)
	if err != nil {
		return nil, err
	}
	mine, err := h.Codes.MyCode(ctx, user)
	if err != nil {
		return nil, err
	}
	return api.GetMyReferralCode200JSONResponse{
		Code: mine.Code, Link: mine.Link, HandleLink: mine.HandleLink, HandleUnlocked: mine.HandleUnlocked,
	}, nil
}

func (h HTTP) PostMeReferral(
	ctx context.Context, req api.PostMeReferralRequestObject,
) (api.PostMeReferralResponseObject, error) {
	const op = "referrals.PostMeReferral"
	user, err := caller(ctx)
	if err != nil {
		return nil, err
	}
	if req.Body == nil || !req.Body.Source.Valid() || req.Body.Code == "" {
		return nil, errs.New(errs.CodeInvalidInput, op)
	}
	cmd := app.AttachReferral{CallerID: user, Code: req.Body.Code, Source: string(req.Body.Source)}
	if err := h.Attach.Handle(ctx, cmd); err != nil {
		return nil, err
	}
	referrer, err := h.referrerOf(ctx, req.Body.Code)
	if err != nil {
		return nil, err
	}
	return api.PostMeReferral201JSONResponse{Referrer: referrer}, nil
}

func (h HTTP) GetReferral(
	ctx context.Context, req api.GetReferralRequestObject,
) (api.GetReferralResponseObject, error) {
	referrer, err := h.referrerOf(ctx, req.Code)
	if errs.CodeOf(err) == errs.CodeReferralCodeUnknown {
		return lookup(nil), nil
	}
	if err != nil {
		return nil, err
	}
	return lookup(&referrer), nil
}

func (h HTTP) GetReferralCabal(
	ctx context.Context, req api.GetReferralCabalRequestObject,
) (api.GetReferralCabalResponseObject, error) {
	cabal, err := ids.ParseCabalID(req.CabalId.String())
	if err != nil {
		return nil, errs.Wrap(err, errs.CodeInvalidInput, "referrals.GetReferralCabal")
	}
	preview, err := h.Cabals.Run(ctx, req.Code, cabal)
	if err != nil {
		return nil, err
	}
	return api.GetReferralCabal200JSONResponse{
		Body:    cabalPreview(preview),
		Headers: api.GetReferralCabal200ResponseHeaders{CacheControl: "no-cache"},
	}, nil
}

func (h HTTP) RecordReferralClick(
	ctx context.Context, req api.RecordReferralClickRequestObject,
) (api.RecordReferralClickResponseObject, error) {
	if err := h.Click.Handle(ctx, app.RecordReferralClick{Code: req.Body.Code}); err != nil {
		return nil, err
	}
	return api.RecordReferralClick204Response{}, nil
}

func (h HTTP) referrerOf(ctx context.Context, code string) (api.ReferralReferrer, error) {
	resolved, err := h.Codes.Resolve(ctx, code)
	if err != nil {
		return api.ReferralReferrer{}, err
	}
	card, err := h.Codes.Referrer(ctx, resolved)
	if err != nil {
		return api.ReferralReferrer{}, err
	}
	var photo *string
	if card.PhotoURL != "" {
		photo = &card.PhotoURL
	}
	return api.ReferralReferrer{
		UserId: card.ID.UUID(), DisplayName: card.DisplayName, PhotoUrl: photo, Handle: card.Handle,
	}, nil
}

func lookup(referrer *api.ReferralReferrer) api.GetReferral200JSONResponse {
	return api.GetReferral200JSONResponse{
		Body:    api.ReferralLookup{Referrer: referrer},
		Headers: api.GetReferral200ResponseHeaders{CacheControl: "public, max-age=300"},
	}
}

func cabalPreview(p app.InvitePreview) api.ReferralCabal {
	out := api.ReferralCabal{}
	if p.Inviter != nil {
		out.Inviter = &api.ReferralCabalInviter{
			DisplayName: p.Inviter.DisplayName, PhotoUrl: photoOf(p.Inviter.PhotoURL),
		}
	}
	if c := p.Cabal; c != nil {
		first := make([]api.ReferralCabalPerson, 0, len(c.FirstMembers))
		for _, m := range c.FirstMembers {
			first = append(first, api.ReferralCabalPerson{DisplayName: m.DisplayName, PhotoUrl: photoOf(m.PhotoURL)})
		}
		out.Cabal = &api.ReferralCabalFacts{
			Id: c.ID.UUID(), Name: c.Name, PictureUrl: photoOf(c.PictureURL), MemberCount: c.MemberCount,
			Since: openapi_types.Date{Time: c.Since}, JoinMode: api.ReferralCabalCabalJoinMode(c.JoinMode),
			FirstMembers: first, PotValueMicros: c.PotValueMicros.String(),
		}
		out.Return1mBps = int32Of(c.Return1MBps)
		out.TurnoutMonthBps = int32Of(c.TurnoutBps)
		out.OpenProposal = proposalOf(c.OpenProposal)
	}
	return out
}

func int32Of(v *int64) *int32 {
	if v == nil {
		return nil
	}
	out := clampInt32(*v)
	return &out
}

func clampInt32(v int64) int32 {
	switch {
	case v > math.MaxInt32:
		return math.MaxInt32
	case v < math.MinInt32:
		return math.MinInt32
	}
	return int32(v)
}

func proposalOf(p *app.PreviewProposal) *api.ReferralCabalProposal {
	if p == nil {
		return nil
	}
	out := &api.ReferralCabalProposal{
		Id: p.ID.UUID(), Kind: p.Kind, Symbol: p.Symbol, AssetName: p.AssetName, AssetLogoUrl: photoOf(p.AssetLogoURL),
		ProposerName: p.ProposerName, Title: p.Title, Yes: p.Yes, No: p.No, Voted: p.Voted,
		Eligible: p.Eligible, ExpiresAt: p.ExpiresAt.UTC(),
	}
	if p.USDCMicros != nil {
		micros := strconv.FormatUint(*p.USDCMicros, 10)
		out.UsdcMicros = &micros
	}
	if p.Replies != nil {
		replies := clampInt32(int64(*p.Replies))
		out.Replies = &replies
	}
	return out
}

func photoOf(url string) *string {
	if url == "" {
		return nil
	}
	return &url
}

func caller(ctx context.Context) (ids.UserID, error) {
	const op = "referrals.caller"
	actor, ok := auth.ActorFrom(ctx)
	if !ok {
		return ids.UserID{}, errs.New(errs.CodeUnauthorized, op)
	}
	if actor.Kind != auth.ActorUser {
		return ids.UserID{}, errs.New(errs.CodeForbidden, op, slog.String("actor_kind", string(actor.Kind)))
	}
	user, err := ids.ParseUserID(actor.ID)
	if err != nil {
		return ids.UserID{}, errs.Wrap(err, errs.CodeUnauthorized, op)
	}
	return user, nil
}
