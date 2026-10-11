package app

import (
	"context"
	"encoding/json"
	"log/slog"
	"math/big"
	"sync/atomic"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/governance/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/market"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/multichain"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
)

const (
	shareNanos       = 1_000_000_000
	fallbackLogEvery = time.Minute
)

type fallbackLog struct{ last atomic.Int64 }

func (f *fallbackLog) note(ctx context.Context, now time.Time, symbol string, err error) {
	code := errs.CodeOf(err)
	last := f.last.Load()
	if code == errs.CodeNoRoute || now.UnixNano()-last < int64(fallbackLogEvery) ||
		!f.last.CompareAndSwap(last, now.UnixNano()) {
		return
	}
	observability.Degraded(ctx, observability.MarketRouterFallback,
		slog.String("symbol", symbol), slog.String("code", string(code)))
}

func (h *ProposeTradeHandler) compare(
	ctx context.Context,
	asset market.Asset,
	trade domain.Trade,
) *multichain.Comparison {
	router := h.ports.Router
	if router == nil || trade.Kind != domain.KindBuy {
		return nil
	}
	var stock multichain.StockID
	var c multichain.Comparison
	err := stock.UnmarshalText([]byte(asset.ID.String()))
	if err == nil {
		c, err = router.Best(ctx, stock, multichain.SideBuy, trade.USDCMicros)
	}
	if err != nil {
		h.fallbacks.note(ctx, h.clock.Now(), asset.Symbol, err)
		return nil
	}
	return &c
}

func displayQuote(asset market.Asset, c *multichain.Comparison, routed uint64, now time.Time) uint64 {
	m := asset.UIMultiplierAt(now)
	if c == nil || m.Num <= 0 || m.Den <= 0 {
		return routed
	}
	units := new(big.Int).SetUint64(uint64(c.Ranked[0].NetShares))
	units.Mul(units, new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(asset.Decimals)), nil))
	units.Mul(units, big.NewInt(m.Den))
	units.Quo(units, new(big.Int).Mul(big.NewInt(shareNanos), big.NewInt(m.Num)))
	if !units.IsUint64() {
		return routed
	}
	return units.Uint64()
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
