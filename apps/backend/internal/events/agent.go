package events

import (
	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

const (
	TypeAgentEnabled       Type = "agent.enabled"
	TypeAgentPaused        Type = "agent.paused"
	TypeAgentRemoved       Type = "agent.removed"
	TypeAgentChangeBlocked Type = "agent.change_blocked"
	TypeAgentKeyRevealed   Type = "agent.key_revealed"
	TypeAgentIntentCreated Type = "agent.intent_created"
)

const (
	agentAggregate       = "agent"
	agentIntentAggregate = "agent_intent"
)

type AgentEnabled struct {
	V                int          `json:"v"`
	AgentID          uuid.UUID    `json:"agent_id"`
	CabalID          uuid.UUID    `json:"cabal_id"`
	ProposalID       uuid.UUID    `json:"proposal_id"`
	Name             string       `json:"name"`
	BudgetUSDCMicros money.Micros `json:"budget_usdc_micros"`
	Reason           string       `json:"reason"`
}

func (AgentEnabled) Type() Type { return TypeAgentEnabled }

func (AgentEnabled) AggregateType() string { return agentAggregate }

func (e AgentEnabled) AggregateID() uuid.UUID { return e.AgentID }

type AgentPaused struct {
	V          int       `json:"v"`
	AgentID    uuid.UUID `json:"agent_id"`
	CabalID    uuid.UUID `json:"cabal_id"`
	ProposalID uuid.UUID `json:"proposal_id"`
}

func (AgentPaused) Type() Type { return TypeAgentPaused }

func (AgentPaused) AggregateType() string { return agentAggregate }

func (e AgentPaused) AggregateID() uuid.UUID { return e.AgentID }

type AgentRemoved struct {
	V          int       `json:"v"`
	AgentID    uuid.UUID `json:"agent_id"`
	CabalID    uuid.UUID `json:"cabal_id"`
	ProposalID uuid.UUID `json:"proposal_id"`
}

func (AgentRemoved) Type() Type { return TypeAgentRemoved }

func (AgentRemoved) AggregateType() string { return agentAggregate }

func (e AgentRemoved) AggregateID() uuid.UUID { return e.AgentID }

type AgentChangeBlocked struct {
	V          int       `json:"v"`
	CabalID    uuid.UUID `json:"cabal_id"`
	ProposalID uuid.UUID `json:"proposal_id"`
	Kind       string    `json:"kind"`
	Code       errs.Code `json:"code"`
}

func (AgentChangeBlocked) Type() Type { return TypeAgentChangeBlocked }

func (AgentChangeBlocked) AggregateType() string { return proposalAggregate }

func (e AgentChangeBlocked) AggregateID() uuid.UUID { return e.ProposalID }

type AgentKeyRevealed struct {
	V       int       `json:"v"`
	AgentID uuid.UUID `json:"agent_id"`
	CabalID uuid.UUID `json:"cabal_id"`
	UserID  uuid.UUID `json:"user_id"  pii:"true"`
}

func (AgentKeyRevealed) Type() Type { return TypeAgentKeyRevealed }

func (AgentKeyRevealed) AggregateType() string { return agentAggregate }

func (e AgentKeyRevealed) AggregateID() uuid.UUID { return e.AgentID }

type AgentIntentCreated struct {
	V              int                 `json:"v"`
	IntentID       uuid.UUID           `json:"intent_id"`
	AgentID        uuid.UUID           `json:"agent_id"`
	CabalID        uuid.UUID           `json:"cabal_id"`
	Side           string              `json:"side"`
	Mint           chain.SolanaAddress `json:"mint"`
	Symbol         string              `json:"symbol"`
	USDCMicros     money.Micros        `json:"usdc_micros"`
	TokenAmount    uint64              `json:"token_amount,string"`
	QuoteOutAmount uint64              `json:"quote_out_amount,string"`
}

func (AgentIntentCreated) Type() Type { return TypeAgentIntentCreated }

func (AgentIntentCreated) AggregateType() string { return agentIntentAggregate }

func (e AgentIntentCreated) AggregateID() uuid.UUID { return e.IntentID }
