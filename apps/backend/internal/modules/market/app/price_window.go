package app

import (
	"context"
	"slices"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/modules/market/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/market/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

const windowReseed = time.Hour

func (p *SamplePrices) ensureWindow(ctx context.Context) error {
	now := p.clock.Now()
	points, err := p.book.PointsCount(ctx)
	if err != nil {
		return err
	}
	p.wrote = 0
	stale := !p.lastTick.IsZero() && now.Sub(p.lastTick) > p.interval*3/2
	if p.window != nil && !stale && points == p.points && now.Sub(p.seededAt) < windowReseed {
		p.lastTick = now
		return nil
	}
	window, err := p.book.RecentSamples(ctx)
	if err != nil {
		p.window = nil
		return err
	}
	p.window, p.lastTick, p.seededAt, p.points = window, now, now, points
	return nil
}

func (p *SamplePrices) absorb(
	ctx context.Context, assets []domain.Asset, answered map[domain.Mint]money.Micros,
	asked []string, inserted []sqlc.InsertPricePointsRow,
) error {
	byMint := make(map[string]domain.Asset, len(assets))
	for _, a := range assets {
		byMint[a.Mint.String()] = a
	}
	stored := make(map[string]struct{}, len(inserted))
	for _, row := range inserted {
		stored[row.Mint] = struct{}{}
		if a, ok := byMint[row.Mint]; ok {
			p.window[a.ID] = withSample(p.window[a.ID], domain.Sample{Micros: answered[a.Mint], ObservedAt: row.Ts})
		}
	}
	var conflicted []domain.AssetID
	for _, mint := range asked {
		if _, ok := stored[mint]; ok {
			continue
		}
		if a, ok := byMint[mint]; ok {
			conflicted = append(conflicted, a.ID)
		}
	}
	return p.refresh(ctx, conflicted)
}

func (p *SamplePrices) refresh(ctx context.Context, ids []domain.AssetID) error {
	if len(ids) == 0 {
		return nil
	}
	fresh, err := p.book.SamplesOf(ctx, ids)
	if err != nil {
		return err
	}
	for _, id := range ids {
		p.window[id] = fresh[id]
	}
	return nil
}

func (p *SamplePrices) accepted() map[domain.AssetID]domain.Sample {
	return acceptAll(p.window)
}

func withSample(newestFirst []domain.Sample, s domain.Sample) []domain.Sample {
	out := append(slices.Clone(newestFirst), s)
	slices.SortFunc(out, func(a, b domain.Sample) int { return b.ObservedAt.Compare(a.ObservedAt) })
	return out[:min(len(out), domain.HoldWindow)]
}
