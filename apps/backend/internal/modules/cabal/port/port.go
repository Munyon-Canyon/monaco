package port

import (
	"context"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/modules/cabal/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

type Status string

const (
	StatusActive Status = "active"
	StatusBanned Status = "banned"
)

type CabalView struct {
	ID          ids.CabalID
	Name        string
	PictureURL  string
	CreatorID   ids.UserID
	Status      Status
	MemberCount int
	CreatedAt   time.Time
}

type MemberView struct {
	UserID   ids.UserID
	Role     domain.Role
	CanVote  bool
	JoinedAt time.Time
}

type Rules struct {
	JoinMode       domain.JoinMode
	VoterMode      domain.VoterMode
	Threshold      domain.Threshold
	ProposalExpiry time.Duration
	SlippageBps    int32
}

type TreasuryWallet struct {
	CabalID       ids.CabalID
	PrivyWalletID string
	Address       chain.SolanaAddress
}

type ViewReader interface {
	Cabal(ctx context.Context, id ids.CabalID) (CabalView, error)
	Cabals(ctx context.Context, cabalIDs []ids.CabalID) (map[ids.CabalID]CabalView, error)
	Rules(ctx context.Context, id ids.CabalID) (Rules, error)
	SlippageBps(ctx context.Context, id ids.CabalID) (int32, error)
	Status(ctx context.Context, id ids.CabalID) (Status, error)
}

type MemberReader interface {
	IsMember(ctx context.Context, id ids.CabalID, user ids.UserID) (bool, error)
	Member(ctx context.Context, id ids.CabalID, user ids.UserID) (MemberView, error)
	Members(ctx context.Context, id ids.CabalID) ([]MemberView, error)
	VoterSet(ctx context.Context, id ids.CabalID) ([]ids.UserID, error)
	CabalsOf(ctx context.Context, user ids.UserID) ([]ids.CabalID, error)
}

type TreasuryReader interface {
	TreasuryWallet(ctx context.Context, id ids.CabalID) (TreasuryWallet, error)
	TreasuryWallets(ctx context.Context) ([]TreasuryWallet, error)
}

type Queries interface {
	ViewReader
	MemberReader
	TreasuryReader
}
