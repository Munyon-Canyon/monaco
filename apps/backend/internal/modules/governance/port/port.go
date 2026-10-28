package port

import (
	"context"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/modules/governance/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

type Status = domain.Status

const StatusPassed = domain.StatusPassed

type Queries interface {
	Status(ctx context.Context, id ids.ProposalID) (Status, error)
	Retryable(ctx context.Context, id ids.ProposalID) (bool, error)
	Proposer(ctx context.Context, id ids.ProposalID) (ids.UserID, error)
}

type Tally struct {
	Yes, No, Voters, Needed int
	ExpiresAt               time.Time
	Status                  Status
	Kind                    string
}

type Tallies interface {
	Tallies(ctx context.Context, ids []ids.ProposalID) (map[ids.ProposalID]Tally, error)
}

type Voters interface {
	Voters(ctx context.Context, id ids.ProposalID) ([]ids.UserID, error)
}

type OpenProposal struct {
	ID         ids.ProposalID
	Kind       string
	Symbol     string
	Mint       string
	USDCMicros *uint64
	ProposerID ids.UserID
	Thesis     string
	Yes        int32
	No         int32
	Voted      int32
	Eligible   int32
	ExpiresAt  time.Time
}

type InviteFacts interface {
	NewestOpenProposal(ctx context.Context, cabal ids.CabalID) (OpenProposal, bool, error)
	MonthTurnout(ctx context.Context, cabal ids.CabalID, from time.Time) (voted, eligible int, err error)
}

type ProposedMints interface {
	ProposedMints(ctx context.Context) ([]chain.SolanaAddress, error)
}

type Ballot struct {
	VoterID ids.UserID
	Choice  *string
	CastAt  *time.Time
}

type Ballots interface {
	Ballots(ctx context.Context, id ids.ProposalID) ([]Ballot, error)
}

type ProposalHead struct {
	Kind, Symbol, Thesis string
}

type ProposalHeads interface {
	ProposalHead(ctx context.Context, id ids.ProposalID) (ProposalHead, error)
}
