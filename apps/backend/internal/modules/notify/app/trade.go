package app

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/market"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

const (
	opTrade        = "notify.Trade"
	proposalSource = "proposal"
	unnamedAsset   = "a stock"
)

type Assets interface {
	AssetBySymbol(ctx context.Context, symbol string) (market.Asset, error)
}

type TradeFilled struct {
	Cabals Cabals
	Assets Assets
}

func (TradeFilled) Name() string { return "trade_filled" }

func (k TradeFilled) Recipients(ctx context.Context, e events.TradeConfirmed) ([]ids.UserID, error) {
	return tradeRecipients(ctx, k.Cabals, e.Source, e.CabalID)
}

func (k TradeFilled) Render(ctx context.Context, e events.TradeConfirmed, _ ids.UserID) (Message, error) {
	words, err := wordsFor(ctx, k.Assets, e.Action, e.Symbol)
	if err != nil {
		return Message{}, err
	}
	body := fmt.Sprintf("Your cabal %s %s of %s", words.past, usd(e.USDCMicros), words.asset)
	return tradeMessage(k.Name(), e.SwapID, e.CabalID, "Trade filled", body), nil
}

type TradeFailed struct {
	Cabals Cabals
	Assets Assets
}

func (TradeFailed) Name() string { return "trade_failed" }

func (k TradeFailed) Recipients(ctx context.Context, e events.TradeFailed) ([]ids.UserID, error) {
	return tradeRecipients(ctx, k.Cabals, e.Source, e.CabalID)
}

func (k TradeFailed) Render(ctx context.Context, e events.TradeFailed, _ ids.UserID) (Message, error) {
	words, err := wordsFor(ctx, k.Assets, e.Action, e.Symbol)
	if err != nil {
		return Message{}, err
	}
	body := fmt.Sprintf("Your cabal's %s of %s failed. No money moved.", words.noun, words.asset)
	return tradeMessage(k.Name(), e.SwapID, e.CabalID, "Trade didn't go through", body), nil
}

func tradeRecipients(
	ctx context.Context, cabals Cabals, source events.TradeSource, cabalID uuid.UUID,
) ([]ids.UserID, error) {
	if source.Kind != proposalSource {
		return nil, nil
	}
	return memberIDs(ctx, cabals, &cabalID)
}

type tradeWords struct{ past, noun, asset string }

func wordsFor(ctx context.Context, assets Assets, action, symbol string) (tradeWords, error) {
	var words tradeWords
	switch action {
	case "buy":
		words.past, words.noun = "bought", "buy"
	case "sell":
		words.past, words.noun = "sold", "sell"
	default:
		return tradeWords{}, errs.New(errs.CodeInvalidInput, opTrade, slog.String("action", action))
	}
	asset, err := assets.AssetBySymbol(ctx, symbol)
	switch {
	case err == nil:
		words.asset = asset.DisplayName
	case errs.CodeOf(err) == errs.CodeAssetNotFound:
		words.asset = unnamedAsset
	default:
		return tradeWords{}, err
	}
	return words, nil
}

func tradeMessage(kind string, swapID, cabalID uuid.UUID, title, body string) Message {
	return Message{
		Title:      title,
		Body:       body,
		Data:       map[string]string{"kind": kind, "cabal_id": cabalID.String(), "txn_id": swapID.String()},
		CollapseID: "trade-" + swapID.String(),
	}
}
