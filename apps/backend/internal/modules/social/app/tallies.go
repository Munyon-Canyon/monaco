package app

import (
	"context"

	governanceport "github.com/monaco/monaco/apps/backend/internal/modules/governance/port"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

type Tally = governanceport.Tally

type Tallies interface {
	Tallies(ctx context.Context, proposals []ids.ProposalID) (map[ids.ProposalID]Tally, error)
}
