package adapters

import (
	"context"
	"log/slog"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/cabal/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/cabal/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/cabal/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/auth"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpx"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpx/api"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

type HTTP struct {
	Create *app.CreateCabalHandler
	DB     sqlc.DBTX
	Users  app.UserCards
}

var _ httpx.CabalRoutes = HTTP{}

func (h HTTP) PostCabal(
	ctx context.Context, req api.PostCabalRequestObject,
) (api.PostCabalResponseObject, error) {
	user, err := caller(ctx)
	if err != nil {
		return nil, err
	}
	cmd, err := createCommand(user, req)
	if err != nil {
		return nil, err
	}
	created, err := h.Create.Handle(ctx, cmd)
	if err != nil {
		return nil, err
	}
	view, err := app.GetCabal(ctx, h.DB, h.Users, created.ID, user)
	if err != nil {
		return nil, err
	}
	return api.PostCabal201JSONResponse(wireCabal(view)), nil
}

func (h HTTP) GetCabal(
	ctx context.Context, req api.GetCabalRequestObject,
) (api.GetCabalResponseObject, error) {
	user, err := caller(ctx)
	if err != nil {
		return nil, err
	}
	view, err := app.GetCabal(ctx, h.DB, h.Users, ids.CabalIDFrom(req.Id), user)
	if err != nil {
		return nil, err
	}
	return api.GetCabal200JSONResponse(wireCabal(view)), nil
}

func createCommand(user ids.UserID, req api.PostCabalRequestObject) (app.CreateCabal, error) {
	const op = "cabal.PostCabal"
	if req.Body == nil {
		return app.CreateCabal{}, errs.New(errs.CodeInvalidInput, op, slog.String("reason", "body"))
	}
	name, err := domain.ParseName(req.Body.Name)
	if err != nil {
		return app.CreateCabal{}, err
	}
	slippage := domain.DefaultSlippageBps
	if req.Body.SlippageBps != nil {
		slippage = *req.Body.SlippageBps
	}
	rules, err := domain.NewRules(
		req.Body.JoinMode, req.Body.VoterMode, req.Body.Threshold, req.Body.ProposalExpirySeconds, slippage,
	)
	if err != nil {
		return app.CreateCabal{}, err
	}
	return app.CreateCabal{
		ActorID: user, IdempotencyKey: req.Params.IdempotencyKey, Name: name, Rules: rules,
	}, nil
}

func caller(ctx context.Context) (ids.UserID, error) {
	const op = "cabal.caller"
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

func wireCabal(view app.CabalView) api.Cabal {
	members := make([]api.CabalMember, 0, len(view.Members))
	for _, member := range view.Members {
		members = append(members, api.CabalMember{
			UserId: member.UserID, Handle: nullString(member.Handle), DisplayName: member.DisplayName,
			PhotoUrl: nullString(member.PhotoURL), Role: member.Role, CanVote: member.CanVote,
			JoinedAt: member.JoinedAt,
		})
	}
	return api.Cabal{
		Id: view.ID.UUID(), Name: view.Name, PictureUrl: view.PictureURL, Status: view.Status,
		Rules: api.CabalRules{
			JoinMode: view.JoinMode, VoterMode: view.VoterMode, Threshold: view.Threshold,
			ProposalExpirySeconds: view.ExpirySeconds, SlippageBps: view.SlippageBps,
		},
		Creator: api.CabalPerson{
			UserId: view.Creator.UserID, Handle: nullString(view.Creator.Handle),
			DisplayName: view.Creator.DisplayName, PhotoUrl: nullString(view.Creator.PhotoURL),
		},
		MemberCount: view.MemberCount, Members: members, Me: wireMe(view.Me),
		MyAccessRequest: wireAccess(view.Access), InviteCode: view.InviteCode,
		TreasuryAddress: string(view.TreasuryAddress),
	}
}

func wireMe(me *app.Membership) *api.CabalMembership {
	if me == nil {
		return nil
	}
	return &api.CabalMembership{Role: me.Role, CanVote: me.CanVote}
}

func wireAccess(access *app.Access) *api.CabalAccess {
	if access == nil {
		return nil
	}
	return &api.CabalAccess{Id: access.ID, Direction: access.Direction, Status: access.Status}
}

func nullString(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}
