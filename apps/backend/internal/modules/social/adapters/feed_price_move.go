package adapters

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/domain/feed"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
)

func (h Feed) PriceMoved(ctx context.Context, tx db.Tx, e events.AssetPriceMoved, at time.Time) error {
	id := eventID(ctx)
	payload := feed.Payload{
		Symbol: e.Symbol, AssetName: e.AssetName, ThresholdBps: e.ThresholdBps, ChangeBps: e.ChangeBps,
		MarkMicros: e.MarkMicros, PrevClose: e.PrevCloseMicros, TradingDay: e.TradingDay,
	}
	if err := sqlc.New(tx.Queries()).InsertFeedConsumerItem(ctx, sqlc.InsertFeedConsumerItemParams{
		ID: id, Kind: string(feed.KindPriceMove), RefType: string(feed.RefAssetPriceMoves), RefID: id,
		AssetID: pgtype.UUID{Bytes: e.AssetID, Valid: true}, Symbol: pgtype.Text{String: e.Symbol, Valid: true},
		Title: feed.RenderTitle(feed.KindPriceMove, payload), Payload: payload.JSON(), At: at,
	}); err != nil {
		return err
	}
	tx.AfterCommit(func(ctx context.Context) { h.Bus.PublishHint(ctx, "global.feed", nil) })
	return nil
}
