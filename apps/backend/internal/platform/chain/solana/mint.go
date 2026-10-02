package solana

import (
	"context"
	"encoding/json"
	"log/slog"
	"maps"
	"strconv"
	"strings"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

const maxMultiplierScale = 19

type Multiplier struct {
	Num, Den uint64
}

type MintConfig struct {
	Mint           chain.Mint
	TokenProgram   chain.SolanaAddress
	TransferFeeBps uint16
	MaxFee         money.BaseUnits
	UIMultiplier   Multiplier
}

type feeWire struct {
	Epoch                  uint64 `json:"epoch"`
	MaximumFee             uint64 `json:"maximumFee"`
	TransferFeeBasisPoints uint16 `json:"transferFeeBasisPoints"`
}

type scaledUIWire struct {
	Multiplier    string `json:"multiplier"`
	NewMultiplier string `json:"newMultiplier"`
	NewFrom       int64  `json:"newMultiplierEffectiveTimestamp"`
}

type mintWire struct {
	Value *mintAccount `json:"value"`
}

type mintsWire struct {
	Value []json.RawMessage `json:"value"`
}

type mintAccount struct {
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

func (c *Client) MintConfigs(
	ctx context.Context, mints []chain.SolanaAddress,
) (map[chain.SolanaAddress]MintConfig, map[chain.SolanaAddress]error, error) {
	const op = "solana.MintConfigs"
	if err := addresses(op, mints...); err != nil {
		return nil, nil, err
	}
	configs, uncached := c.cachedMintConfigs(mints)
	failures := map[chain.SolanaAddress]error{}
	var epoch *uint64
	for len(uncached) > 0 {
		chunk := uncached[:min(len(uncached), 100)]
		uncached = uncached[len(chunk):]
		got, failed, nextEpoch, err := c.fetchMintConfigsChunk(ctx, op, chunk, epoch)
		epoch = nextEpoch
		maps.Copy(configs, got)
		maps.Copy(failures, failed)
		c.cacheMintConfigs(got)
		if err != nil {
			return configs, failures, err
		}
	}
	return configs, failures, nil
}

func (c *Client) cachedMintConfigs(
	mints []chain.SolanaAddress,
) (map[chain.SolanaAddress]MintConfig, []chain.SolanaAddress) {
	configs := map[chain.SolanaAddress]MintConfig{}
	uncached := make([]chain.SolanaAddress, 0, len(mints))
	for _, mint := range mints {
		c.mu.Lock()
		hit, ok := c.mints[mint]
		c.mu.Unlock()
		if ok && c.clock.Now().Sub(hit.at) < mintTTL {
			configs[mint] = hit.cfg
		} else {
			uncached = append(uncached, mint)
		}
	}
	return configs, uncached
}

func (c *Client) fetchMintConfigsChunk(
	ctx context.Context, op string, mints []chain.SolanaAddress, epoch *uint64,
) (map[chain.SolanaAddress]MintConfig, map[chain.SolanaAddress]error, *uint64, error) {
	var wire mintsWire
	opts := map[string]string{"encoding": "jsonParsed", "commitment": "confirmed"}
	if err := c.call(ctx, "getMultipleAccounts", []any{mints, opts}, &wire); err != nil {
		return nil, nil, epoch, err
	}
	accounts, failures := mintAccounts(op, mints, wire.Value)
	var epochErr error
	if epoch == nil && needsEpoch(accounts) {
		var epochWire struct {
			Epoch uint64 `json:"epoch"`
		}
		if err := c.call(ctx, "getEpochInfo", []any{commitment("confirmed")}, &epochWire); err != nil {
			epochErr = err
		} else {
			epoch = &epochWire.Epoch
		}
	}
	configs := map[chain.SolanaAddress]MintConfig{}
	for mint, account := range accounts {
		if !validMint(account) {
			failures[mint] = errs.New(errs.CodeInvalidAddress, op, slog.String("mint", string(mint)))
			continue
		}
		if epochErr != nil && hasTransferFee(account) {
			failures[mint] = epochErr
			continue
		}
		cfg, err := c.parseMint(ctx, op, mint, account, epoch)
		if err != nil {
			failures[mint] = err
			continue
		}
		configs[mint] = cfg
	}
	return configs, failures, epoch, epochErr
}

func mintAccounts(
	op string, mints []chain.SolanaAddress, values []json.RawMessage,
) (map[chain.SolanaAddress]*mintAccount, map[chain.SolanaAddress]error) {
	accounts := make(map[chain.SolanaAddress]*mintAccount, len(mints))
	failures := map[chain.SolanaAddress]error{}
	for i, mint := range mints {
		if i >= len(values) {
			failures[mint] = errs.New(errs.CodeNotFound, op, slog.String("mint", string(mint)))
			continue
		}
		var account *mintAccount
		if err := json.Unmarshal(values[i], &account); err != nil {
			failures[mint] = errs.Wrap(err, errs.CodeDecodeFailed, op, slog.String("mint", string(mint)))
			continue
		}
		if account == nil {
			failures[mint] = errs.New(errs.CodeNotFound, op, slog.String("mint", string(mint)))
			continue
		}
		accounts[mint] = account
	}
	return accounts, failures
}

func (c *Client) cacheMintConfigs(configs map[chain.SolanaAddress]MintConfig) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for mint, cfg := range configs {
		c.mints[mint] = cachedMint{cfg: cfg, at: c.clock.Now()}
	}
}

func needsEpoch(accounts map[chain.SolanaAddress]*mintAccount) bool {
	for _, account := range accounts {
		if hasTransferFee(account) {
			return true
		}
	}
	return false
}

func hasTransferFee(account *mintAccount) bool {
	for _, ext := range account.Data.Parsed.Info.Extensions {
		if ext.Extension == "transferFeeConfig" {
			return true
		}
	}
	return false
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
	if !validMint(w.Value) {
		return MintConfig{}, errs.New(errs.CodeInvalidAddress, op, slog.String("mint", string(mint)))
	}
	return c.parseMint(ctx, op, mint, w.Value, nil)
}

func validMint(v *mintAccount) bool {
	return (v.Owner == chain.SPLProgram || v.Owner == chain.SPL2022Program) && v.Data.Parsed.Type == "mint"
}

func (c *Client) parseMint(
	ctx context.Context, op string, mint chain.SolanaAddress, v *mintAccount, batchEpoch *uint64,
) (MintConfig, error) {
	decimals := v.Data.Parsed.Info.Decimals
	out := MintConfig{
		Mint:         chain.Mint{Address: mint, Decimals: decimals},
		TokenProgram: v.Owner,
		MaxFee:       money.NewBaseUnits(0, decimals),
		UIMultiplier: Multiplier{Num: 1, Den: 1},
	}
	for _, ext := range v.Data.Parsed.Info.Extensions {
		var err error
		switch ext.Extension {
		case "transferFeeConfig":
			if batchEpoch == nil {
				out.TransferFeeBps, out.MaxFee, err = c.currentFee(ctx, op, ext.State, decimals)
			} else {
				out.TransferFeeBps, out.MaxFee, err = currentFeeAtEpoch(op, ext.State, decimals, *batchEpoch)
			}
		case "scaledUiAmountConfig":
			out.UIMultiplier, err = c.currentMultiplier(op, ext.State)
		}
		if err != nil {
			return MintConfig{}, err
		}
	}
	return out, nil
}

func (c *Client) currentFee(
	ctx context.Context, op string, state json.RawMessage, decimals uint8,
) (uint16, money.BaseUnits, error) {
	fees, err := decodeFees(op, state)
	if err != nil {
		return 0, money.BaseUnits{}, err
	}
	var epoch struct {
		Epoch uint64 `json:"epoch"`
	}
	if err := c.call(ctx, "getEpochInfo", []any{commitment("confirmed")}, &epoch); err != nil {
		return 0, money.BaseUnits{}, err
	}
	return fees.atEpoch(decimals, epoch.Epoch)
}

func currentFeeAtEpoch(
	op string, state json.RawMessage, decimals uint8, epoch uint64,
) (uint16, money.BaseUnits, error) {
	fees, err := decodeFees(op, state)
	if err != nil {
		return 0, money.BaseUnits{}, err
	}
	return fees.atEpoch(decimals, epoch)
}

type transferFees struct {
	Older feeWire `json:"olderTransferFee"`
	Newer feeWire `json:"newerTransferFee"`
}

func decodeFees(op string, state json.RawMessage) (transferFees, error) {
	var fees transferFees
	if err := json.Unmarshal(state, &fees); err != nil {
		return transferFees{}, errs.Wrap(err, errs.CodeDecodeFailed, op)
	}
	return fees, nil
}

func (fees transferFees) atEpoch(decimals uint8, epoch uint64) (uint16, money.BaseUnits, error) {
	fee := fees.Older
	if epoch >= fees.Newer.Epoch {
		fee = fees.Newer
	}
	return fee.TransferFeeBasisPoints, money.NewBaseUnits(fee.MaximumFee, decimals), nil
}

func (c *Client) currentMultiplier(op string, state json.RawMessage) (Multiplier, error) {
	var scaled scaledUIWire
	if err := json.Unmarshal(state, &scaled); err != nil {
		return Multiplier{}, errs.Wrap(err, errs.CodeDecodeFailed, op)
	}
	raw := scaled.Multiplier
	if c.clock.Now().Unix() >= scaled.NewFrom {
		raw = scaled.NewMultiplier
	}
	return parseMultiplier(op, raw)
}

func parseMultiplier(op, raw string) (Multiplier, error) {
	whole, frac, _ := strings.Cut(raw, ".")
	num, err := strconv.ParseUint(whole+frac, 10, 64)
	if err != nil || num == 0 || whole == "" || len(frac) > maxMultiplierScale {
		return Multiplier{}, errs.New(errs.CodeDecodeFailed, op, slog.String("multiplier", raw))
	}
	den := uint64(1)
	for range len(frac) {
		den *= 10
	}
	g := gcd(num, den)
	return Multiplier{Num: num / g, Den: den / g}, nil
}

func gcd(a, b uint64) uint64 {
	for b != 0 {
		a, b = b, a%b
	}
	return a
}
