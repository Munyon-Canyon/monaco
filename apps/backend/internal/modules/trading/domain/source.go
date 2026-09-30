package domain

import (
	"slices"

	"github.com/google/uuid"
)

type SourceKind string

const (
	SourceProposal SourceKind = "proposal"
	SourceCashout  SourceKind = "cashout"
)

func SourceKinds() []SourceKind { return []SourceKind{SourceProposal, SourceCashout} }

func ParseSourceKind(raw string) (SourceKind, error) {
	if !slices.Contains(SourceKinds(), SourceKind(raw)) {
		return "", unknown("trading.ParseSourceKind", raw)
	}
	return SourceKind(raw), nil
}

type Action string

const (
	ActionBuy  Action = "buy"
	ActionSell Action = "sell"
)

func Actions() []Action { return []Action{ActionBuy, ActionSell} }

func ParseAction(raw string) (Action, error) {
	if !slices.Contains(Actions(), Action(raw)) {
		return "", unknown("trading.ParseAction", raw)
	}
	return Action(raw), nil
}

type Source struct {
	Kind SourceKind
	ID   uuid.UUID
}
