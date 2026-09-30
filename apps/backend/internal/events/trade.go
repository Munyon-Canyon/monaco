package events

import (
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

const (
	TypeTradeBlocked   Type = "trade.blocked"
	TypeTradeSubmitted Type = "trade.submitted"
	TypeTradeConfirmed Type = "trade.confirmed"
	TypeTradeFailed    Type = "trade.failed"
)

const swapAggregate = "swap"

type TradeSource struct {
	Kind string    `json:"kind"`
	ID   uuid.UUID `json:"id"`
}

type TradeBlocked struct {
	V               int         `json:"v"`
	CabalID         uuid.UUID   `json:"cabal_id"`
	Source          TradeSource `json:"source"`
	SourceBatchSize int         `json:"source_batch_size"`
	Action          string      `json:"action"`
	Symbol          string      `json:"symbol"`
	Code            errs.Code   `json:"code"`
	Have            uint64      `json:"have,string"`
	Need            uint64      `json:"need,string"`
}

func (TradeBlocked) Type() Type { return TypeTradeBlocked }

func (e TradeBlocked) AggregateType() string { return e.Source.Kind }

func (e TradeBlocked) AggregateID() uuid.UUID { return e.Source.ID }

type TradeSubmitted struct {
	V               int                 `json:"v"`
	SwapID          uuid.UUID           `json:"swap_id"`
	CabalID         uuid.UUID           `json:"cabal_id"`
	Source          TradeSource         `json:"source"`
	SourceBatchSize int                 `json:"source_batch_size"`
	Action          string              `json:"action"`
	Symbol          string              `json:"symbol"`
	InMint          chain.SolanaAddress `json:"in_mint"`
	OutMint         chain.SolanaAddress `json:"out_mint"`
	InAmount        uint64              `json:"in_amount,string"`
	TxSignature     chain.Signature     `json:"tx_signature"`
}

func (TradeSubmitted) Type() Type { return TypeTradeSubmitted }

func (TradeSubmitted) AggregateType() string { return swapAggregate }

func (e TradeSubmitted) AggregateID() uuid.UUID { return e.SwapID }

type TradeConfirmed struct {
	V               int                 `json:"v"`
	SwapID          uuid.UUID           `json:"swap_id"`
	CabalID         uuid.UUID           `json:"cabal_id"`
	Source          TradeSource         `json:"source"`
	SourceBatchSize int                 `json:"source_batch_size"`
	Action          string              `json:"action"`
	Symbol          string              `json:"symbol"`
	InMint          chain.SolanaAddress `json:"in_mint"`
	InAmount        uint64              `json:"in_amount,string"`
	OutMint         chain.SolanaAddress `json:"out_mint"`
	OutAmount       uint64              `json:"out_amount,string"`
	USDCMicros      money.Micros        `json:"usdc_micros"`
	FeeMicros       money.Micros        `json:"fee_micros"`
	TxSignature     chain.Signature     `json:"tx_signature"`
	ConfirmedAt     time.Time           `json:"confirmed_at"`
}

func (TradeConfirmed) Type() Type { return TypeTradeConfirmed }

func (TradeConfirmed) AggregateType() string { return swapAggregate }

func (e TradeConfirmed) AggregateID() uuid.UUID { return e.SwapID }

type TradeFailed struct {
	V               int                 `json:"v"`
	SwapID          uuid.UUID           `json:"swap_id"`
	CabalID         uuid.UUID           `json:"cabal_id"`
	Source          TradeSource         `json:"source"`
	SourceBatchSize int                 `json:"source_batch_size"`
	Action          string              `json:"action"`
	Symbol          string              `json:"symbol"`
	InMint          chain.SolanaAddress `json:"in_mint"`
	InAmount        uint64              `json:"in_amount,string"`
	FailureCode     string              `json:"failure_code"`
	JupiterCode     string              `json:"jupiter_code,omitempty"`
}

func (TradeFailed) Type() Type { return TypeTradeFailed }

func (TradeFailed) AggregateType() string { return swapAggregate }

func (e TradeFailed) AggregateID() uuid.UUID { return e.SwapID }
