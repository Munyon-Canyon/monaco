package exports

import (
	"context"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/analytics/app"
)

const proposalSource = "proposal"

func (p Proposals) TradeConfirmed(ctx context.Context, e events.TradeConfirmed) (app.Capture, bool, error) {
	return p.byTradeSource(ctx, "trade_executed", e.Source, e.CabalID, map[string]any{
		"action": e.Action, "symbol": e.Symbol, "usdc_amount": usdc(e.USDCMicros),
	})
}

func (p Proposals) TradeBlocked(ctx context.Context, e events.TradeBlocked) (app.Capture, bool, error) {
	return p.byTradeSource(ctx, "trade_blocked", e.Source, e.CabalID, map[string]any{"code": string(e.Code)})
}

func (p Proposals) TradeFailed(ctx context.Context, e events.TradeFailed) (app.Capture, bool, error) {
	return p.byTradeSource(ctx, "trade_failed", e.Source, e.CabalID, map[string]any{"failure_code": e.FailureCode})
}

func (p Proposals) byTradeSource(
	ctx context.Context, event string, source events.TradeSource, cabal uuid.UUID, extra map[string]any,
) (app.Capture, bool, error) {
	if source.Kind != proposalSource {
		return app.Capture{}, false, nil
	}
	return p.byProposer(ctx, event, source.ID, cabal, extra)
}
