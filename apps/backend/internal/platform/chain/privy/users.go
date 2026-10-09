package privy

import (
	"context"
	"log/slog"
	"net/http"
	"net/url"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
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

type emailAccount struct {
	Type    string `json:"type"`
	Address string `json:"address"`
}

func (c *Client) CreateUser(ctx context.Context, email string) (UserID, error) {
	var out struct {
		ID UserID `json:"id"`
	}
	err := c.do(ctx, call{
		op: "privy.CreateUser", method: http.MethodPost, path: "/v1/users",
		body: struct {
			LinkedAccounts []emailAccount `json:"linked_accounts"`
		}{LinkedAccounts: []emailAccount{{Type: "email", Address: email}}},
	}, &out)
	if err != nil {
		return "", err
	}
	if out.ID == "" {
		return "", errs.New(errs.CodeDecodeFailed, "privy.CreateUser", slog.String("reason", "no_id"))
	}
	return out.ID, nil
}

func (c *Client) DeleteUser(ctx context.Context, id UserID) error {
	return c.do(ctx, call{
		op: "privy.DeleteUser", method: http.MethodDelete, path: "/v1/users/" + url.PathEscape(string(id)),
	}, nil)
}

type ListedUser struct {
	ID        UserID
	CreatedAt time.Time
	Accounts  int
	Email     string
}

func (c *Client) ListUsers(ctx context.Context) ([]ListedUser, error) {
	var all []ListedUser
	cursor := ""
	for {
		var page struct {
			Data []struct {
				ID             UserID          `json:"id"`
				CreatedAt      int64           `json:"created_at"`
				LinkedAccounts []linkedAccount `json:"linked_accounts"`
			} `json:"data"`
			NextCursor string `json:"next_cursor"`
		}
		path := "/v1/users"
		if cursor != "" {
			path += "?cursor=" + url.QueryEscape(cursor)
		}
		if err := c.do(ctx, call{op: "privy.ListUsers", method: http.MethodGet, path: path}, &page); err != nil {
			return nil, err
		}
		for _, w := range page.Data {
			u := User{ID: w.ID}
			for _, a := range w.LinkedAccounts {
				u.link(a)
			}
			all = append(all, ListedUser{
				ID: w.ID, CreatedAt: time.Unix(w.CreatedAt, 0).UTC(), Accounts: len(w.LinkedAccounts), Email: u.Email,
			})
		}
		if page.NextCursor == "" {
			return all, nil
		}
		cursor = page.NextCursor
	}
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
