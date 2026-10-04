package app

import (
	"context"

	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

type Hints interface {
	ProposalCreated(ctx context.Context, cabalID ids.CabalID, proposalID ids.ProposalID)
	ProposalUpdated(ctx context.Context, cabalID ids.CabalID, proposalID ids.ProposalID)
}

type NoHints struct{}

func (NoHints) ProposalCreated(context.Context, ids.CabalID, ids.ProposalID) {}

func (NoHints) ProposalUpdated(context.Context, ids.CabalID, ids.ProposalID) {}
