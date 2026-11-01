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

func (b *PriceBook) DaySamples(
	ctx context.Context, id domain.AssetID, dayStart, until time.Time,
) ([]domain.Sample, error) {
	rows, err := b.q.DayPriceSamples(ctx, sqlc.DayPriceSamplesParams{
		AssetID: id.UUID(), DayStart: dayStart, Lookback: domain.HoldWindow, Until: until,
	})
	if err != nil {
		return nil, errs.Wrap(err, errs.CodeOf(err), "market.PriceBook.DaySamples")
	}
	out := make([]domain.Sample, 0, len(rows))
	for _, row := range rows {
		micros, err := money.SignedMicrosFromInt64(row.PriceMicros).Micros()
		if err != nil {
			return nil, errs.Wrap(err, errs.CodeDecodeFailed, "market.PriceBook.DaySamples",
				slog.String("asset_id", id.String()), slog.Int64("price_micros", row.PriceMicros))
		}
		out = append(out, domain.Sample{Micros: micros, ObservedAt: row.Ts})
	}
	return out, nil
}

func (b *PriceBook) PricesAsOf(
	ctx context.Context, ids []domain.AssetID, at time.Time,
) (map[domain.AssetID]domain.Sample, error) {
	return b.accepted(ctx, "market.PriceBook.PricesAsOf", sqlc.RecentPriceSamplesParams{At: at, Ids: assetUUIDs(ids)})
}

func (b *PriceBook) CurrentPricesAsOf(
	ctx context.Context, ids []domain.AssetID, at time.Time,
) (map[domain.AssetID]domain.Sample, error) {
	recent, err := b.raw(ctx, "market.PriceBook.CurrentPricesAsOf",
		sqlc.RecentPriceSamplesParams{At: at, Ids: assetUUIDs(ids)})
	if err != nil {
		return nil, err
	}
	out := make(map[domain.AssetID]domain.Sample, len(recent))
	for id, newestFirst := range recent {
		if s, ok := domain.AcceptNewest(newestFirst); ok {
			out[id] = s
		}
	}
	return out, nil
}

func (b *PriceBook) Volume24h(ctx context.Context, mint string, now time.Time) (*money.Micros, error) {
	got, err := volumesOf(ctx, b.q, []string{mint}, now)
	if err != nil {
		return nil, err
	}
	var out *money.Micros
	if v, ok := got[mint]; ok {
		out = &v.USD
	}
	return out, nil
}

func (b *PriceBook) PointsCount(ctx context.Context) (int64, error) {
	n, err := b.q.CountPricePoints(ctx)
	if err != nil {
		return 0, errs.Wrap(err, errs.CodeOf(err), "market.PriceBook.PointsCount")
	}
	return n, nil
}

func (b *PriceBook) RecentSamples(ctx context.Context) (map[domain.AssetID][]domain.Sample, error) {
	return b.raw(ctx, "market.PriceBook.RecentSamples",
		sqlc.RecentPriceSamplesParams{At: b.clock.Now(), EveryAsset: true})
}

func (b *PriceBook) SamplesOf(ctx context.Context, ids []domain.AssetID) (map[domain.AssetID][]domain.Sample, error) {
	return b.raw(ctx, "market.PriceBook.SamplesOf",
		sqlc.RecentPriceSamplesParams{At: b.clock.Now(), Ids: assetUUIDs(ids)})
}

func assetUUIDs(ids []domain.AssetID) []uuid.UUID {
	out := make([]uuid.UUID, len(ids))
	for i, id := range ids {
		out[i] = id.UUID()
	}
	return out
}

func (b *PriceBook) accepted(
	ctx context.Context, op string, arg sqlc.RecentPriceSamplesParams,
) (map[domain.AssetID]domain.Sample, error) {
	recent, err := b.raw(ctx, op, arg)
	if err != nil {
		return nil, err
	}
	return acceptAll(recent), nil
}

func acceptAll(recent map[domain.AssetID][]domain.Sample) map[domain.AssetID]domain.Sample {
	out := make(map[domain.AssetID]domain.Sample, len(recent))
	for id, newestFirst := range recent {
		if s, ok := domain.Accept(newestFirst); ok {
			out[id] = s
		}
	}
	return out
}

func (b *PriceBook) raw(
	ctx context.Context, op string, arg sqlc.RecentPriceSamplesParams,
) (map[domain.AssetID][]domain.Sample, error) {
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
	return recent, nil
}

func sampleOf(row sqlc.RecentPriceSamplesRow) (domain.AssetID, domain.Sample, error) {
	id, idErr := domain.ParseAssetID(row.AssetID.String())
	micros, microsErr := money.SignedMicrosFromInt64(row.PriceMicros).Micros()
	if err := errors.Join(idErr, microsErr); err != nil {
		return domain.AssetID{}, domain.Sample{}, errs.Wrap(err, errs.CodeDecodeFailed, "market.sampleOf",
			slog.String("asset_id", row.AssetID.String()), slog.Int64("price_micros", row.PriceMicros))
	}
	return id, domain.Sample{Micros: micros, ObservedAt: row.Ts, Source: domain.Source(row.Source)}, nil
}
