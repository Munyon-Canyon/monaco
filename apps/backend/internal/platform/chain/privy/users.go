package privy

import (
	"context"
	"net/http"
	"net/url"

	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
)

type XAccount struct {
	UserID   string
	Username string
}

type User struct {
	ID             UserID
	Email          string
	AppleEmail     string
	GoogleEmail    string
	Phone          string
	X              *XAccount
	EmbeddedWallet *chain.Wallet
}

type linkedAccount struct {
	Type             string `json:"type"`
	Email            string `json:"email"`
	Number           string `json:"number"`
	Subject          string `json:"subject"`
	Username         string `json:"username"`
	ID               string `json:"id"`
	Address          string `json:"address"`
	ChainType        string `json:"chain_type"`
	WalletClientType string `json:"wallet_client_type"`
}

func (c *Client) GetUser(ctx context.Context, id UserID) (User, error) {
	var w struct {
		ID             UserID          `json:"id"`
		LinkedAccounts []linkedAccount `json:"linked_accounts"`
	}
	err := c.do(
		ctx,
		call{op: "privy.GetUser", method: http.MethodGet, path: "/v1/users/" + url.PathEscape(string(id))},
		&w,
	)
	if err != nil {
		return User{}, err
	}
	u := User{ID: w.ID}
	for _, a := range w.LinkedAccounts {
		u.link(a)
	}
	return u, nil
}

func (u *User) link(a linkedAccount) {
	switch a.Type {
	case "email":
		u.Email = a.Address
	case "apple_oauth":
		u.AppleEmail = a.Email
	case "google_oauth":
		u.GoogleEmail = a.Email
	case "phone":
		u.Phone = a.Number
	case "twitter_oauth":
		u.X = &XAccount{UserID: a.Subject, Username: a.Username}
	case "wallet":
		if a.ChainType == "solana" && a.WalletClientType == "privy" {
			u.EmbeddedWallet = &chain.Wallet{ID: a.ID, Address: chain.SolanaAddress(a.Address)}
		}
	}
}
