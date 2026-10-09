package adapters

import (
	"context"

	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/governance/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/governance/domain"
	api "github.com/monaco/monaco/apps/backend/internal/platform/httpx/api/governanceapi"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

func (h HTTP) VoidAdminProposal(
	ctx context.Context, req api.VoidAdminProposalRequestObject,
) (api.VoidAdminProposalResponseObject, error) {
	text, err := events.NewReason(req.Body.Reason)
	if err != nil {
		return nil, err
	}
	reason := domain.VoidReason(text.String())
	if err := h.Void.Handle(ctx, app.VoidProposal{ProposalID: ids.ProposalIDFrom(req.Id), Reason: reason}); err != nil {
		return nil, err
	}
	return api.VoidAdminProposal200JSONResponse{
		Id: req.Id, Status: api.VoidedProposalStatusVoided, VoidReason: string(reason),
	}, nil
}
