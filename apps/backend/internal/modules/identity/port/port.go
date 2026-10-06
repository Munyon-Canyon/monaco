package port

import (
	"context"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/modules/identity/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

const (
	MaxUsersByID   = 500
	MaxHandles     = 50
	MaxPhoneHashes = 2000
	MaxXUserIDs    = 2000
	MaxWalletPage  = 500
)

type UserCard struct {
	ID                 ids.UserID
	Handle             string
	DisplayName        string
	PhotoURL           string
	AuthState          domain.AuthState
	AccountStatus      domain.AccountStatus
	PhoneVerified      bool
	XLinked            bool
	AuthStateChangedAt time.Time
	CreatedAt          time.Time
	FirstDepositAt     *time.Time
	Deleted            bool
}

type MemberWallet struct {
	UserID        ids.UserID
	PrivyWalletID string
	Address       chain.SolanaAddress
}

type UserReader interface {
	UsersByID(ctx context.Context, userIDs []ids.UserID) (map[ids.UserID]UserCard, error)
	UserByHandle(ctx context.Context, handle string) (UserCard, error)
	UserIDsByHandles(ctx context.Context, handles []string) (map[string]ids.UserID, error)
}

type ContactMatcher interface {
	UsersByPhoneHashes(ctx context.Context, hashes [][]byte) (map[string]ids.UserID, error)
	UsersByXUserIDs(ctx context.Context, xUserIDs []string) (map[string]ids.UserID, error)
}

type WalletReader interface {
	MemberWallet(ctx context.Context, id ids.UserID) (MemberWallet, error)
	MemberWallets(ctx context.Context, after ids.UserID, limit int) ([]MemberWallet, error)
}

type Queries interface {
	UserReader
	ContactMatcher
	WalletReader
}
