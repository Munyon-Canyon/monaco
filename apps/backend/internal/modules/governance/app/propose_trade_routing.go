package app

import (
	"context"
	"encoding/json"
	"log/slog"
	"math/big"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/governance/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/market"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/multichain"
)

const shareNanos = 1_000_000_000

func (h *ProposeTradeHandler) best(
	ctx context.Context, asset market.Asset, trade domain.Trade,
) (*multichain.Comparison, error) {
	if !asset.Tradable() {
		return nil, errs.New(errs.CodeAssetUntradable, "governance.best", slog.String("symbol", asset.Symbol))
	}
	var stock multichain.StockID
	var c multichain.Comparison
	err := stock.UnmarshalText([]byte(asset.ID.String()))
	if err == nil {
		c, err = multichain.BestOn(ctx, h.ports.Router, stock, multichain.SideBuy, trade.USDCMicros, trade.Network)
	}
	if err != nil {
		return nil, err
	}
	return &c, nil
}

func displayQuote(asset market.Asset, c *multichain.Comparison, routed uint64, now time.Time) uint64 {
	m := asset.UIMultiplierAt(now)
	if c == nil || m.Num <= 0 || m.Den <= 0 {
		return routed
	}
	var shares multichain.ShareNanos
	for _, q := range c.Ranked {
		if q.ListingID == c.Winner {
			shares = q.NetShares
		}
	}
	units := new(big.Int).SetUint64(uint64(shares))
	units.Mul(units, new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(asset.Decimals)), nil))
	units.Mul(units, big.NewInt(m.Den))
	units.Quo(units, new(big.Int).Mul(big.NewInt(shareNanos), big.NewInt(m.Num)))
	if !units.IsUint64() {
		return routed
	}
	return units.Uint64()
}

func pinned(c *multichain.Comparison, n chain.Network) (*multichain.Comparison, error) {
	if n == "" {
		return c, nil
	}
	if c != nil {
		if p, ok := c.Pin(n); ok {
			return &p, nil
		}
	}
	return nil, errs.New(errs.CodeInvalidNetwork, "governance.pinned", slog.String("network", string(n)))
}

func encodeComparison(c *multichain.Comparison) ([]byte, error) {
	if c == nil {
		return nil, nil
	}
	raw, err := json.Marshal(c)
	if err != nil {
		return nil, errs.Wrap(err, errs.CodeInternal, "governance.encodeComparison")
	}
	return raw, nil
}

func decodeComparison(raw []byte) (*multichain.Comparison, error) {
	var c *multichain.Comparison
	if len(raw) > 0 {
		c = new(multichain.Comparison)
		if err := json.Unmarshal(raw, c); err != nil {
			return c, errs.Wrap(err, errs.CodeDecodeFailed, "governance.decodeComparison")
		}
	}
	return c, nil
}
