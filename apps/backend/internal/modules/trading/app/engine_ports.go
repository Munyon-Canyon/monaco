package app

import (
	"context"

	cabalport "github.com/monaco/monaco/apps/backend/internal/modules/cabal/port"
	fundingport "github.com/monaco/monaco/apps/backend/internal/modules/funding/port"
	governanceport "github.com/monaco/monaco/apps/backend/internal/modules/governance/port"
	"github.com/monaco/monaco/apps/backend/internal/modules/market"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/solana"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

type Catalog interface {
	AssetBySymbol(ctx context.Context, symbol string) (market.Asset, error)
}

type Cabals interface {
	Status(ctx context.Context, id ids.CabalID) (cabalport.Status, error)
	SlippageBps(ctx context.Context, id ids.CabalID) (int32, error)
	TreasuryWallet(ctx context.Context, id ids.CabalID) (cabalport.TreasuryWallet, error)
}

type Pauses interface {
	IsPaused(ctx context.Context, cabalID ids.CabalID) (fundingport.Pause, error)
}

type Proposals interface {
	Status(ctx context.Context, id ids.ProposalID) (governanceport.Status, error)
}

type Balances interface {
	TokenBalance(ctx context.Context, owner chain.SolanaAddress, mint chain.Mint) (money.BaseUnits, error)
	MintConfig(ctx context.Context, mint chain.SolanaAddress) (solana.MintConfig, error)
}

type EnginePorts struct {
	Catalog   Catalog
	Cabals    Cabals
	Pauses    Pauses
	Proposals Proposals
	Balances  Balances
}
