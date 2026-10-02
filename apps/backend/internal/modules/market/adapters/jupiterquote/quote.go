package jupiterquote

import (
	"context"

	"github.com/monaco/monaco/apps/backend/internal/modules/market/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/market/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/jupiter"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

type Source struct {
	client *jupiter.Client
}

func New(client *jupiter.Client) Source { return Source{client: client} }

func (s Source) Quote(ctx context.Context, in, out domain.Mint, amount money.BaseUnits) (app.Quote, error) {
	q, err := s.client.Quote(ctx, jupiter.QuoteSpec{
		In:     jupiter.Mint{Address: in.String(), Decimals: amount.Decimals()},
		Out:    jupiter.Mint{Address: out.String(), Decimals: amount.Decimals()},
		Amount: amount,
	})
	if err != nil {
		return app.Quote{}, err
	}
	return app.Quote{
		InAmount: q.InAmount, OutAmount: q.OutAmount, PriceImpactBps: q.PriceImpactBps, Routable: q.Routable,
	}, nil
}
