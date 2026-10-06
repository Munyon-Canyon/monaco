package port

import (
	"context"

	"github.com/monaco/monaco/apps/backend/internal/modules/governance/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

type Status = domain.Status

const StatusPassed = domain.StatusPassed

type Queries interface {
	Status(ctx context.Context, id ids.ProposalID) (Status, error)
	Proposer(ctx context.Context, id ids.ProposalID) (ids.UserID, error)
}

type ProposedMints interface {
	ProposedMints(ctx context.Context) ([]chain.SolanaAddress, error)
}
