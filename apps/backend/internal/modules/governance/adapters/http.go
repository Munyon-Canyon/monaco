package adapters

import (
	"context"
	"log/slog"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/governance/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/governance/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/auth"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpx"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpx/api"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

type HTTP struct {
	Vote     *app.CastVoteHandler
	Withdraw *app.WithdrawProposalHandler
	Reads    *app.ProposalReads
}

var _ httpx.GovernanceRoutes = HTTP{}

func (h HTTP) PostProposalVote(
	ctx context.Context, req api.PostProposalVoteRequestObject,
) (api.PostProposalVoteResponseObject, error) {
	voter, err := caller(ctx)
	if err != nil {
		return nil, err
	}
	choice, err := domain.ParseChoice(string(req.Body.Choice))
	if err != nil {
		return nil, err
	}
	got, err := h.Vote.Handle(ctx, app.CastVote{
		ProposalID: ids.ProposalIDFrom(req.Id), VoterID: voter, Choice: choice,
	})
	if err != nil {
		return nil, err
	}
	return api.PostProposalVote200JSONResponse{
		ProposalId: got.ProposalID.UUID(),
		Status:     api.ProposalStatus(got.Status),
		MyBallot:   api.BallotChoice(got.MyBallot),
		Tally:      wireTally(got.Tally),
	}, nil
}

func (h HTTP) GetCabalProposals(
	ctx context.Context, req api.GetCabalProposalsRequestObject,
) (api.GetCabalProposalsResponseObject, error) {
	user, err := caller(ctx)
	if err != nil {
		return nil, err
	}
	list := app.ListProposals{CabalID: ids.CabalIDFrom(req.Id), Caller: user}
	if req.Params.Filter != nil {
		list.Filter = app.Filter(*req.Params.Filter)
	}
	if req.Params.Limit != nil {
		list.Limit = *req.Params.Limit
	}
	if req.Params.Cursor != nil {
		list.Cursor = *req.Params.Cursor
	}
	page, err := h.Reads.List(ctx, list)
	if err != nil {
		return nil, err
	}
	out := api.GetCabalProposals200JSONResponse{Proposals: make([]api.Proposal, len(page.Items))}
	for i, v := range page.Items {
		out.Proposals[i] = wireProposal(v)
	}
	if page.NextCursor != "" {
		out.NextCursor = &page.NextCursor
	}
	return out, nil
}

func (h HTTP) GetProposal(
	ctx context.Context, req api.GetProposalRequestObject,
) (api.GetProposalResponseObject, error) {
	user, err := caller(ctx)
	if err != nil {
		return nil, err
	}
	out, err := h.detail(ctx, ids.ProposalIDFrom(req.Id), user)
	if err != nil {
		return nil, err
	}
	return api.GetProposal200JSONResponse(out), nil
}

func (h HTTP) DeleteProposal(
	ctx context.Context, req api.DeleteProposalRequestObject,
) (api.DeleteProposalResponseObject, error) {
	user, err := caller(ctx)
	if err != nil {
		return nil, err
	}
	id := ids.ProposalIDFrom(req.Id)
	if err := h.Withdraw.Handle(ctx, app.WithdrawProposal{ProposalID: id, ActorID: user}); err != nil {
		return nil, err
	}
	out, err := h.detail(ctx, id, user)
	if err != nil {
		return nil, err
	}
	return api.DeleteProposal200JSONResponse(out), nil
}

func (h HTTP) detail(ctx context.Context, id ids.ProposalID, user ids.UserID) (api.ProposalDetail, error) {
	got, err := h.Reads.Get(ctx, app.GetProposal{ID: id, Caller: user})
	if err != nil {
		return api.ProposalDetail{}, err
	}
	p := wireProposal(got.Proposal)
	out := api.ProposalDetail{
		Id: p.Id, CabalId: p.CabalId, ProposerId: p.ProposerId, Kind: p.Kind, Symbol: p.Symbol,
		UsdcMicros: p.UsdcMicros, TokenAmount: p.TokenAmount, QuoteOutAmount: p.QuoteOutAmount, Thesis: p.Thesis,
		Status: p.Status, StatusReason: p.StatusReason, StatusMessage: p.StatusMessage, ExpiresAt: p.ExpiresAt,
		CreatedAt: p.CreatedAt, Tally: p.Tally, MyBallot: p.MyBallot,
		Voters: make([]api.ProposalVoter, len(got.Voters)), CanVote: got.CanVote, CanWithdraw: got.CanWithdraw,
	}
	for i, v := range got.Voters {
		out.Voters[i] = api.ProposalVoter{UserId: v.UserID.UUID()}
		if v.Choice != "" {
			choice, at := api.BallotChoice(v.Choice), v.CastAt
			out.Voters[i].Choice, out.Voters[i].CastAt = &choice, &at
		}
	}
	if s := got.Swap; s != nil {
		out.Swap = &api.LinkedSwap{
			SwapId: s.ID.UUID(), Status: api.LinkedSwapStatus(s.Status), Retryable: s.Retryable,
			FailureCode: present(string(s.FailureCode)), TxSignature: present(string(s.TxSignature)),
		}
		if s.Status == "failed" {
			out.Swap.FailureMessage = ptr(errs.Message(errs.CodeSwapFailed))
		}
	}
	return out, nil
}

func (h HTTP) GetMyPendingVotes(
	ctx context.Context, _ api.GetMyPendingVotesRequestObject,
) (api.GetMyPendingVotesResponseObject, error) {
	user, err := caller(ctx)
	if err != nil {
		return nil, err
	}
	pending, err := h.Reads.PendingVotes(ctx, user)
	if err != nil {
		return nil, err
	}
	out := make(api.GetMyPendingVotes200JSONResponse, len(pending))
	for i, v := range pending {
		out[i] = api.PendingVote{
			ProposalId: v.ProposalID.UUID(), CabalId: v.CabalID.UUID(), Kind: api.ProposalKind(v.Kind),
			Symbol: v.Symbol, ExpiresAt: v.ExpiresAt,
		}
	}
	return out, nil
}

func wireProposal(v app.ProposalView) api.Proposal {
	out := api.Proposal{
		Id: v.ID.UUID(), CabalId: v.CabalID.UUID(), ProposerId: v.ProposerID.UUID(), Kind: api.ProposalKind(v.Kind),
		Symbol: v.Symbol, UsdcMicros: positive(v.USDCMicros), TokenAmount: positive(v.TokenAmount),
		QuoteOutAmount: v.QuoteOut, Status: api.ProposalStatus(v.Status), ExpiresAt: v.ExpiresAt,
		CreatedAt: v.CreatedAt, Tally: wireTally(v.Tally),
	}
	out.Thesis = present(v.Thesis)
	if v.StatusReason != "" {
		reason, message := string(v.StatusReason), errs.Message(v.StatusReason)
		out.StatusReason, out.StatusMessage = &reason, &message
	}
	if v.MyBallot != "" {
		ballot := api.BallotChoice(v.MyBallot)
		out.MyBallot = &ballot
	}
	return out
}

func wireTally(t app.Tally) api.Tally {
	return api.Tally{Yes: t.Yes, No: t.No, Voters: t.Voters, Needed: t.Needed}
}

func present(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func ptr[T any](v T) *T { return &v }

func positive(n int64) *int64 {
	if n <= 0 {
		return nil
	}
	return &n
}

func caller(ctx context.Context) (ids.UserID, error) {
	const op = "governance.caller"
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
