package app

import (
	"context"
	"log/slog"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/market/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/market/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

type quotes struct {
	newest      map[string][]domain.Sample
	closed      map[string][]domain.Sample
	opened      map[string][]domain.Sample
	openedSince time.Time
	spark       map[string][]domain.Sample
}

func (l *ListAssets) load(
	ctx context.Context, assets []domain.Asset, now time.Time, equitySession domain.SessionInfo,
) (quotes, error) {
	mints := mintsOf(assets)
	newest, err := l.newest(ctx, mints, now)
	if err != nil {
		return quotes{}, err
	}
	equity, preIPO := splitMints(assets)
	closed := map[string][]domain.Sample{}
	if len(equity) > 0 {
		closed, err = l.newest(ctx, equity, equitySession.LastClose)
		if err != nil {
			return quotes{}, err
		}
	}
	opened := map[string][]domain.Sample{}
	openedSince := time.Time{}
	if len(preIPO) > 0 {
		openedSince = now.UTC().Truncate(24 * time.Hour)
		opened, err = l.samplesSince(ctx, preIPO, openedSince, now)
		if err != nil {
			return quotes{}, err
		}
	}
	spark, err := l.sparkline(ctx, mints, now.Add(-24*time.Hour), now)
	if err != nil {
		return quotes{}, err
	}
	return quotes{newest: newest, closed: closed, opened: opened, openedSince: openedSince, spark: spark}, nil
}

func (l *ListAssets) newest(
	ctx context.Context, mints []string, at time.Time,
) (map[string][]domain.Sample, error) {
	rows, err := l.read.NewestSamples(ctx, sqlc.NewestSamplesParams{Mints: mints, At: at})
	if err != nil {
		return nil, errs.Wrap(err, errs.CodeOf(err), "market.NewestSamples")
	}
	return groupNewest(rows)
}

func (l *ListAssets) samplesSince(
	ctx context.Context, mints []string, since, until time.Time,
) (map[string][]domain.Sample, error) {
	rows, err := l.read.FirstSamplesSince(ctx, sqlc.FirstSamplesSinceParams{Mints: mints, Since: since, Until: until})
	if err != nil {
		return nil, errs.Wrap(err, errs.CodeOf(err), "market.FirstSamplesSince")
	}
	return groupSamples(rows)
}

func (l *ListAssets) sparkline(
	ctx context.Context, mints []string, since, until time.Time,
) (map[string][]domain.Sample, error) {
	rows, err := l.read.SparklineCloses(ctx, sqlc.SparklineClosesParams{Mints: mints, Since: since, Until: until})
	if err != nil {
		return nil, errs.Wrap(err, errs.CodeOf(err), "market.SparklineCloses")
	}
	return groupSpark(rows)
}

func summary(asset domain.Asset, session domain.SessionInfo, book quotes) Summary {
	out := Summary{Asset: asset, Session: session}
	newest := book.newest[asset.Mint.String()]
	accepted, ok := domain.Accept(newest)
	if !ok {
		return out
	}
	out.Priced = true
	out.Price = accepted
	if ref, known := reference(asset, book); known {
		if bps, measured := domain.QuoteBps(accepted.Micros, ref); measured {
			out.Change = &bps
		}
	}
	out.Sparkline = domain.LastSparkline(book.spark[asset.Mint.String()], newest)
	return out
}

func reference(asset domain.Asset, book quotes) (money.Micros, bool) {
	if asset.Kind == domain.KindPreIPO {
		sample, ok := domain.FirstAcceptedOnDay(book.opened[asset.Mint.String()], book.openedSince)
		return sample.Micros, ok
	}
	accepted, ok := domain.Accept(book.closed[asset.Mint.String()])
	return accepted.Micros, ok
}

func mintsOf(assets []domain.Asset) []string {
	out := make([]string, len(assets))
	for i, asset := range assets {
		out[i] = asset.Mint.String()
	}
	return out
}

func splitMints(assets []domain.Asset) (equity, preIPO []string) {
	for _, asset := range assets {
		if asset.Kind == domain.KindPreIPO {
			preIPO = append(preIPO, asset.Mint.String())
			continue
		}
		equity = append(equity, asset.Mint.String())
	}
	return equity, preIPO
}

func groupNewest(rows []sqlc.NewestSamplesRow) (map[string][]domain.Sample, error) {
	out := make(map[string][]domain.Sample, len(rows))
	for _, row := range rows {
		sample, err := priceSample(row.Ts, row.PriceMicros)
		if err != nil {
			return nil, err
		}
		out[row.Mint] = append(out[row.Mint], sample)
	}
	return out, nil
}

func groupSamples(rows []sqlc.FirstSamplesSinceRow) (map[string][]domain.Sample, error) {
	out := make(map[string][]domain.Sample, len(rows))
	for _, row := range rows {
		sample, err := priceSample(row.Ts, row.PriceMicros)
		if err != nil {
			return nil, err
		}
		out[row.Mint] = append(out[row.Mint], sample)
	}
	return out, nil
}

func groupSpark(rows []sqlc.SparklineClosesRow) (map[string][]domain.Sample, error) {
	out := make(map[string][]domain.Sample, len(rows))
	for _, row := range rows {
		sample, err := priceSample(row.Bucket, row.CloseMicros)
		if err != nil {
			return nil, err
		}
		out[row.Mint] = append(out[row.Mint], sample)
	}
	return out, nil
}

func priceSample(at time.Time, micros int64) (domain.Sample, error) {
	m, err := money.SignedMicrosFromInt64(micros).Micros()
	if err != nil {
		return domain.Sample{}, errs.Wrap(err, errs.CodeDecodeFailed, "market.priceSample",
			slog.Int64("price_micros", micros))
	}
	return domain.Sample{Micros: m, ObservedAt: at}, nil
}
