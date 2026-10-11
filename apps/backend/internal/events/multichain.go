package events

import (
	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

const (
	TypeDepositEVMForwarded Type = "deposit.evm_forwarded"
	TypeTradeRouted         Type = "trade.routed"
	TypeTradeStepChanged    Type = "trade.step_changed"
)

type DepositEVMForwarded struct {
	V                int           `json:"v"`
	ForwardID        uuid.UUID     `json:"forward_id"`
	UserID           uuid.UUID     `json:"user_id"            pii:"true"`
	Network          chain.Network `json:"network"`
	SourceTx         string        `json:"source_tx"`
	PrivyExecutionID string        `json:"privy_execution_id"`
	AmountMicros     money.Micros  `json:"amount_micros"`
}

func (DepositEVMForwarded) Type() Type { return TypeDepositEVMForwarded }

func (DepositEVMForwarded) AggregateType() string { return "evm_deposit_forward" }

func (e DepositEVMForwarded) AggregateID() uuid.UUID { return e.ForwardID }

type TradeRouted struct {
	V               int           `json:"v"`
	SwapID          uuid.UUID     `json:"swap_id"`
	CabalID         uuid.UUID     `json:"cabal_id"`
	Action          string        `json:"action"`
	WinnerListingID uuid.UUID     `json:"winner_listing_id"`
	Network         chain.Network `json:"network"`
	Venue           string        `json:"venue"`
	QuotesCompared  int           `json:"quotes_compared"`
	AmountMicros    money.Micros  `json:"amount_micros"`
}

func (TradeRouted) Type() Type { return TypeTradeRouted }

func (TradeRouted) AggregateType() string { return swapAggregate }

func (e TradeRouted) AggregateID() uuid.UUID { return e.SwapID }

type TradeStepChanged struct {
	V             int           `json:"v"`
	SwapID        uuid.UUID     `json:"swap_id"`
	CabalID       uuid.UUID     `json:"cabal_id"`
	Step          string        `json:"step"`
	Network       chain.Network `json:"network"`
	Status        string        `json:"status"`
	PrivyActionID string        `json:"privy_action_id"`
	AmountMicros  money.Micros  `json:"amount_micros"`
}

func (TradeStepChanged) Type() Type { return TypeTradeStepChanged }

func (TradeStepChanged) AggregateType() string { return swapAggregate }

func (e TradeStepChanged) AggregateID() uuid.UUID { return e.SwapID }
