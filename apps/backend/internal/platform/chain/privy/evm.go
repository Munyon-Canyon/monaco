package privy

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/multichain"
)

func (c *Client) CreateAppEVMWallet(ctx context.Context, key string) (multichain.EVMWallet, error) {
	const op = "privy.CreateAppEVMWallet"
	if key == "" {
		return multichain.EVMWallet{}, errs.New(errs.CodeInvalidInput, op, slog.String("reason", "idempotency key"))
	}
	if c.cfg.AuthorizationKeyID == "" {
		return multichain.EVMWallet{}, errs.New(
			errs.CodeInternal,
			op,
			slog.String("reason", "authorization key id missing"),
		)
	}
	var w walletWire
	in := call{
		op:      op,
		method:  http.MethodPost,
		path:    "/v1/wallets",
		body:    createWallet{ChainType: "ethereum", OwnerID: c.cfg.AuthorizationKeyID},
		headers: map[string]string{idempotencyHeader: key},
	}
	if err := c.do(ctx, in, &w); err != nil {
		return multichain.EVMWallet{}, err
	}
	addr, err := chain.ParseEVMAddress(w.Address)
	if err != nil || w.ID == "" || w.OwnerID != c.cfg.AuthorizationKeyID {
		return multichain.EVMWallet{}, errs.New(errs.CodeDecodeFailed, op, slog.String("wallet_id", w.ID))
	}
	return multichain.EVMWallet{ID: w.ID, Address: addr}, nil
}
