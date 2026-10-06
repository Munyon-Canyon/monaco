package app

import (
	"context"

	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

type ProposerReader interface {
	Proposer(ctx context.Context, id ids.ProposalID) (ids.UserID, error)
}
