package domain

import (
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

type LoginProvider string

const (
	LoginSMS    LoginProvider = "sms"
	LoginEmail  LoginProvider = "email"
	LoginApple  LoginProvider = "apple"
	LoginGoogle LoginProvider = "google"
)

type Wallet struct {
	PrivyWalletID string
	Address       chain.SolanaAddress
}

type NewUser struct {
	ID            ids.UserID
	PrivyUserID   string
	LoginProvider LoginProvider
	Email         string
	Wallet        *Wallet
}

type User struct {
	ID            ids.UserID
	PrivyUserID   string
	Handle        string
	AuthState     AuthState
	AccountStatus AccountStatus
	Wallet        *Wallet
}
