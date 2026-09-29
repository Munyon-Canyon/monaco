package solana

import (
	"context"
	"log/slog"
	"strconv"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

const lamportDecimals = 9

type State uint8

const (
	StateNotFound State = iota + 1
	StateProcessing
	StateFinalized
)

type Status struct {
	Signature   chain.Signature
	State       State
	Failed      bool
	BlockHeight uint64
}

func (s Status) BlockhashExpired(lastValidBlockHeight uint64) bool {
	return s.State == StateNotFound && s.BlockHeight > lastValidBlockHeight
}

type SignatureInfo struct {
	Signature chain.Signature
	Slot      uint64
	Failed    bool
}

func commitment(level string) map[string]string { return map[string]string{"commitment": level} }

func (c *Client) SOLBalance(ctx context.Context, addr chain.SolanaAddress) (money.BaseUnits, error) {
	if err := addresses("solana.SOLBalance", addr); err != nil {
		return money.BaseUnits{}, err
	}
	var w struct {
		Value uint64 `json:"value"`
	}
	err := c.call(ctx, "getBalance", []any{addr, commitment("confirmed")}, &w)
	return money.NewBaseUnits(w.Value, lamportDecimals), err
}

type tokenAccountsWire struct {
	Value []struct {
		Account struct {
			Data struct {
				Parsed struct {
					Info struct {
						TokenAmount struct {
							Amount   string `json:"amount"`
							Decimals uint8  `json:"decimals"`
						} `json:"tokenAmount"`
					} `json:"info"`
				} `json:"parsed"`
			} `json:"data"`
		} `json:"account"`
	} `json:"value"`
}

func (c *Client) TokenBalance(
	ctx context.Context,
	owner chain.SolanaAddress,
	mint chain.Mint,
) (money.BaseUnits, error) {
	const op = "solana.TokenBalance"
	total := money.NewBaseUnits(0, mint.Decimals)
	if err := addresses(op, owner, mint.Address); err != nil {
		return total, err
	}
	var w tokenAccountsWire
	opts := map[string]string{"encoding": "jsonParsed", "commitment": "confirmed"}
	if err := c.call(
		ctx,
		"getTokenAccountsByOwner",
		[]any{owner, map[string]any{"mint": mint.Address}, opts},
		&w,
	); err != nil {
		return total, err
	}
	for _, acct := range w.Value {
		amt := acct.Account.Data.Parsed.Info.TokenAmount
		v, err := strconv.ParseUint(amt.Amount, 10, 64)
		if err != nil {
			return total, errs.Wrap(err, errs.CodeDecodeFailed, op, slog.String("amount", amt.Amount))
		}
		if total, err = total.Add(money.NewBaseUnits(v, amt.Decimals)); err != nil {
			return money.NewBaseUnits(0, mint.Decimals), errs.Wrap(err, errs.CodeDecodeFailed, op)
		}
	}
	return total, nil
}

type statusWire struct {
	Err                any    `json:"err"`
	ConfirmationStatus string `json:"confirmationStatus"`
}

func (c *Client) SignatureStatuses(ctx context.Context, sigs []chain.Signature) ([]Status, error) {
	const op = "solana.SignatureStatuses"
	if len(sigs) == 0 {
		return nil, nil
	}
	if len(sigs) > maxStatuses {
		return nil, errs.New(errs.CodeInvalidInput, op, slog.Int("signatures", len(sigs)))
	}
	var height uint64
	if err := c.call(ctx, "getBlockHeight", []any{commitment("finalized")}, &height); err != nil {
		return nil, err
	}
	var w struct {
		Value []*statusWire `json:"value"`
	}
	opts := map[string]bool{"searchTransactionHistory": true}
	if err := c.call(ctx, "getSignatureStatuses", []any{sigs, opts}, &w); err != nil {
		return nil, err
	}
	if len(w.Value) != len(sigs) {
		return nil, errs.New(
			errs.CodeDecodeFailed,
			op,
			slog.Int("statuses", len(w.Value)),
			slog.Int("signatures", len(sigs)),
		)
	}
	out := make([]Status, len(sigs))
	for i, s := range w.Value {
		out[i] = Status{Signature: sigs[i], State: StateNotFound, BlockHeight: height}
		if s == nil {
			continue
		}
		out[i].State, out[i].Failed = StateProcessing, s.Err != nil
		if s.ConfirmationStatus == "finalized" {
			out[i].State = StateFinalized
		}
	}
	return out, nil
}

func (c *Client) SignaturesFor(
	ctx context.Context, addr chain.SolanaAddress, before chain.Signature, limit int,
) ([]SignatureInfo, error) {
	if err := addresses("solana.SignaturesFor", addr); err != nil {
		return nil, err
	}
	opts := map[string]any{"limit": limit, "commitment": "finalized"}
	if before != "" {
		opts["before"] = before
	}
	var w []struct {
		Signature chain.Signature `json:"signature"`
		Slot      uint64          `json:"slot"`
		Err       any             `json:"err"`
	}
	if err := c.call(ctx, "getSignaturesForAddress", []any{addr, opts}, &w); err != nil {
		return nil, err
	}
	out := make([]SignatureInfo, len(w))
	for i, s := range w {
		out[i] = SignatureInfo{Signature: s.Signature, Slot: s.Slot, Failed: s.Err != nil}
	}
	return out, nil
}
