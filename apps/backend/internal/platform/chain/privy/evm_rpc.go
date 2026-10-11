package privy

import (
	"context"
	"encoding/hex"
	"log/slog"
	"net/url"
	"strings"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/multichain"
)

var _ multichain.EVMWallets = (*Client)(nil)

type evmTxWire struct {
	To      string `json:"to"`
	Data    string `json:"data"`
	Value   string `json:"value"`
	ChainID int64  `json:"chain_id"`
}

type evmRPCBody struct {
	Method    string `json:"method"`
	CAIP2     string `json:"caip2"`
	ChainType string `json:"chain_type"`
	Sponsor   bool   `json:"sponsor,omitempty"`
	Params    struct {
		Transaction evmTxWire `json:"transaction"`
	} `json:"params"`
}

func (c *Client) evmRPC(op, method string, tx multichain.EVMTransaction, sponsor bool) (evmRPCBody, error) {
	if c.authKey == nil {
		return evmRPCBody{}, errs.New(errs.CodeInternal, op, slog.String("reason", "authorization key missing"))
	}
	if !tx.Network.IsEVM() {
		return evmRPCBody{}, errs.New(errs.CodeInvalidNetwork, op, slog.String("network", string(tx.Network)))
	}
	value := "0x0"
	if tx.Value != nil {
		if tx.Value.Sign() < 0 {
			return evmRPCBody{}, errs.New(errs.CodeInvalidInput, op, slog.String("reason", "negative value"))
		}
		value = "0x" + tx.Value.Text(16)
	}
	body := evmRPCBody{Method: method, CAIP2: tx.Network.CAIP2(), ChainType: "ethereum", Sponsor: sponsor}
	body.Params.Transaction = evmTxWire{
		To: tx.To.String(), Data: "0x" + hex.EncodeToString(tx.Data), Value: value, ChainID: tx.Network.EVMChainID(),
	}
	return body, nil
}

func (c *Client) SignEVMTransaction(
	ctx context.Context, walletID string, tx multichain.EVMTransaction, sponsor bool,
) ([]byte, error) {
	const op = "privy.SignEVMTransaction"
	if sponsor {
		return nil, errs.New(errs.CodeInvalidInput, op, slog.String("reason", "sponsorship applies to sends only"))
	}
	body, err := c.evmRPC(op, "eth_signTransaction", tx, false)
	if err != nil {
		return nil, err
	}
	var w struct {
		Data struct {
			SignedTransaction string `json:"signed_transaction"`
		} `json:"data"`
	}
	path := "/v1/wallets/" + url.PathEscape(walletID) + "/rpc"
	if err := c.do(ctx, c.signedPost(op, path, body, tx.IdempotencyKey), &w); err != nil {
		return nil, err
	}
	raw, ok := strings.CutPrefix(w.Data.SignedTransaction, "0x")
	signed, err := hex.DecodeString(raw)
	if !ok || err != nil || len(signed) == 0 {
		return nil, errs.New(errs.CodeDecodeFailed, op, slog.String("wallet_id", walletID))
	}
	return signed, nil
}

func (c *Client) SendEVMTransaction(
	ctx context.Context, walletID string, tx multichain.EVMTransaction, sponsor bool,
) (string, error) {
	const op = "privy.SendEVMTransaction"
	if tx.IdempotencyKey == "" {
		return "", errs.New(errs.CodeInvalidInput, op, slog.String("reason", "idempotency key"))
	}
	body, err := c.evmRPC(op, "eth_sendTransaction", tx, sponsor)
	if err != nil {
		return "", err
	}
	var w struct {
		Data struct {
			Hash              string `json:"hash"`
			UserOperationHash string `json:"user_operation_hash"`
		} `json:"data"`
	}
	path := "/v1/wallets/" + url.PathEscape(walletID) + "/rpc"
	if err := c.do(ctx, c.signedPost(op, path, body, tx.IdempotencyKey), &w); err != nil {
		return "", err
	}
	hash := w.Data.Hash
	if hash == "" {
		hash = w.Data.UserOperationHash
	}
	if !evmHash(hash) {
		return "", errs.New(errs.CodeDecodeFailed, op, slog.String("wallet_id", walletID))
	}
	return strings.ToLower(hash), nil
}

func evmHash(s string) bool {
	raw, ok := strings.CutPrefix(s, "0x")
	b, err := hex.DecodeString(raw)
	return ok && err == nil && len(b) == 32
}
