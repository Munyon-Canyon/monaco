package adapters

import (
	"context"

	"github.com/monaco/monaco/apps/backend/internal/modules/governance/app"
	api "github.com/monaco/monaco/apps/backend/internal/platform/httpx/api/governanceapi"
)

func (h HTTP) GetMyMarketFacts(
	ctx context.Context, req api.GetMyMarketFactsRequestObject,
) (api.GetMyMarketFactsResponseObject, error) {
	user, err := caller(ctx)
	if err != nil {
		return nil, err
	}
	symbols, err := app.ParseMarketFactSymbols(req.Params.Symbols)
	if err != nil {
		return nil, err
	}
	rows, err := h.Facts.For(ctx, user, symbols)
	if err != nil {
		return nil, err
	}
	out := api.GetMyMarketFacts200JSONResponse{Facts: make([]api.MarketFactRow, len(rows))}
	for i, row := range rows {
		out.Facts[i] = api.MarketFactRow{Symbol: row.Symbol, Fact: wireMarketFact(row.Fact)}
	}
	return out, nil
}

func wireMarketFact(f *app.MarketFact) *api.MarketFact {
	if f == nil {
		return nil
	}
	out := api.MarketFact{Kind: api.MarketFactKind(f.Kind)}
	switch f.Kind {
	case app.MarketFactVoteOpen:
		out.CabalName, out.Count = &f.CabalName, &f.Count
	case app.MarketFactHeld:
		out.Units, out.CabalCount = &f.Units, &f.CabalCount
	case app.MarketFactCalled:
		out.ProposerName = &f.ProposerName
	}
	return &out
}
