package app

import (
	"context"

	governanceport "github.com/monaco/monaco/apps/backend/internal/modules/governance/port"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

type Tally = governanceport.Tally

type ViewerBallot = governanceport.ViewerBallot

type ViewerBallots interface {
	MyBallots(
		ctx context.Context,
		viewer ids.UserID,
		proposals []ids.ProposalID,
	) (map[ids.ProposalID]ViewerBallot, error)
}

type Tallies interface {
	Tallies(ctx context.Context, proposals []ids.ProposalID) (map[ids.ProposalID]Tally, error)
}
