package app

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/market/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/market/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

type PriceBook struct {
	q     *sqlc.Queries
	clock clock.Clock
}

func NewPriceBook(db sqlc.DBTX, c clock.Clock) *PriceBook {
	return &PriceBook{q: sqlc.New(db), clock: c}
}

func (b *PriceBook) LatestPrices(ctx context.Context) (map[domain.AssetID]domain.Sample, error) {
	return b.accepted(ctx, "market.PriceBook.LatestPrices",
		sqlc.RecentPriceSamplesParams{At: b.clock.Now(), EveryAsset: true})
}

func (b *PriceBook) PricesAsOf(
	ctx context.Context, ids []domain.AssetID, at time.Time,
) (map[domain.AssetID]domain.Sample, error) {
	asked := make([]uuid.UUID, len(ids))
	for i, id := range ids {
		asked[i] = id.UUID()
	}
	return b.accepted(ctx, "market.PriceBook.PricesAsOf", sqlc.RecentPriceSamplesParams{At: at, Ids: asked})
}

func (b *PriceBook) accepted(
	ctx context.Context, op string, arg sqlc.RecentPriceSamplesParams,
) (map[domain.AssetID]domain.Sample, error) {
	arg.PerAsset = domain.HoldWindow
	rows, err := b.q.RecentPriceSamples(ctx, arg)
	if err != nil {
		return nil, errs.Wrap(err, errs.CodeOf(err), op)
	}
	recent := map[domain.AssetID][]domain.Sample{}
	for _, row := range rows {
		id, s, err := sampleOf(row)
		if err != nil {
			return nil, err
		}
		recent[id] = append(recent[id], s)
	}
	out := make(map[domain.AssetID]domain.Sample, len(recent))
	for id, newestFirst := range recent {
		if s, ok := domain.Accept(newestFirst); ok {
			out[id] = s
		}
	}
	return out, nil
}

func sampleOf(row sqlc.RecentPriceSamplesRow) (domain.AssetID, domain.Sample, error) {
	id, idErr := domain.ParseAssetID(row.AssetID.String())
	micros, microsErr := money.SignedMicrosFromInt64(row.PriceMicros).Micros()
	if err := errors.Join(idErr, microsErr); err != nil {
		return domain.AssetID{}, domain.Sample{}, errs.Wrap(err, errs.CodeDecodeFailed, "market.sampleOf",
			slog.String("asset_id", row.AssetID.String()), slog.Int64("price_micros", row.PriceMicros))
	}
	return id, domain.Sample{Micros: micros, ObservedAt: row.Ts}, nil
}
