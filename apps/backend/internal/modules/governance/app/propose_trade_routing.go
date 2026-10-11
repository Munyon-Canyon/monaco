package app

import (
	"context"
	"encoding/json"
	"math/big"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/governance/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/market"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/multichain"
)

const shareNanos = 1_000_000_000

func compare(
	ctx context.Context, router multichain.Router, asset market.Asset, trade domain.Trade,
) *multichain.Comparison {
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
