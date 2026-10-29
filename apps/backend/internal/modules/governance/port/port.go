package port

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/modules/governance/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

type Status = domain.Status

const (
	StatusOpen   = domain.StatusOpen
	StatusPassed = domain.StatusPassed
)

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

type VoteState struct {
	CabalID                 ids.CabalID
	Kind, Symbol, Mint      string
	USDCMicros              money.Micros
	Status                  Status
	Yes, No, Eligible, Need int
	CreatedAt, ExpiresAt    time.Time
}

type ReminderCursor struct {
	DueAt      time.Time
	ProposalID ids.ProposalID
}

type DueReminder struct {
	ProposalID ids.ProposalID
	DueAt      time.Time
}

type VoteStates interface {
	VoteState(ctx context.Context, id ids.ProposalID) (VoteState, error)
	DueForReminder(ctx context.Context, now time.Time, after ReminderCursor, limit int) ([]DueReminder, error)
	NonVoters(ctx context.Context, id ids.ProposalID) ([]ids.UserID, error)
}

const MemberActivityMaxLimit = 100

type Cursor struct {
	At time.Time
	ID uuid.UUID
}

type CastVote struct {
	ProposalID ids.ProposalID
	CabalID    ids.CabalID
	Kind       string
	Symbol     string
	Mint       string
	USDCMicros *uint64
	Choice     string
	CastAt     time.Time
}

type ProposedRow struct {
	ProposalID ids.ProposalID
	CabalID    ids.CabalID
	Kind       string
	Symbol     string
	Mint       string
	USDCMicros *uint64
	Status     Status
	Yes, No    int
	CreatedAt  time.Time
}

type Subject struct {
	CabalID    ids.CabalID
	Kind       string
	Symbol     string
	USDCMicros *uint64
}

type MemberActivity interface {
	VotesBy(ctx context.Context, user ids.UserID, after *Cursor, limit int) ([]CastVote, error)
	ProposalsBy(ctx context.Context, user ids.UserID, after *Cursor, limit int) ([]ProposedRow, error)
	ProposalSubjects(ctx context.Context, proposals []ids.ProposalID) (map[ids.ProposalID]Subject, error)
}
