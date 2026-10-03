package app

import (
	"context"
	"log/slog"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/market/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/market/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
)

func (p *SamplePrices) recordMoves(ctx context.Context, assets []domain.Asset, bucket time.Time) error {
	marks, err := p.book.LatestPrices(ctx)
	if err != nil {
		return err
	}
	for _, a := range assets {
		mark, ok := marks[a.ID]
		if !ok || !mark.ObservedAt.Equal(bucket) {
			continue
		}
		ref, day, ok, err := p.reference(ctx, a, bucket)
		if err != nil {
			return err
		}
		if !ok {
			continue
		}
		for _, th := range domain.Crossed(ref.Micros, mark.Micros) {
			if err := p.recordMove(ctx, a, th, ref, mark, day); err != nil {
				return err
			}
		}
	}
	return nil
}

func (p *SamplePrices) reference(
	ctx context.Context, a domain.Asset, at time.Time,
) (domain.Sample, domain.Date, bool, error) {
	info, err := domain.Session(a.Kind, at)
	if err != nil {
		return domain.Sample{}, domain.Date{}, false, err
	}
	if a.Kind == domain.KindPreIPO {
		start := time.Date(info.TradingDay.Year, info.TradingDay.Month, info.TradingDay.Day, 0, 0, 0, 0, time.UTC)
		samples, err := p.book.DaySamples(ctx, a.ID, start, at)
		if err != nil {
			return domain.Sample{}, domain.Date{}, false, err
		}
		s, ok := domain.FirstAcceptedOnDay(samples, start)
		return s, info.TradingDay, ok, nil
	}
	if info.State != domain.StateOpen {
		return domain.Sample{}, info.TradingDay, false, nil
	}
	prices, err := p.book.PricesAsOf(ctx, []domain.AssetID{a.ID}, info.LastClose)
	if err != nil {
		return domain.Sample{}, domain.Date{}, false, err
	}
	s, ok := prices[a.ID]
	return s, info.TradingDay, ok, nil
}

func (p *SamplePrices) recordMove(
	ctx context.Context, a domain.Asset, th domain.ThresholdBps, ref, mark domain.Sample, day domain.Date,
) error {
	eventID := p.ids.NewV7()
	change := domain.ChangeBps(ref.Micros, mark.Micros)
	err := p.uow.Do(ctx, func(ctx context.Context, tx db.Tx) error {
		n, err := sqlc.New(tx.Queries()).InsertAssetPriceMove(ctx, sqlc.InsertAssetPriceMoveParams{
			AssetID: a.ID.UUID(), ThresholdBps: int64(th), TradingDay: day.String(),
			EventID: eventID, CreatedAt: p.clock.Now(),
		})
		if err != nil {
			return err
		}
		if n == 0 {
			return nil
		}
		moved := events.AssetPriceMoved{
			V: 1, AssetID: a.ID.UUID(), Symbol: a.Symbol, AssetName: domain.DisplayName(a.Issuer, a.DisplayName),
			ThresholdBps: int64(th), ChangeBps: change, MarkMicros: mark.Micros, PrevCloseMicros: ref.Micros,
			TradingDay: day.String(), ObservedAt: mark.ObservedAt,
		}
		if err := tx.Events.Append(ctx, moved); err != nil {
			return err
		}
		tx.AfterCommit(func(ctx context.Context) {
			observability.Info(ctx, observability.MarketPriceMoved,
				slog.String("asset", a.Symbol),
				slog.Int64("threshold", int64(th)),
				slog.Int64("change", change),
			)
		})
		return nil
	})
	if err != nil {
		return errs.Wrap(err, errs.CodeOf(err), "market.SamplePrices.recordMove",
			slog.String("symbol", a.Symbol), slog.Int64("threshold", int64(th)))
	}
	return nil
}
