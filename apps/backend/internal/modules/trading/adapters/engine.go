package adapters

import (
	"context"
	"slices"

	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/trading/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/trading/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/auth"
	"github.com/monaco/monaco/apps/backend/internal/platform/bus"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

type Engine struct {
	Trades *app.ExecuteTradeHandler
}

func (e Engine) Handle(ctx context.Context, d bus.Delivery, ev events.ProposalPassed) error {
	action := domain.Action(ev.Kind)
	if !slices.Contains(domain.Actions(), action) {
		return nil
	}
	ctx = auth.WithActor(ctx, auth.Actor{Kind: auth.ActorSystem, ID: "trading.engine"})
	return e.Trades.Handle(ctx, d, app.ExecuteTrade{
		ProposalID: ids.ProposalIDFrom(ev.ProposalID), CabalID: ids.CabalIDFrom(ev.CabalID), Action: action,
		Symbol: ev.Symbol, Mint: ev.Mint, USDCMicros: ev.USDCMicros, TokenAmount: ev.TokenAmount,
		QuoteOutAmount: ev.QuoteOutAmount,
	}, bus.Heartbeat(ctx))
}
