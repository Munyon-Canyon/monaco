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
}

type balanceWire struct {
	AccountIndex  int    `json:"accountIndex"`
	Mint          string `json:"mint"`
	Owner         string `json:"owner"`
	UITokenAmount struct {
		Decimals uint8 `json:"decimals"`
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
	} `json:"info"`
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
	accounts := tokenAccounts(w)
	ixs := w.Transaction.Message.Instructions
	for _, inner := range w.Meta.InnerInstructions {
		ixs = append(ixs, inner.Instructions...)
	}
	var out []Transfer
	for _, ix := range ixs {
		t, ok, err := inbound(ix, accounts, owner)
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

func tokenAccounts(w *transactionWire) map[string]balanceWire {
	keys := w.Transaction.Message.AccountKeys
	out := map[string]balanceWire{}
	for _, b := range append(w.Meta.PreTokenBalances, w.Meta.PostTokenBalances...) {
		if b.AccountIndex >= 0 && b.AccountIndex < len(keys) {
			out[keys[b.AccountIndex].Pubkey] = b
		}
	}
	return out
}

func inbound(ix instructionWire, accounts map[string]balanceWire, owner chain.SolanaAddress) (Transfer, bool, error) {
	p, ok := tokenTransfer(ix)
	dest, known := accounts[p.Info.Destination]
	if !ok || !known || dest.Owner != string(owner) {
		return Transfer{}, false, nil
	}
	from := p.Info.Authority
	if src, ok := accounts[p.Info.Source]; ok {
		from = src.Owner
	}
	if from == string(owner) {
		return Transfer{}, false, nil
	}
	raw := p.Info.Amount
	if raw == "" {
		raw = p.Info.TokenAmount.Amount
	}
	v, err := strconv.ParseUint(raw, 10, 64)
	if err != nil {
		return Transfer{}, false, errs.Wrap(err, errs.CodeDecodeFailed, "solana.inbound", slog.String("amount", raw))
	}
	decimals := dest.UITokenAmount.Decimals
	return Transfer{
		From:   chain.SolanaAddress(from),
		Mint:   chain.Mint{Address: chain.SolanaAddress(dest.Mint), Decimals: decimals},
		Amount: money.NewBaseUnits(v, decimals),
	}, true, nil
}

func tokenTransfer(ix instructionWire) (transferWire, bool) {
	var p transferWire
	if ix.Program != "spl-token" && ix.Program != "spl-token-2022" || json.Unmarshal(ix.Parsed, &p) != nil {
		return transferWire{}, false
	}
	return p, p.Type == "transfer" || p.Type == "transferChecked"
}
