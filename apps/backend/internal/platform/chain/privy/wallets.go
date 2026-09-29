package privy

import (
	"context"
	"log/slog"
	"net/http"
	"net/url"
	"slices"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
)

const idempotencyHeader = "privy-idempotency-key"

type signerWire struct {
	SignerID string `json:"signer_id"`
}

type walletWire struct {
	ID                string       `json:"id"`
	Address           string       `json:"address"`
	OwnerID           string       `json:"owner_id"`
	AdditionalSigners []signerWire `json:"additional_signers"`
}

type createWallet struct {
	ChainType         string            `json:"chain_type"`
	Owner             map[string]string `json:"owner,omitempty"`
	OwnerID           string            `json:"owner_id,omitempty"`
	AdditionalSigners []signerWire      `json:"additional_signers,omitempty"`
}

func (c *Client) FindOrCreateMemberWallet(ctx context.Context, user UserID) (chain.Wallet, error) {
	const op = "privy.FindOrCreateMemberWallet"
	var list struct {
		Data []walletWire `json:"data"`
	}
	q := url.Values{"user_id": {string(user)}, "chain_type": {"solana"}}
	if err := c.do(ctx, call{op: op, method: http.MethodGet, path: "/v1/wallets?" + q.Encode()}, &list); err != nil {
		return chain.Wallet{}, err
	}
	if len(list.Data) > 0 {
		return c.wallet(op, list.Data[0])
	}
	body := createWallet{
		ChainType:         "solana",
		Owner:             map[string]string{"user_id": string(user)},
		AdditionalSigners: []signerWire{{SignerID: c.cfg.AuthorizationKeyID}},
	}
	return c.create(ctx, op, "member-wallet:"+string(user), body)
}

func (c *Client) CreateAppWallet(ctx context.Context, idempotencyKey string) (chain.Wallet, error) {
	const op = "privy.CreateAppWallet"
	if idempotencyKey == "" {
		return chain.Wallet{}, errs.New(errs.CodeInvalidInput, op, slog.String("reason", "idempotency key"))
	}
	return c.create(ctx, op, idempotencyKey, createWallet{ChainType: "solana", OwnerID: c.cfg.AuthorizationKeyID})
}

func (c *Client) create(ctx context.Context, op, key string, body createWallet) (chain.Wallet, error) {
	if c.cfg.AuthorizationKeyID == "" {
		return chain.Wallet{}, errs.New(errs.CodeInternal, op, slog.String("reason", "authorization key id missing"))
	}
	var w walletWire
	in := call{
		op:      op,
		method:  http.MethodPost,
		path:    "/v1/wallets",
		body:    body,
		headers: map[string]string{idempotencyHeader: key},
	}
	if err := c.do(ctx, in, &w); err != nil {
		return chain.Wallet{}, err
	}
	return c.wallet(op, w)
}

func (c *Client) wallet(op string, w walletWire) (chain.Wallet, error) {
	addr, err := chain.ParseAddress(w.Address)
	if err != nil || w.ID == "" {
		return chain.Wallet{}, errs.New(errs.CodeDecodeFailed, op, slog.String("wallet_id", w.ID))
	}
	app := c.cfg.AuthorizationKeyID
	signer := slices.ContainsFunc(w.AdditionalSigners, func(s signerWire) bool { return s.SignerID == app })
	return chain.Wallet{ID: w.ID, Address: addr, HasAppSigner: app != "" && (w.OwnerID == app || signer)}, nil
}
