package port

import (
	"context"
	"math/big"
	"time"

	"github.com/google/uuid"

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
	Unpriced    bool
	EVMCash     money.Micros
	InTransit   money.Micros
	Listings    []ListingPosition
}

type ListingPosition struct {
	Key   string
	Units *big.Int
}

type MemberStake struct {
	UserID               ids.UserID
	CabalID              ids.CabalID
	ShareUnits           money.SharesUnits
	NetContributedMicros money.SignedMicros
}

type MemberFlow struct {
	UserID  ids.UserID
	CabalID ids.CabalID
	Amount  money.SignedMicros
	At      time.Time
}

type ContributionPoint struct {
	At             time.Time
	NetContributed money.SignedMicros
}

type StakePoint struct {
	CabalID        ids.CabalID
	At             time.Time
	ShareUnits     money.SharesUnits
	NetContributed money.SignedMicros
}

type PositionsReader interface {
	SolanaPositions(ctx context.Context, cabalID ids.CabalID) ([]Position, error)
	PotValue(ctx context.Context, cabalID ids.CabalID) (money.Micros, error)
	TotalShares(ctx context.Context, cabalID ids.CabalID) (money.SharesUnits, error)
}

type StakesReader interface {
	ShareUnits(ctx context.Context, cabalID ids.CabalID, user ids.UserID) (money.SharesUnits, error)
	Stake(ctx context.Context, cabalID ids.CabalID, user ids.UserID) (Stake, error)
	StakesOf(ctx context.Context, user ids.UserID) ([]Stake, error)
	OpenFund(ctx context.Context, cabalID ids.CabalID, user ids.UserID) (bool, error)
	ShareUnitsAt(ctx context.Context, cabalID ids.CabalID, user ids.UserID, t time.Time) (money.SharesUnits, error)
}

type HistoricalReader interface {
	CabalPositionsAt(ctx context.Context, t time.Time) ([]CabalPositions, error)
	MemberStakesAt(ctx context.Context, t time.Time) ([]MemberStake, error)
	MemberFlowsBetween(ctx context.Context, from, to time.Time) ([]MemberFlow, error)
}

type HistoryReader interface {
	CabalContributionHistory(ctx context.Context, cabalID ids.CabalID) ([]ContributionPoint, error)
	UserStakeHistory(ctx context.Context, userID ids.UserID) ([]StakePoint, error)
	OwnershipBps(ctx context.Context, user ids.UserID) (map[ids.CabalID]int32, error)
}

type ReservationReader interface {
	CashOutReservations(ctx context.Context) (map[ids.CabalID]money.Micros, error)
}

type Queries interface {
	PositionsReader
	StakesReader
	HistoricalReader
	HistoryReader
	ReservationReader
}

type SignatureOwner interface {
	OwnsSignature(ctx context.Context, sig chain.Signature) (bool, error)
}

type WalletLedger interface {
	WalletLedgerMicros(ctx context.Context, user ids.UserID, mint chain.SolanaAddress) (money.SignedMicros, int, error)
}

type Holding struct {
	Symbol          string
	DisplayName     string
	Kind            string
	Units           string
	TokenAmount     money.BaseUnits
	PriceMicros     money.Micros
	ValueMicros     money.Micros
	WeightBps       int32
	CostBasisMicros money.Micros
	PnLMicros       money.SignedMicros
	AvgEntryMicros  *money.Micros
	FirstBoughtAt   *time.Time
	Listings        []HoldingListing
}

type HoldingListing struct {
	ListingID   uuid.UUID
	Issuer      string
	Network     chain.Network
	TokenAmount *big.Int
	ValueMicros money.Micros
}

type Slice struct {
	ShareUnits           money.SharesUnits
	ValueMicros          money.Micros
	SliceBps             int32
	NetContributedMicros money.SignedMicros
	PnLMicros            money.SignedMicros
	FirstFundedAt        *time.Time
}

type CabalPot struct {
	CabalID        ids.CabalID
	PotValueMicros money.Micros
	CashMicros     money.Micros
	CashWeightBps  int32
	PnLMicros      money.SignedMicros
	ReturnBps      *int32
	PricesAsOf     time.Time
	Holdings       []Holding
	Me             *Slice
}

type HeldMints interface {
	HeldMints(ctx context.Context) ([]chain.SolanaAddress, error)
}
