package adapters

import (
	"context"
	"fmt"

	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

type HintPublisher interface {
	PublishHint(ctx context.Context, key string, payload []byte)
}

type Hints struct {
	Publish HintPublisher
}

func (h Hints) ProposalCreated(ctx context.Context, cabalID ids.CabalID, proposalID ids.ProposalID) {
	h.publish(ctx, cabalID, "proposal_created", proposalID)
}

func (h Hints) ProposalUpdated(ctx context.Context, cabalID ids.CabalID, proposalID ids.ProposalID) {
	h.publish(ctx, cabalID, "proposal_updated", proposalID)
}

func (h Hints) publish(ctx context.Context, cabalID ids.CabalID, kind string, proposalID ids.ProposalID) {
	h.Publish.PublishHint(ctx, "cabal."+cabalID.String()+"."+kind,
		fmt.Appendf(nil, `{"proposal_id":%q}`, proposalID.String()))
}
