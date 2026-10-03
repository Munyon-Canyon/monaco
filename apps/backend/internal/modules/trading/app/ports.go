package app

import (
	"context"

	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

type QuoteSpec struct {
	InMint, OutMint chain.Mint
	InAmount        uint64
}

type Quote struct {
	InAmount       uint64
	OutAmount      uint64
	PriceImpactBps int64
	Routable       bool
}

type OrderSpec struct {
	Taker           chain.SolanaAddress
	InMint, OutMint chain.Mint
	InAmount        uint64
	SlippageBps     int64
}

type Order struct {
	RequestID   string
	Transaction []byte
}

type ExecuteStatus uint8

const (
	ExecuteSuccess ExecuteStatus = iota + 1
	ExecuteFailed
	ExecutePending
)

type ExecuteResult struct {
	Status    ExecuteStatus
	OutAmount uint64
	ErrorCode int
}

type Venue interface {
	Quote(ctx context.Context, spec QuoteSpec) (Quote, error)
	Order(ctx context.Context, spec OrderSpec) (Order, error)
	ExecuteUntilTerminal(ctx context.Context, requestID string, signed []byte) (ExecuteResult, error)
}

type Signer interface {
	Sign(
		ctx context.Context, privyWalletID string, unsigned []byte,
	) (signed []byte, signature chain.Signature, err error)
}

type Holding struct {
	Mint   chain.Mint
	Symbol string
	Units  uint64
}

type Holdings interface {
	Positions(ctx context.Context, cabal ids.CabalID) ([]Holding, error)
}

type Wallets interface {
	TreasuryWallet(ctx context.Context, cabal ids.CabalID) (TreasuryWallet, error)
}
