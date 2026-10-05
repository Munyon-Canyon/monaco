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
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
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

func (e Engine) HandleRetry(ctx context.Context, d bus.Delivery, ev events.TradeRetryRequested) error {
	cmd := app.ExecuteTrade{
		ProposalID: ids.ProposalIDFrom(ev.Source.ID), CabalID: ids.CabalIDFrom(ev.CabalID),
		Action: domain.Action(ev.Action), Symbol: ev.Symbol, Mint: ev.OutMint,
		USDCMicros: money.MicrosFromUint64(ev.InAmount), QuoteOutAmount: ev.QuoteOutAmount,
		Retry: &app.Retry{Of: ids.SwapIDFrom(ev.SwapID), SlippageBps: ev.SlippageBps},
	}
	if cmd.Action == domain.ActionSell {
		cmd.Mint, cmd.USDCMicros, cmd.TokenAmount = ev.InMint, money.Micros{}, ev.InAmount
	}
	ctx = auth.WithActor(ctx, auth.Actor{Kind: auth.ActorSystem, ID: "trading.engine"})
	return e.Trades.Handle(ctx, d, cmd, bus.Heartbeat(ctx))
}
