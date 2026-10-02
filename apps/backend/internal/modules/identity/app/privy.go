package app

import (
	"cmp"
	"context"

	"github.com/monaco/monaco/apps/backend/internal/modules/identity/domain"
)

type PrivyUserID string

type PrivyUser struct {
	ID          PrivyUserID
	Email       string
	AppleEmail  string
	GoogleEmail string
	PhoneE164   string
	X           *domain.XAccount
}

func (u PrivyUser) LoginMethods() domain.LoginMethods {
	return domain.LoginMethods{
		Phone: u.PhoneE164 != "", Email: u.Email != "", Apple: u.AppleEmail != "", Google: u.GoogleEmail != "",
	}
}

func (u PrivyUser) Links() domain.Links { return domain.Links{Phone: u.PhoneE164, X: u.X} }

func (u PrivyUser) ContactEmail() string { return cmp.Or(u.Email, u.AppleEmail, u.GoogleEmail) }

type PrivyWallet struct {
	domain.Wallet
	HasAppSigner bool
}

type PrivyUsers interface {
	Verify(ctx context.Context, raw string) (PrivyUserID, error)
	User(ctx context.Context, id PrivyUserID) (PrivyUser, error)
	Create(ctx context.Context, email string) (PrivyUserID, error)
}

type MemberWallets interface {
	FindOrCreate(ctx context.Context, id PrivyUserID) (PrivyWallet, error)
}
