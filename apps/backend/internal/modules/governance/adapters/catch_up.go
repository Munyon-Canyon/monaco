package adapters

import (
	"context"

	"github.com/monaco/monaco/apps/backend/internal/modules/governance/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/governance/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/governance/port"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

type CatchUp struct{ reads *app.PendingVoteReads }

var _ port.CatchUp = CatchUp{}

func NewCatchUp(reads *app.PendingVoteReads) CatchUp { return CatchUp{reads: reads} }

func (c CatchUp) PendingVotes(ctx context.Context, viewer ids.UserID) ([]port.CatchUpProposal, error) {
	pending, err := c.reads.PendingVotes(ctx, viewer)
	if err != nil {
		return nil, err
	}
	out := make([]port.CatchUpProposal, len(pending))
	for i, v := range pending {
		out[i] = port.CatchUpProposal{
			ID: v.ProposalID, CabalID: v.CabalID, CabalName: v.CabalName, Kind: string(v.Kind), Symbol: v.Symbol,
			AssetName: v.AssetName, AssetLogo: v.AssetLogo,
			USDCMicros: positiveAmount(v.USDC.Uint64()), TokenAmount: positiveAmount(v.TokenAmount.Uint64()),
			Proposer: port.CatchUpPerson{
				ID: v.Proposer.ID, Name: firstNonEmpty(v.Proposer.DisplayName, v.Proposer.Handle),
				PhotoURL: v.Proposer.PhotoURL,
			},
			CreatedAt: v.CreatedAt, ExpiresAt: v.ExpiresAt, Status: domain.StatusOpen,
			Yes: v.Tally.Yes, No: v.Tally.No, Voted: v.VotedCount, Eligible: v.Tally.Voters, ReplyCount: v.ReplyCount,
		}
	}
	return out, nil
}

func (c CatchUp) ProposalCards(
	ctx context.Context, viewer ids.UserID, proposals []ids.ProposalID,
) (map[ids.ProposalID]port.CatchUpProposal, error) {
	return c.reads.ProposalCards(ctx, viewer, proposals)
}

func positiveAmount(n uint64) *uint64 {
	if n == 0 {
		return nil
	}
	return &n
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}
