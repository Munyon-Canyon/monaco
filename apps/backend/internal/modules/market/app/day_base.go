package app

import (
	"context"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/market/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/market/sqlc"
)

type dayBaseReader interface {
	NewestSamples(context.Context, sqlc.NewestSamplesParams) ([]sqlc.NewestSamplesRow, error)
	FirstSamplesSince(context.Context, sqlc.FirstSamplesSinceParams) ([]sqlc.FirstSamplesSinceRow, error)
}

func closedSamples(
	ctx context.Context, read dayBaseReader, mints []string, session domain.SessionInfo,
) (map[string][]domain.Sample, error) {
	rows, err := read.NewestSamples(ctx, sqlc.NewestSamplesParams{Mints: mints, At: session.LastClose})
	if err != nil {
		return nil, errs.Wrap(err, errs.CodeOf(err), "market.NewestSamples")
	}
	return groupNewest(rows)
}

func openedSamples(
	ctx context.Context, read dayBaseReader, mints []string, now time.Time,
) (map[string][]domain.Sample, time.Time, error) {
	since := now.UTC().Truncate(24 * time.Hour)
	rows, err := read.FirstSamplesSince(ctx, sqlc.FirstSamplesSinceParams{Mints: mints, Since: since, Until: now})
	if err != nil {
		return nil, since, errs.Wrap(err, errs.CodeOf(err), "market.FirstSamplesSince")
	}
	opened, err := groupSamples(rows)
	return opened, since, err
}
