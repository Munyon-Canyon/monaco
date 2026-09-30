package events

import (
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
)

const (
	TypeCabalCreated         Type = "cabal.created"
	TypeCabalMemberJoined    Type = "cabal.member_joined"
	TypeCabalAccessRequested Type = "cabal.access_requested"
	TypeCabalAccessDecided   Type = "cabal.access_decided"
	TypeCabalMemberLeft      Type = "cabal.member_left"
	TypeCabalUpdated         Type = "cabal.updated"
)

const cabalAggregate = "cabal"

type CabalCreated struct {
	V                     int                 `json:"v"`
	CabalID               uuid.UUID           `json:"cabal_id"`
	CreatorID             uuid.UUID           `json:"creator_id"              pii:"true"`
	Name                  string              `json:"name"`
	JoinMode              string              `json:"join_mode"`
	VoterMode             string              `json:"voter_mode"`
	Threshold             string              `json:"threshold"`
	ProposalExpirySeconds int32               `json:"proposal_expiry_seconds"`
	SlippageBps           int32               `json:"slippage_bps"`
	TreasuryAddress       chain.SolanaAddress `json:"treasury_address"`
}

func (CabalCreated) Type() Type { return TypeCabalCreated }

func (CabalCreated) AggregateType() string { return cabalAggregate }

func (e CabalCreated) AggregateID() uuid.UUID { return e.CabalID }

type CabalMemberJoined struct {
	V         int       `json:"v"`
	CabalID   uuid.UUID `json:"cabal_id"`
	UserID    uuid.UUID `json:"user_id"             pii:"true"`
	Role      string    `json:"role"`
	Via       string    `json:"via"`
	RequestID uuid.UUID `json:"request_id,omitzero"`
}

func (CabalMemberJoined) Type() Type { return TypeCabalMemberJoined }

func (CabalMemberJoined) AggregateType() string { return cabalAggregate }

func (e CabalMemberJoined) AggregateID() uuid.UUID { return e.CabalID }

type CabalAccessRequested struct {
	V         int       `json:"v"`
	RequestID uuid.UUID `json:"request_id"`
	CabalID   uuid.UUID `json:"cabal_id"`
	UserID    uuid.UUID `json:"user_id"             pii:"true"`
	Direction string    `json:"direction"`
	ActorID   uuid.UUID `json:"actor_id"            pii:"true"`
	ExpiresAt time.Time `json:"expires_at,omitzero"`
}

func (CabalAccessRequested) Type() Type { return TypeCabalAccessRequested }

func (CabalAccessRequested) AggregateType() string { return cabalAggregate }

func (e CabalAccessRequested) AggregateID() uuid.UUID { return e.CabalID }

type CabalAccessDecided struct {
	V         int       `json:"v"`
	RequestID uuid.UUID `json:"request_id"`
	CabalID   uuid.UUID `json:"cabal_id"`
	UserID    uuid.UUID `json:"user_id"           pii:"true"`
	Direction string    `json:"direction"`
	Decision  string    `json:"decision"`
	ActorID   uuid.UUID `json:"actor_id,omitzero" pii:"true"`
}

func (CabalAccessDecided) Type() Type { return TypeCabalAccessDecided }

func (CabalAccessDecided) AggregateType() string { return cabalAggregate }

func (e CabalAccessDecided) AggregateID() uuid.UUID { return e.CabalID }

type CabalMemberLeft struct {
	V        int       `json:"v"`
	CabalID  uuid.UUID `json:"cabal_id"`
	UserID   uuid.UUID `json:"user_id"   pii:"true"`
	WasVoter bool      `json:"was_voter"`
}

func (CabalMemberLeft) Type() Type { return TypeCabalMemberLeft }

func (CabalMemberLeft) AggregateType() string { return cabalAggregate }

func (e CabalMemberLeft) AggregateID() uuid.UUID { return e.CabalID }

type CabalChanges struct {
	Name                  *string     `json:"name,omitempty"`
	PictureURL            *string     `json:"picture_url,omitempty"`
	JoinMode              *string     `json:"join_mode,omitempty"`
	VoterMode             *string     `json:"voter_mode,omitempty"`
	VoterIDs              []uuid.UUID `json:"voter_ids,omitempty"               pii:"true"`
	Threshold             *string     `json:"threshold,omitempty"`
	ProposalExpirySeconds *int32      `json:"proposal_expiry_seconds,omitempty"`
	SlippageBps           *int32      `json:"slippage_bps,omitempty"`
}

type CabalUpdated struct {
	V       int          `json:"v"`
	CabalID uuid.UUID    `json:"cabal_id"`
	ActorID uuid.UUID    `json:"actor_id" pii:"true"`
	Changes CabalChanges `json:"changes"`
}

func (CabalUpdated) Type() Type { return TypeCabalUpdated }

func (CabalUpdated) AggregateType() string { return cabalAggregate }

func (e CabalUpdated) AggregateID() uuid.UUID { return e.CabalID }
