package solana

import (
	"context"
	"log/slog"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
)

func (c *Client) TransactionSignatures(ctx context.Context, sig chain.Signature) ([]chain.Signature, error) {
	const op = "solana.TransactionSignatures"
	var w *struct {
		Transaction struct {
			Signatures []chain.Signature `json:"signatures"`
		} `json:"transaction"`
	}
	opts := map[string]any{"encoding": "json", "commitment": "finalized", "maxSupportedTransactionVersion": 0}
	if err := c.call(ctx, "getTransaction", []any{sig, opts}, &w); err != nil {
		return nil, err
	}
	if w == nil {
		return nil, errs.New(errs.CodeNotFound, op, slog.String("signature", string(sig)))
	}
	return w.Transaction.Signatures, nil
}
