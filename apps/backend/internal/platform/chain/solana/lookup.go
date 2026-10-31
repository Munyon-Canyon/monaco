package solana

import (
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"log/slog"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
)

const (
	LookupTableProgram chain.SolanaAddress = "AddressLookupTab1e1111111111111111111111111"

	lookupTableMeta = 56
	maxTables       = 100
)

type lookupAccountWire struct {
	Owner string    `json:"owner"`
	Data  [2]string `json:"data"`
}

func (c *Client) LookupTables(
	ctx context.Context, tables []chain.SolanaAddress,
) (chain.LookupTables, error) {
	const op = "solana.LookupTables"
	if len(tables) > maxTables {
		return nil, errs.New(errs.CodeInvalidInput, op, slog.Int("tables", len(tables)))
	}
	if err := addresses(op, tables...); err != nil {
		return nil, err
	}
	var wire struct {
		Value []*lookupAccountWire `json:"value"`
	}
	opts := map[string]any{"encoding": "base64", "commitment": "confirmed"}
	if err := c.call(ctx, "getMultipleAccounts", []any{tables, opts}, &wire); err != nil {
		return nil, err
	}
	if len(wire.Value) != len(tables) {
		return nil, errs.New(errs.CodeDecodeFailed, op)
	}
	out := make(chain.LookupTables, len(tables))
	for i, t := range tables {
		entries, err := lookupEntries(op, t, wire.Value[i])
		if err != nil {
			return nil, err
		}
		out[t] = entries
	}
	return out, nil
}

func lookupEntries(op string, table chain.SolanaAddress, in *lookupAccountWire) ([]chain.SolanaAddress, error) {
	if in == nil || in.Owner != string(LookupTableProgram) || in.Data[1] != "base64" {
		return nil, errs.New(errs.CodeDecodeFailed, op, slog.String("table", string(table)))
	}
	raw, err := base64.StdEncoding.DecodeString(in.Data[0])
	if err != nil || len(raw) < lookupTableMeta || (len(raw)-lookupTableMeta)%ed25519.PublicKeySize != 0 {
		return nil, errs.New(errs.CodeDecodeFailed, op, slog.String("table", string(table)))
	}
	entries := make([]chain.SolanaAddress, 0, (len(raw)-lookupTableMeta)/ed25519.PublicKeySize)
	for rest := raw[lookupTableMeta:]; len(rest) > 0; rest = rest[ed25519.PublicKeySize:] {
		entries = append(entries, chain.AddressOf(rest[:ed25519.PublicKeySize]))
	}
	return entries, nil
}
