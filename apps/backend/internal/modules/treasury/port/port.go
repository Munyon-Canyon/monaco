package port

import (
	"context"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

type Position struct {
	Mint      chain.SolanaAddress
	Units     money.BaseUnits
	CostBasis money.Micros
}

type Stake struct {
	CabalID           ids.CabalID
	UserID            ids.UserID
	ShareUnits        money.SharesUnits
	TotalShares       money.SharesUnits
	ValueMicros       money.Micros
	ContributedMicros money.Micros
	WithdrawnMicros   money.Micros
}

type CabalPositions struct {
	CabalID     ids.CabalID
	Holdings    []Position
	TotalShares money.SharesUnits
}

type MemberStake struct {
	UserID               ids.UserID
	CabalID              ids.CabalID
	ShareUnits           money.SharesUnits
	NetContributedMicros money.SignedMicros
}

type PositionsReader interface {
	Positions(ctx context.Context, cabalID ids.CabalID) ([]Position, error)
	PotValue(ctx context.Context, cabalID ids.CabalID) (money.Micros, error)
	TotalShares(ctx context.Context, cabalID ids.CabalID) (money.SharesUnits, error)
}

type StakesReader interface {
	ShareUnits(ctx context.Context, cabalID ids.CabalID, user ids.UserID) (money.SharesUnits, error)
	Stake(ctx context.Context, cabalID ids.CabalID, user ids.UserID) (Stake, error)
	StakesOf(ctx context.Context, user ids.UserID) ([]Stake, error)
	ShareUnitsAt(ctx context.Context, cabalID ids.CabalID, user ids.UserID, t time.Time) (money.SharesUnits, error)
}

type HistoricalReader interface {
	CabalPositionsAt(ctx context.Context, t time.Time) ([]CabalPositions, error)
	MemberStakesAt(ctx context.Context, t time.Time) ([]MemberStake, error)
}

type Queries interface {
	PositionsReader
	StakesReader
	HistoricalReader
}

type SignatureOwner interface {
	OwnsSignature(ctx context.Context, sig chain.Signature) (bool, error)
}

type WalletLedger interface {
	WalletLedgerMicros(ctx context.Context, user ids.UserID, mint chain.SolanaAddress) (money.SignedMicros, int, error)
}
