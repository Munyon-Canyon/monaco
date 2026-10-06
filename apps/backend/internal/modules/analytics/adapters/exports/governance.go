package exports

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/analytics/app"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

const microsPerUSDC = 1_000_000

type Proposals struct{ Proposers app.ProposerReader }

func (Proposals) ProposalPassed(_ context.Context, e events.ProposalPassed) (app.Capture, bool, error) {
	return app.Capture{
		Event:      "proposal_passed",
		DistinctID: e.ProposerID.String(),
		Properties: proposalProps(e.ProposalID, e.CabalID, map[string]any{
			"kind": e.Kind, "symbol": e.Symbol, "usdc_amount": usdc(e.USDCMicros),
		}),
	}, true, nil
}

func (p Proposals) ProposalFailed(ctx context.Context, e events.ProposalFailed) (app.Capture, bool, error) {
	return p.byProposer(ctx, "proposal_failed", e.ProposalID, e.CabalID, nil)
}

func (p Proposals) ProposalExpired(ctx context.Context, e events.ProposalExpired) (app.Capture, bool, error) {
	return p.byProposer(ctx, "proposal_expired", e.ProposalID, e.CabalID, nil)
}

func (p Proposals) byProposer(
	ctx context.Context, event string, proposal, cabal uuid.UUID, extra map[string]any,
) (app.Capture, bool, error) {
	proposer, err := p.Proposers.Proposer(ctx, ids.ProposalIDFrom(proposal))
	if err != nil {
		return app.Capture{}, false, err
	}
	return app.Capture{
		Event: event, DistinctID: proposer.String(), Properties: proposalProps(proposal, cabal, extra),
	}, true, nil
}

func proposalProps(proposal, cabal uuid.UUID, extra map[string]any) map[string]any {
	props := map[string]any{"proposal_id": proposal.String(), "cabal_id": cabal.String()}
	maps.Copy(props, extra)
	return props
}

func usdc(m money.Micros) json.Number {
	return json.Number(fmt.Sprintf("%d.%06d", m.Uint64()/microsPerUSDC, m.Uint64()%microsPerUSDC))
}
