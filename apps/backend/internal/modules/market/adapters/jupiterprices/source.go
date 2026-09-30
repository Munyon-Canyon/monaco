package jupiterprices

import (
	"context"

	"github.com/monaco/monaco/apps/backend/internal/modules/market/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/market/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/jupiter"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

var _ app.PriceSource = Source{}

type Source struct {
	client *jupiter.Client
}

func New(client *jupiter.Client) Source { return Source{client: client} }

func (s Source) Prices(ctx context.Context, mints []domain.Mint) (map[domain.Mint]money.Micros, error) {
	asked := make([]jupiter.Mint, len(mints))
	for i, m := range mints {
		asked[i] = jupiter.Mint{Address: m.String()}
	}
	answered, err := s.client.Prices(ctx, asked)
	out := make(map[domain.Mint]money.Micros, len(answered))
	for i, m := range mints {
		if p, ok := answered[asked[i]]; ok {
			out[m] = p.USDMicros
		}
	}
	return out, err
}
