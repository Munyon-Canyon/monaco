package app

import (
	"context"

	"github.com/monaco/monaco/apps/backend/internal/modules/identity/domain"
)

type PrivyUserID string

type XAccount struct {
	UserID   string
	Username string
}

type PrivyUser struct {
	ID          PrivyUserID
	AppleEmail  string
	GoogleEmail string
	PhoneE164   string
	X           *XAccount
}

type PrivyWallet struct {
	domain.Wallet
	HasAppSigner bool
}

type PrivyUsers interface {
	Verify(ctx context.Context, raw string) (PrivyUserID, error)
	User(ctx context.Context, id PrivyUserID) (PrivyUser, error)
}

type MemberWallets interface {
	FindOrCreate(ctx context.Context, id PrivyUserID) (PrivyWallet, error)
}
