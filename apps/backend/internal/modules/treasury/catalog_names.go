package treasury

import (
	"context"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/modules/market"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/domain"
)

type catalogNames struct {
	Catalog market.Catalog
}

func (c catalogNames) AssetNames(ctx context.Context) (map[domain.Asset]app.AssetName, error) {
	all, err := c.Catalog.ListAll(ctx)
	if err != nil {
		return nil, err
	}
	out := make(map[domain.Asset]app.AssetName, len(all))
	for _, a := range all {
		out[domain.Asset(a.Mint.String())] = app.AssetName{
			Symbol: a.Symbol, Name: a.DisplayName, Decimals: a.Decimals,
			MultiplierAt: func(t time.Time) (int64, int64) {
				m := a.UIMultiplierAt(t)
				return m.Num, m.Den
			},
		}
	}
	return out, nil
}
