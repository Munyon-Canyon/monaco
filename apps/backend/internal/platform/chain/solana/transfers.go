package solana

import (
	"context"
	"encoding/json"
	"log/slog"
	"strconv"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

type Transfer struct {
	Signature chain.Signature
	From      chain.SolanaAddress
	Mint      chain.Mint
	Amount    money.BaseUnits
	Fee       money.BaseUnits
	Net       money.BaseUnits
}

type balanceWire struct {
	AccountIndex  int    `json:"accountIndex"`
	Mint          string `json:"mint"`
	Owner         string `json:"owner"`
	UITokenAmount struct {
		Amount   string `json:"amount"`
		Decimals uint8  `json:"decimals"`
	} `json:"uiTokenAmount"`
}

type instructionWire struct {
	Program string          `json:"program"`
	Parsed  json.RawMessage `json:"parsed"`
}

type transferWire struct {
	Type string `json:"type"`
	Info struct {
		Source      string `json:"source"`
		Destination string `json:"destination"`
		Authority   string `json:"authority"`
		Amount      string `json:"amount"`
		TokenAmount struct {
			Amount string `json:"amount"`
		} `json:"tokenAmount"`
		FeeAmount struct {
			Amount string `json:"amount"`
		} `json:"feeAmount"`
	} `json:"info"`
}

type parsedTransfer struct {
	program string
	transferWire
}

type tokenBalances struct {
	pre, post map[string]balanceWire
}

func (b tokenBalances) account(pubkey string) (balanceWire, bool) {
	if a, ok := b.post[pubkey]; ok {
		return a, true
	}
	a, ok := b.pre[pubkey]
	return a, ok
}

type transactionWire struct {
	Meta *struct {
		Err               any           `json:"err"`
		PreTokenBalances  []balanceWire `json:"preTokenBalances"`
		PostTokenBalances []balanceWire `json:"postTokenBalances"`
		InnerInstructions []struct {
			Instructions []instructionWire `json:"instructions"`
		} `json:"innerInstructions"`
	} `json:"meta"`
	Transaction struct {
		Message struct {
			AccountKeys []struct {
				Pubkey string `json:"pubkey"`
			} `json:"accountKeys"`
			Instructions []instructionWire `json:"instructions"`
		} `json:"message"`
	} `json:"transaction"`
}

func (c *Client) InboundTransfers(
	ctx context.Context,
	sig chain.Signature,
	owner chain.SolanaAddress,
) ([]Transfer, error) {
	const op = "solana.InboundTransfers"
	if err := addresses(op, owner); err != nil {
		return nil, err
	}
	var w *transactionWire
	opts := map[string]any{"encoding": "jsonParsed", "commitment": "finalized", "maxSupportedTransactionVersion": 0}
	if err := c.call(ctx, "getTransaction", []any{sig, opts}, &w); err != nil {
		return nil, err
	}
	if w == nil || w.Meta == nil {
		return nil, errs.New(errs.CodeNotFound, op, slog.String("signature", string(sig)))
	}
	if w.Meta.Err != nil {
		return nil, nil
	}
	balances := tokenAccounts(w)
	ixs := w.Transaction.Message.Instructions
	for _, inner := range w.Meta.InnerInstructions {
		ixs = append(ixs, inner.Instructions...)
	}
	var transfers []parsedTransfer
	for _, ix := range ixs {
		if p, ok := tokenTransfer(ix); ok {
			transfers = append(transfers, p)
		}
	}
	touches := token2022Touches(transfers)
	var out []Transfer
	for _, p := range transfers {
		t, ok, err := inbound(p, balances, touches, owner)
		if err != nil {
			return nil, errs.Wrap(err, errs.CodeDecodeFailed, op, slog.String("signature", string(sig)))
		}
		if ok {
			t.Signature = sig
			out = append(out, t)
		}
	}
	return out, nil
}

func tokenAccounts(w *transactionWire) tokenBalances {
	keys := w.Transaction.Message.AccountKeys
	index := func(bs []balanceWire) map[string]balanceWire {
		out := map[string]balanceWire{}
		for _, b := range bs {
			if b.AccountIndex >= 0 && b.AccountIndex < len(keys) {
				out[keys[b.AccountIndex].Pubkey] = b
			}
		}
		return out
	}
	return tokenBalances{pre: index(w.Meta.PreTokenBalances), post: index(w.Meta.PostTokenBalances)}
}

func token2022Touches(transfers []parsedTransfer) map[string]int {
	out := map[string]int{}
	for _, p := range transfers {
		if p.program == "spl-token-2022" && p.Info.FeeAmount.Amount == "" {
			out[p.Info.Destination]++
			if p.Info.Source != p.Info.Destination {
				out[p.Info.Source]++
			}
		}
	}
	return out
}

func inbound(
	p parsedTransfer, balances tokenBalances, touches map[string]int, owner chain.SolanaAddress,
) (Transfer, bool, error) {
	dest, known := balances.account(p.Info.Destination)
	if !known || dest.Owner != string(owner) {
		return Transfer{}, false, nil
	}
	from := p.Info.Authority
	if src, ok := balances.account(p.Info.Source); ok {
		from = src.Owner
	}
	if from == string(owner) {
		return Transfer{}, false, nil
	}
	decimals := dest.UITokenAmount.Decimals
	raw := p.Info.Amount
	if raw == "" {
		raw = p.Info.TokenAmount.Amount
	}
	amount, err := baseUnits(raw, decimals)
	if err != nil {
		return Transfer{}, false, err
	}
	fee, err := transferFee(p, amount, balances, touches)
	if err != nil {
		return Transfer{}, false, err
	}
	net, err := amount.Sub(fee)
	if err != nil {
		return Transfer{}, false, err
	}
	return Transfer{
		From:   chain.SolanaAddress(from),
		Mint:   chain.Mint{Address: chain.SolanaAddress(dest.Mint), Decimals: decimals},
		Amount: amount,
		Fee:    fee,
		Net:    net,
	}, true, nil
}

func transferFee(
	p parsedTransfer, amount money.BaseUnits, balances tokenBalances, touches map[string]int,
) (money.BaseUnits, error) {
	decimals := amount.Decimals()
	switch {
	case p.Info.FeeAmount.Amount != "":
		return baseUnits(p.Info.FeeAmount.Amount, decimals)
	case p.program == "spl-token":
		return money.NewBaseUnits(0, decimals), nil
	case touches[p.Info.Destination] > 1:
		return money.BaseUnits{}, errs.New(errs.CodeDecodeFailed, "solana.transferFee",
			slog.String("account", p.Info.Destination), slog.String("reason", "several transfers move this balance"))
	}
	pre := money.NewBaseUnits(0, decimals)
	if b, ok := balances.pre[p.Info.Destination]; ok {
		v, err := baseUnits(b.UITokenAmount.Amount, decimals)
		if err != nil {
			return money.BaseUnits{}, err
		}
		pre = v
	}
	post, err := baseUnits(balances.post[p.Info.Destination].UITokenAmount.Amount, decimals)
	if err != nil {
		return money.BaseUnits{}, err
	}
	received, err := post.Sub(pre)
	if err != nil {
		return money.BaseUnits{}, err
	}
	return amount.Sub(received)
}

func baseUnits(raw string, decimals uint8) (money.BaseUnits, error) {
	v, err := strconv.ParseUint(raw, 10, 64)
	if err != nil {
		return money.BaseUnits{}, errs.Wrap(err, errs.CodeDecodeFailed, "solana.baseUnits", slog.String("amount", raw))
	}
	return money.NewBaseUnits(v, decimals), nil
}

func tokenTransfer(ix instructionWire) (parsedTransfer, bool) {
	var p transferWire
	if ix.Program != "spl-token" && ix.Program != "spl-token-2022" || json.Unmarshal(ix.Parsed, &p) != nil {
		return parsedTransfer{}, false
	}
	switch p.Type {
	case "transfer", "transferChecked", "transferCheckedWithFee":
		return parsedTransfer{program: ix.Program, transferWire: p}, true
	}
	return parsedTransfer{}, false
}
