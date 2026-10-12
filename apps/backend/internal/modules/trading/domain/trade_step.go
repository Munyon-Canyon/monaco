package domain

import (
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

type StepKind string

const (
	StepBridgeOut  StepKind = "bridge_out"
	StepSwap       StepKind = "swap"
	StepBridgeBack StepKind = "bridge_back"
)

type StepStatus string

const (
	StepPending   StepStatus = "pending"
	StepSubmitted StepStatus = "submitted"
	StepSucceeded StepStatus = "succeeded"
	StepFailed    StepStatus = "failed"
)

type StepRef struct {
	SwapID  ids.SwapID
	CabalID ids.CabalID
	Step    StepKind
	Network chain.Network
}

type Arrival struct {
	Amount money.Micros
	TxHash string
}

func (a Arrival) Observed() bool { return a.TxHash != "" }

type TradeStep struct {
	ID                  uuid.UUID
	SwapID              ids.SwapID
	CabalID             ids.CabalID
	Source              Source
	SourceBatchSize     int
	Step                StepKind
	Network             chain.Network
	PrivyActionID       string
	Amount              money.Micros
	Status              StepStatus
	Error               string
	Attempt             int
	PriorPrivyActionIDs []string
	Arrival             Arrival
	SentAt              time.Time
	CreatedAt           time.Time
	UpdatedAt           time.Time
}
