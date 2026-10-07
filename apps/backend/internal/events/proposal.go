package events

import (
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

const (
	TypeProposalCreated          Type = "proposal.created"
	TypeProposalPassed           Type = "proposal.passed"
	TypeProposalFailed           Type = "proposal.failed"
	TypeProposalExpired          Type = "proposal.expired"
	TypeProposalWithdrawn        Type = "proposal.withdrawn"
	TypeProposalVoided           Type = "proposal.voided"
	TypeProposalExecuted         Type = "proposal.executed"
	TypeProposalExecutionBlocked Type = "proposal.execution_blocked"
	TypeProposalReopened         Type = "proposal.reopened"
)

const proposalAggregate = "proposal"

type ProposalCreated struct {
	V              int                 `json:"v"`
	ProposalID     uuid.UUID           `json:"proposal_id"`
	CabalID        uuid.UUID           `json:"cabal_id"`
	ProposerID     uuid.UUID           `json:"proposer_id"                  pii:"true"`
	Kind           string              `json:"kind"`
	Symbol         string              `json:"symbol"`
	Mint           chain.SolanaAddress `json:"mint"`
	USDCMicros     money.Micros        `json:"usdc_micros,omitzero"`
	TokenAmount    uint64              `json:"token_amount,string,omitzero"`
	QuoteOutAmount uint64              `json:"quote_out_amount,string"`
	ExpiresAt      time.Time           `json:"expires_at"`
	VoterCount     int                 `json:"voter_count"`
}

func (ProposalCreated) Type() Type { return TypeProposalCreated }

func (ProposalCreated) AggregateType() string { return proposalAggregate }

func (e ProposalCreated) AggregateID() uuid.UUID { return e.ProposalID }

type ProposalPassed struct {
	V              int                 `json:"v"`
	ProposalID     uuid.UUID           `json:"proposal_id"`
	CabalID        uuid.UUID           `json:"cabal_id"`
	Kind           string              `json:"kind"`
	Symbol         string              `json:"symbol"`
	Mint           chain.SolanaAddress `json:"mint"`
	USDCMicros     money.Micros        `json:"usdc_micros,omitzero"`
	TokenAmount    uint64              `json:"token_amount,string,omitzero"`
	QuoteOutAmount uint64              `json:"quote_out_amount,string"`
	ProposerID     uuid.UUID           `json:"proposer_id"                  pii:"true"`
}

func (ProposalPassed) Type() Type { return TypeProposalPassed }

func (ProposalPassed) AggregateType() string { return proposalAggregate }

func (e ProposalPassed) AggregateID() uuid.UUID { return e.ProposalID }

type ProposalFailed struct {
	V          int       `json:"v"`
	ProposalID uuid.UUID `json:"proposal_id"`
	CabalID    uuid.UUID `json:"cabal_id"`
}

func (ProposalFailed) Type() Type { return TypeProposalFailed }

func (ProposalFailed) AggregateType() string { return proposalAggregate }

func (e ProposalFailed) AggregateID() uuid.UUID { return e.ProposalID }

type ProposalExpired struct {
	V          int       `json:"v"`
	ProposalID uuid.UUID `json:"proposal_id"`
	CabalID    uuid.UUID `json:"cabal_id"`
}

func (ProposalExpired) Type() Type { return TypeProposalExpired }

func (ProposalExpired) AggregateType() string { return proposalAggregate }

func (e ProposalExpired) AggregateID() uuid.UUID { return e.ProposalID }

type ProposalWithdrawn struct {
	V          int       `json:"v"`
	ProposalID uuid.UUID `json:"proposal_id"`
	CabalID    uuid.UUID `json:"cabal_id"`
	ProposerID uuid.UUID `json:"proposer_id" pii:"true"`
}

func (ProposalWithdrawn) Type() Type { return TypeProposalWithdrawn }

func (ProposalWithdrawn) AggregateType() string { return proposalAggregate }

func (e ProposalWithdrawn) AggregateID() uuid.UUID { return e.ProposalID }

type ProposalVoided struct {
	V          int       `json:"v"`
	ProposalID uuid.UUID `json:"proposal_id"`
	CabalID    uuid.UUID `json:"cabal_id"`
	ActorType  string    `json:"actor_type"`
	Reason     string    `json:"reason"`
}

func (ProposalVoided) Type() Type { return TypeProposalVoided }

func (ProposalVoided) AggregateType() string { return proposalAggregate }

func (e ProposalVoided) AggregateID() uuid.UUID { return e.ProposalID }

type ProposalExecuted struct {
	V          int       `json:"v"`
	ProposalID uuid.UUID `json:"proposal_id"`
	CabalID    uuid.UUID `json:"cabal_id"`
	SwapID     uuid.UUID `json:"swap_id"`
}

func (ProposalExecuted) Type() Type { return TypeProposalExecuted }

func (ProposalExecuted) AggregateType() string { return proposalAggregate }

func (e ProposalExecuted) AggregateID() uuid.UUID { return e.ProposalID }

type ProposalExecutionBlocked struct {
	V          int       `json:"v"`
	ProposalID uuid.UUID `json:"proposal_id"`
	CabalID    uuid.UUID `json:"cabal_id"`
	Code       errs.Code `json:"code"`
}

func (ProposalExecutionBlocked) Type() Type { return TypeProposalExecutionBlocked }

func (ProposalExecutionBlocked) AggregateType() string { return proposalAggregate }

func (e ProposalExecutionBlocked) AggregateID() uuid.UUID { return e.ProposalID }

type ProposalReopened struct {
	V          int       `json:"v"`
	ProposalID uuid.UUID `json:"proposal_id"`
	CabalID    uuid.UUID `json:"cabal_id"`
	SwapID     uuid.UUID `json:"swap_id"`
}

func (ProposalReopened) Type() Type { return TypeProposalReopened }

func (ProposalReopened) AggregateType() string { return proposalAggregate }

func (e ProposalReopened) AggregateID() uuid.UUID { return e.ProposalID }
