package solana

import (
	"context"
	"log/slog"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

func (c *Client) Received(
	ctx context.Context, sig chain.Signature, owner chain.SolanaAddress, mint chain.Mint,
) (money.BaseUnits, error) {
	const op = "solana.Received"
	if err := addresses(op, owner); err != nil {
		return money.BaseUnits{}, err
	}
	var w *transactionWire
	opts := map[string]any{"encoding": "jsonParsed", "commitment": "confirmed", "maxSupportedTransactionVersion": 0}
	if err := c.call(ctx, "getTransaction", []any{sig, opts}, &w); err != nil {
		return money.BaseUnits{}, err
	}
	if w == nil || w.Meta == nil {
		return money.BaseUnits{}, errs.New(errs.CodeNotFound, op, slog.String("signature", string(sig)))
	}
	zero := money.NewBaseUnits(0, mint.Decimals)
	if w.Meta.Err != nil {
		return zero, nil
	}
	pre, err := ownedTotal(w.Meta.PreTokenBalances, owner, mint)
	if err != nil {
		return money.BaseUnits{}, errs.Wrap(err, errs.CodeDecodeFailed, op, slog.String("signature", string(sig)))
	}
	post, err := ownedTotal(w.Meta.PostTokenBalances, owner, mint)
	if err != nil {
		return money.BaseUnits{}, errs.Wrap(err, errs.CodeDecodeFailed, op, slog.String("signature", string(sig)))
	}
	got, err := post.Sub(pre)
	if err != nil {
		return money.BaseUnits{}, errs.Wrap(err, errs.CodeDecodeFailed, op, slog.String("signature", string(sig)),
			slog.String("pre", pre.String()), slog.String("post", post.String()))
	}
	return got, nil
}

func ownedTotal(balances []balanceWire, owner chain.SolanaAddress, mint chain.Mint) (money.BaseUnits, error) {
	total := money.NewBaseUnits(0, mint.Decimals)
	for _, b := range balances {
		if b.Owner != string(owner) || b.Mint != string(mint.Address) {
			continue
		}
		if b.UITokenAmount.Decimals != mint.Decimals {
			return money.BaseUnits{}, errs.New(errs.CodeDecodeFailed, "solana.ownedTotal",
				slog.Int("decimals", int(b.UITokenAmount.Decimals)), slog.Int("want", int(mint.Decimals)))
		}
		v, err := baseUnits(b.UITokenAmount.Amount, mint.Decimals)
		if err != nil {
			return money.BaseUnits{}, err
		}
		if total, err = total.Add(v); err != nil {
			return money.BaseUnits{}, err
		}
	}
	return total, nil
}
