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

func (p *SamplePrices) recordMoves(
	ctx context.Context, assets []domain.Asset, bucket time.Time, marks map[domain.AssetID]domain.Sample,
) error {
	moving, err := marked(assets, bucket, marks)
	if err != nil {
		return err
	}
	if err := p.loadCloses(ctx, moving); err != nil {
		return err
	}
	for _, m := range moving {
		a, mark := m.asset, marks[m.asset.ID]
		ref, day, ok, err := p.reference(ctx, m, bucket)
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

func marked(
	assets []domain.Asset, bucket time.Time, marks map[domain.AssetID]domain.Sample,
) ([]movingAsset, error) {
	var moving []movingAsset
	for _, a := range assets {
		if mark, ok := marks[a.ID]; ok && mark.ObservedAt.Equal(bucket) {
			info, err := domain.Session(a.Kind, bucket)
			if err != nil {
				return nil, err
			}
			moving = append(moving, movingAsset{asset: a, info: info})
		}
	}
	return moving, nil
}

type movingAsset struct {
	asset domain.Asset
	info  domain.SessionInfo
}

func (p *SamplePrices) reference(
	ctx context.Context, m movingAsset, at time.Time,
) (domain.Sample, domain.Date, bool, error) {
	a, info := m.asset, m.info
	if a.Kind == domain.KindPreIPO {
		s, ok, err := p.dayOpen(ctx, a.ID, info.TradingDay, at)
		return s, info.TradingDay, ok, err
	}
	if info.State != domain.StateOpen {
		return domain.Sample{}, info.TradingDay, false, nil
	}
	s, ok := p.closes[info.LastClose].prices[a.ID]
	return s, info.TradingDay, ok, nil
}

func (p *SamplePrices) dayOpen(
	ctx context.Context, id domain.AssetID, day domain.Date, at time.Time,
) (domain.Sample, bool, error) {
	if open, ok := p.opens[id]; ok && open.day == day {
		return open.s, true, nil
	}
	start := time.Date(day.Year, day.Month, day.Day, 0, 0, 0, 0, time.UTC)
	samples, err := p.book.DaySamples(ctx, id, start, at)
	if err != nil {
		return domain.Sample{}, false, err
	}
	s, ok := domain.FirstAcceptedOnDay(samples, start)
	if ok {
		if p.opens == nil {
			p.opens = map[domain.AssetID]dayOpen{}
		}
		p.opens[id] = dayOpen{day: day, s: s}
	}
	return s, ok, nil
}

type dayOpen struct {
	day domain.Date
	s   domain.Sample
}

type closeRefs struct {
	asked  map[domain.AssetID]bool
	prices map[domain.AssetID]domain.Sample
}

func (p *SamplePrices) loadCloses(ctx context.Context, assets []movingAsset) error {
	missing := p.unreadCloses(assets)
	for lastClose, ids := range missing {
		prices, err := p.book.PricesAsOf(ctx, ids, lastClose)
		if err != nil {
			return err
		}
		refs := p.closes[lastClose]
		for _, id := range ids {
			refs.asked[id] = true
			if s, ok := prices[id]; ok {
				refs.prices[id] = s
			}
		}
	}
	return nil
}

func (p *SamplePrices) unreadCloses(assets []movingAsset) map[time.Time][]domain.AssetID {
	inUse := map[time.Time]closeRefs{}
	missing := map[time.Time][]domain.AssetID{}
	for _, m := range assets {
		a, info := m.asset, m.info
		if a.Kind == domain.KindPreIPO || info.State != domain.StateOpen {
			continue
		}
		refs, ok := inUse[info.LastClose]
		if !ok {
			refs, ok = p.closes[info.LastClose]
			if !ok {
				refs = closeRefs{asked: map[domain.AssetID]bool{}, prices: map[domain.AssetID]domain.Sample{}}
			}
			inUse[info.LastClose] = refs
		}
		if !refs.asked[a.ID] {
			missing[info.LastClose] = append(missing[info.LastClose], a.ID)
		}
	}
	p.closes = inUse
	return missing
}

func (p *SamplePrices) recordMove(
	ctx context.Context, a domain.Asset, th domain.ThresholdBps, ref, mark domain.Sample, day domain.Date,
) error {
	eventID := p.ids.NewV7()
	change := domain.ChangeBps(ref.Micros, mark.Micros)
	volume, err := p.volume.Volume24h(ctx, a.Mint.String(), p.clock.Now())
	if err != nil {
		return errs.Wrap(err, errs.CodeOf(err), "market.SamplePrices.recordMove", slog.String("symbol", a.Symbol))
	}
	err = p.uow.Do(ctx, func(ctx context.Context, tx db.Tx) error {
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
			V: 1, AssetID: a.ID.UUID(), Symbol: a.Symbol,
			AssetName:    domain.DisplayName(a.Issuer, a.DisplayName, a.Symbol),
			ThresholdBps: int64(th), ChangeBps: change, MarkMicros: mark.Micros, PrevCloseMicros: ref.Micros,
			TradingDay: day.String(), ObservedAt: mark.ObservedAt, Volume24hMicros: volume,
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
