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
	Vote *app.CastVoteHandler
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
		Tally: api.Tally{
			Yes: got.Tally.Yes, No: got.Tally.No, Voters: got.Tally.Voters, Needed: got.Tally.Needed,
		},
	}, nil
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
