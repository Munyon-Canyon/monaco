package app

import (
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
	ID             ids.UserID
	Handle         string
	DisplayName    string
	PhotoURL       string
	AuthState      domain.AuthState
	AccountStatus  domain.AccountStatus
	PhoneVerified  bool
	XLinked        bool
	CreatedAt      time.Time
	FirstDepositAt *time.Time
	Deleted        bool
}

type MemberWallet struct {
	UserID        ids.UserID
	PrivyWalletID string
	Address       chain.SolanaAddress
}
