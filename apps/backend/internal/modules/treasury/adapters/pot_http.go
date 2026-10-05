package adapters

import (
	"context"
	"log/slog"
	"math"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/port"
	api "github.com/monaco/monaco/apps/backend/internal/platform/httpx/api/treasuryapi"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

func (h HTTP) GetCabalPot(
	ctx context.Context, req api.GetCabalPotRequestObject,
) (api.GetCabalPotResponseObject, error) {
	const op = "treasury.GetCabalPot"
	user, err := caller(ctx)
	if err != nil {
		return nil, err
	}
	cabalID := ids.CabalIDFrom(req.Id)
	found, err := h.Cabals.Cabals(ctx, []ids.CabalID{cabalID})
	if err != nil {
		return nil, errs.Wrap(err, errs.CodeOf(err), op)
	}
	if _, ok := found[cabalID]; !ok {
		return nil, errs.New(errs.CodeCabalNotFound, op)
	}
	member, err := h.Members.IsMember(ctx, cabalID, user)
	if err != nil {
		return nil, errs.Wrap(err, errs.CodeOf(err), op)
	}
	pot, err := h.Pot.CabalPot(ctx, cabalID, user, member)
	if err != nil {
		return nil, err
	}
	return wirePot(pot)
}

func wirePot(p port.CabalPot) (api.GetCabalPotResponseObject, error) {
	var w int64Wire
	out := api.CabalPot{
		CabalId: p.CabalID.UUID(), PotValueMicros: w.micros(p.PotValueMicros), CashMicros: w.micros(p.CashMicros),
		CashWeightBps: p.CashWeightBps, PnlMicros: p.PnLMicros.Int64(), ReturnBps: p.ReturnBps,
		PricesAsOf: p.PricesAsOf, Holdings: make([]api.CabalHolding, len(p.Holdings)),
	}
	for i, h := range p.Holdings {
		out.Holdings[i] = api.CabalHolding{
			Symbol: h.Symbol, DisplayName: h.DisplayName, Units: h.Units,
			PriceMicros: w.micros(h.PriceMicros), ValueMicros: w.micros(h.ValueMicros), WeightBps: h.WeightBps,
			CostBasisMicros: w.micros(h.CostBasisMicros), PnlMicros: h.PnLMicros.Int64(),
		}
	}
	if p.Me != nil {
		out.Me = &api.CabalPotSlice{
			ShareUnits: w.micros(
				money.MicrosFromUint64(p.Me.ShareUnits.Uint64()),
			),
			ValueMicros:          w.micros(p.Me.ValueMicros),
			SliceBps:             p.Me.SliceBps,
			NetContributedMicros: p.Me.NetContributedMicros.Int64(),
			PnlMicros:            p.Me.PnLMicros.Int64(),
		}
	}
	if w.err != nil {
		return nil, w.err
	}
	return api.GetCabalPot200JSONResponse(out), nil
}

type int64Wire struct{ err error }

func (w *int64Wire) micros(m money.Micros) int64 {
	v := m.Uint64()
	if v > math.MaxInt64 {
		w.err = errs.New(errs.CodeInvalidInput, "treasury.wirePot", slog.Uint64("micros", v))
		return 0
	}
	return int64(v)
}
