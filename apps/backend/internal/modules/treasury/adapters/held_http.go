package adapters

import (
	"context"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/port"
	api "github.com/monaco/monaco/apps/backend/internal/platform/httpx/api/treasuryapi"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

func (h HTTP) GetMyHeld(
	ctx context.Context, req api.GetMyHeldRequestObject,
) (api.GetMyHeldResponseObject, error) {
	user, err := caller(ctx)
	if err != nil {
		return nil, err
	}
	query := app.HeldQuery{Viewer: user}
	if req.Params.Symbol != nil {
		query.Symbol = *req.Params.Symbol
	}
	list, err := h.Held.Handle(ctx, query)
	if err != nil {
		return nil, err
	}
	return wireHeld(list)
}

func wireHeld(list port.HeldList) (api.GetMyHeldResponseObject, error) {
	var w int64Wire
	out := api.HeldList{
		TotalValueMicros: w.micros(list.TotalValueMicros), CabalCount: list.CabalCount,
		PricesAsOf: list.PricesAsOf, Holdings: make([]api.HeldRow, len(list.Holdings)),
	}
	for i, row := range list.Holdings {
		out.Holdings[i] = api.HeldRow{
			Cabal: api.HeldCabal{
				Id: row.Cabal.ID.UUID(), Name: row.Cabal.Name, PictureUrl: optionalText(row.Cabal.PictureURL),
			},
			LogoUrl: optionalText(row.LogoURL),
			Holding: wireHolding(&w, row.Holding),
		}
	}
	if w.err != nil {
		return nil, errs.Wrap(w.err, errs.CodeOf(w.err), "treasury.GetMyHeld")
	}
	return api.GetMyHeld200JSONResponse(out), nil
}

func optionalText(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func wireHolding(w *int64Wire, h port.Holding) api.CabalHolding {
	out := api.CabalHolding{
		Symbol: h.Symbol, DisplayName: h.DisplayName, Kind: api.CabalHoldingKind(h.Kind), Units: h.Units,
		TokenAmount: w.micros(money.MicrosFromUint64(h.TokenAmount.Uint64())),
		PriceMicros: w.micros(h.PriceMicros), ValueMicros: w.micros(h.ValueMicros), WeightBps: h.WeightBps,
		CostBasisMicros: w.micros(h.CostBasisMicros), PnlMicros: h.PnLMicros.Int64(),
		FirstBoughtAt: h.FirstBoughtAt, Listings: wireListings(w, h.Listings),
	}
	if h.AvgEntryMicros != nil {
		avg := h.AvgEntryMicros.String()
		out.AvgEntryMicros = &avg
	}
	return out
}

func (h HTTP) GetHoldingPerformance(
	ctx context.Context, req api.GetHoldingPerformanceRequestObject,
) (api.GetHoldingPerformanceResponseObject, error) {
	user, err := caller(ctx)
	if err != nil {
		return nil, err
	}
	perf, err := h.Performance.Handle(ctx, app.HoldingPerformanceQuery{
		CabalID: ids.CabalIDFrom(req.Id), Caller: user, Symbol: req.Symbol,
	})
	if err != nil {
		return nil, err
	}
	out := api.HoldingPerformance{RealizedPnlMicros: perf.RealizedPnLMicros.Int64()}
	if perf.AvgSellMicros != nil {
		avg := perf.AvgSellMicros.String()
		out.AvgSellMicros = &avg
	}
	return api.GetHoldingPerformance200JSONResponse(out), nil
}
