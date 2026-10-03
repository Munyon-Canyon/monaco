package events

import "github.com/google/uuid"

const (
	TypeCabalPaused  Type = "cabal.paused"
	TypeCabalResumed Type = "cabal.resumed"
)

type CabalPaused struct {
	V       int        `json:"v"`
	PauseID uuid.UUID  `json:"pause_id"`
	CabalID *uuid.UUID `json:"cabal_id"`
	Reason  string     `json:"reason"`
	Scope   string     `json:"scope"`
}

func (CabalPaused) Type() Type { return TypeCabalPaused }

func (CabalPaused) AggregateType() string { return cabalAggregate }

func (e CabalPaused) AggregateID() uuid.UUID { return pauseAggregateID(e.CabalID) }

type CabalResumed struct {
	V       int        `json:"v"`
	CabalID *uuid.UUID `json:"cabal_id"`
	Scope   string     `json:"scope"`
}

func (CabalResumed) Type() Type { return TypeCabalResumed }

func (CabalResumed) AggregateType() string { return cabalAggregate }

func (e CabalResumed) AggregateID() uuid.UUID { return pauseAggregateID(e.CabalID) }

func pauseAggregateID(cabalID *uuid.UUID) uuid.UUID {
	if cabalID == nil {
		return uuid.Nil
	}
	return *cabalID
}
