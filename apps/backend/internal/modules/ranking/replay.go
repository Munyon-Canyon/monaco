package ranking

import (
	"context"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/market"
	"github.com/monaco/monaco/apps/backend/internal/modules/ranking/app"
	"github.com/monaco/monaco/apps/backend/internal/platform/bus"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
)

func (m *Module) Projection(source module.Set) []bus.HandlerSpec {
	for _, mod := range source {
		if src, ok := mod.(*Module); ok {
			ports := src.ports
			ports.Previous = m.ports.Previous
			return []bus.HandlerSpec{m.replaySnapshot(ports)}
		}
	}
	return nil
}

func (m *Module) replaySnapshot(ports app.Ports) bus.HandlerSpec {
	usdc := chain.SolanaAddress(m.deps.Config.Solana.USDCMint)
	return bus.HandleFetched(
		"ranking.replay.snapshot",
		func(ctx context.Context, e events.RankingSnapshotWritten) (app.Valuation, error) {
			pinned := ports
			pinned.Market = pinnedMarket{Market: ports.Market, at: e.PricesAsOf}
			return app.NewRunValuation(pinned, usdc).Run(ctx, e.AsOf)
		},
		func(ctx context.Context, tx db.Tx, e events.RankingSnapshotWritten, v app.Valuation, _ time.Time) error {
			return app.ReplaySnapshot(ctx, tx, e, v)
		},
	)
}

type pinnedMarket struct {
	app.Market
	at time.Time
}

func (p pinnedMarket) LatestPrices(ctx context.Context) (map[market.AssetID]market.Price, error) {
	assets, err := p.ListAll(ctx)
	if err != nil {
		return nil, err
	}
	assetIDs := make([]market.AssetID, len(assets))
	for i, asset := range assets {
		assetIDs[i] = asset.ID
	}
	return p.PricesAsOf(ctx, assetIDs, p.at)
}
