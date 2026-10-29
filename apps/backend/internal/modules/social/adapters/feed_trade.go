package adapters

import (
	"context"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/domain/feed"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

func (h Feed) FetchTrade(ctx context.Context, e events.TradeConfirmed) (app.AssetCard, error) {
	if e.Source.Kind != proposalSource {
		return app.AssetCard{}, nil
	}
	mint, _ := tradeLeg(e)
	return h.Assets.AssetByMint(ctx, string(mint))
}

func (h Feed) ApplyTrade(
	ctx context.Context, tx db.Tx, e events.TradeConfirmed, asset app.AssetCard, at time.Time,
) error {
	if e.Source.Kind != proposalSource {
		return nil
	}
	_, tokens := tradeLeg(e)
	fill := feed.Fill{
		TokenAmount: money.NewBaseUnits(tokens, asset.Decimals), USDCMicros: e.USDCMicros, FilledAt: e.ConfirmedAt,
	}
	return h.recordSwap(ctx, tx, e.Source.ID, func(p *feed.Payload) error {
		return p.RecordFill(e.SwapID, fill, e.SourceBatchSize)
	}, at)
}

func tradeLeg(e events.TradeConfirmed) (chain.SolanaAddress, uint64) {
	if e.Action == string(feed.ActionSell) {
		return e.InMint, e.InAmount
	}
	return e.OutMint, e.OutAmount
}
