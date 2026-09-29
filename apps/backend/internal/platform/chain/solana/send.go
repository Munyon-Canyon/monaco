package solana

import (
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"log/slog"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
)

type Blockhash struct {
	Hash                 [32]byte
	LastValidBlockHeight uint64
}

func (c *Client) LatestBlockhash(ctx context.Context) (Blockhash, error) {
	const op = "solana.LatestBlockhash"
	var w struct {
		Value struct {
			Blockhash            string `json:"blockhash"`
			LastValidBlockHeight uint64 `json:"lastValidBlockHeight"`
		} `json:"value"`
	}
	if err := c.call(ctx, "getLatestBlockhash", []any{commitment("confirmed")}, &w); err != nil {
		return Blockhash{}, err
	}
	raw, ok := chain.DecodeBase58(w.Value.Blockhash)
	if !ok || len(raw) != len(Blockhash{}.Hash) {
		return Blockhash{}, errs.New(errs.CodeDecodeFailed, op, slog.String("blockhash", w.Value.Blockhash))
	}
	out := Blockhash{LastValidBlockHeight: w.Value.LastValidBlockHeight}
	copy(out.Hash[:], raw)
	return out, nil
}

func (c *Client) SendTransaction(ctx context.Context, tx []byte) (chain.Signature, error) {
	const op = "solana.SendTransaction"
	var sig string
	opts := map[string]string{"encoding": "base64", "preflightCommitment": "confirmed"}
	if err := c.call(ctx, "sendTransaction", []any{base64.StdEncoding.EncodeToString(tx), opts}, &sig); err != nil {
		return "", err
	}
	if raw, ok := chain.DecodeBase58(sig); !ok || len(raw) != ed25519.SignatureSize {
		return "", errs.New(errs.CodeDecodeFailed, op)
	}
	return chain.Signature(sig), nil
}
