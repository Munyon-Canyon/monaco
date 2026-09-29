package solana

import (
	"context"
	"encoding/json"
	"log/slog"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

type MintConfig struct {
	Mint           chain.Mint
	TokenProgram   chain.SolanaAddress
	TransferFeeBps uint16
	MaxFee         money.BaseUnits
}

type feeWire struct {
	Epoch                  uint64 `json:"epoch"`
	MaximumFee             uint64 `json:"maximumFee"`
	TransferFeeBasisPoints uint16 `json:"transferFeeBasisPoints"`
}

type mintWire struct {
	Value *struct {
		Owner chain.SolanaAddress `json:"owner"`
		Data  struct {
			Parsed struct {
				Type string `json:"type"`
				Info struct {
					Decimals   uint8 `json:"decimals"`
					Extensions []struct {
						Extension string          `json:"extension"`
						State     json.RawMessage `json:"state"`
					} `json:"extensions"`
				} `json:"info"`
			} `json:"parsed"`
		} `json:"data"`
	} `json:"value"`
}

func (c *Client) MintConfig(ctx context.Context, mint chain.SolanaAddress) (MintConfig, error) {
	const op = "solana.MintConfig"
	if err := addresses(op, mint); err != nil {
		return MintConfig{}, err
	}
	c.mu.Lock()
	hit, ok := c.mints[mint]
	c.mu.Unlock()
	if ok && c.clock.Now().Sub(hit.at) < mintTTL {
		return hit.cfg, nil
	}
	cfg, err := c.fetchMint(ctx, op, mint)
	if err != nil {
		return MintConfig{}, err
	}
	c.mu.Lock()
	c.mints[mint] = cachedMint{cfg: cfg, at: c.clock.Now()}
	c.mu.Unlock()
	return cfg, nil
}

func (c *Client) fetchMint(ctx context.Context, op string, mint chain.SolanaAddress) (MintConfig, error) {
	var w mintWire
	opts := map[string]string{"encoding": "jsonParsed", "commitment": "confirmed"}
	if err := c.call(ctx, "getAccountInfo", []any{mint, opts}, &w); err != nil {
		return MintConfig{}, err
	}
	if w.Value == nil {
		return MintConfig{}, errs.New(errs.CodeNotFound, op, slog.String("mint", string(mint)))
	}
	v := w.Value
	if v.Owner != chain.SPLProgram && v.Owner != chain.SPL2022Program || v.Data.Parsed.Type != "mint" {
		return MintConfig{}, errs.New(errs.CodeInvalidAddress, op, slog.String("mint", string(mint)))
	}
	decimals := v.Data.Parsed.Info.Decimals
	out := MintConfig{
		Mint:         chain.Mint{Address: mint, Decimals: decimals},
		TokenProgram: v.Owner,
		MaxFee:       money.NewBaseUnits(0, decimals),
	}
	for _, ext := range v.Data.Parsed.Info.Extensions {
		if ext.Extension != "transferFeeConfig" {
			continue
		}
		fee, err := c.currentFee(ctx, op, ext.State)
		if err != nil {
			return MintConfig{}, err
		}
		out.TransferFeeBps, out.MaxFee = fee.TransferFeeBasisPoints, money.NewBaseUnits(fee.MaximumFee, decimals)
	}
	return out, nil
}

func (c *Client) currentFee(ctx context.Context, op string, state json.RawMessage) (feeWire, error) {
	var fees struct {
		Older feeWire `json:"olderTransferFee"`
		Newer feeWire `json:"newerTransferFee"`
	}
	if err := json.Unmarshal(state, &fees); err != nil {
		return feeWire{}, errs.Wrap(err, errs.CodeDecodeFailed, op)
	}
	var epoch struct {
		Epoch uint64 `json:"epoch"`
	}
	if err := c.call(ctx, "getEpochInfo", []any{commitment("confirmed")}, &epoch); err != nil {
		return feeWire{}, err
	}
	if epoch.Epoch >= fees.Newer.Epoch {
		return fees.Newer, nil
	}
	return fees.Older, nil
}
