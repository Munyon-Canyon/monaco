package adapters

import (
	"context"

	"github.com/monaco/monaco/apps/backend/internal/modules/governance/app"
	api "github.com/monaco/monaco/apps/backend/internal/platform/httpx/api/governanceapi"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

func (h HTTP) GetAdminProposal(
	ctx context.Context, req api.GetAdminProposalRequestObject,
) (api.GetAdminProposalResponseObject, error) {
	got, err := h.Admin.Proposal(ctx, ids.ProposalIDFrom(req.Id))
	if err != nil {
		return nil, err
	}
	return api.GetAdminProposal200JSONResponse(wireAdminProposal(got)), nil
}

func (h HTTP) GetAdminCabalProposals(
	ctx context.Context, req api.GetAdminCabalProposalsRequestObject,
) (api.GetAdminCabalProposalsResponseObject, error) {
	filter, limit, cursor := app.FilterAll, 0, ""
	if req.Params.Status != nil {
		filter = app.Filter(*req.Params.Status)
	}
	if req.Params.Limit != nil {
		limit = *req.Params.Limit
	}
	if req.Params.Cursor != nil {
		cursor = *req.Params.Cursor
	}
	page, err := h.Admin.CabalProposals(ctx, ids.CabalIDFrom(req.Id), filter, limit, cursor)
	if err != nil {
		return nil, err
	}
	out := api.GetAdminCabalProposals200JSONResponse{Proposals: make([]api.Proposal, len(page.Items))}
	for i, v := range page.Items {
		out.Proposals[i] = wireProposal(v)
	}
	out.NextCursor = present(page.NextCursor)
	return out, nil
}

func wireAdminProposal(got app.AdminProposal) api.AdminProposal {
	p := wireProposal(got.Detail.Proposal)
	out := api.AdminProposal{
		Id: p.Id, CabalId: p.CabalId,
		Proposer: api.AdminProposer{Id: got.Proposer.ID.UUID(), Handle: got.Proposer.Handle},
		Kind:     p.Kind, Symbol: p.Symbol, UsdcMicros: p.UsdcMicros, TokenAmount: p.TokenAmount,
		QuoteOutAmount: p.QuoteOutAmount, Thesis: p.Thesis, Status: p.Status, StatusReason: p.StatusReason,
		ExpiresAt: p.ExpiresAt, CreatedAt: p.CreatedAt, Tally: p.Tally,
		Voters:             make([]api.ProposalVoter, len(got.Detail.Voters)),
		StatusHistory:      make([]api.AdminStatusChange, len(got.History)),
		RecentAdminActions: make([]api.AdminProposalAction, len(got.Actions)),
	}
	for i, v := range got.Detail.Voters {
		out.Voters[i] = api.ProposalVoter{UserId: v.UserID.UUID()}
		if v.Choice != "" {
			choice, at := api.BallotChoice(v.Choice), v.CastAt
			out.Voters[i].Choice, out.Voters[i].CastAt = &choice, &at
		}
	}
	if s := got.Detail.Swap; s != nil {
		out.Swap = &api.AdminProposalSwap{
			SwapId: s.ID.UUID(), Status: api.AdminProposalSwapStatus(s.Status),
			FailureCode: present(string(s.FailureCode)), TxSignature: present(string(s.TxSignature)),
		}
	}
	for i, c := range got.History {
		out.StatusHistory[i] = api.AdminStatusChange{
			Status: api.ProposalStatus(c.Status), At: c.At, ActorType: api.AdminStatusChangeActorType(c.ActorType),
		}
	}
	for i, a := range got.Actions {
		out.RecentAdminActions[i] = api.AdminProposalAction{
			Id: a.ID, AdminId: a.AdminID, Action: a.Kind, Reason: a.Reason, CreatedAt: a.At,
		}
	}
	return out
}
