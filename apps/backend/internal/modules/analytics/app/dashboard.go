package app

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/platform/bucket"
	dbsqlc "github.com/monaco/monaco/apps/backend/internal/platform/db/sqlc"
)

const MaxRange = 400 * 24 * time.Hour

type ReadOnly func(ctx context.Context, fn func(ctx context.Context, db dbsqlc.DBTX) error) error

type Window struct {
	From time.Time
	To   time.Time
	Size bucket.Size
}

func ParseWindow(from, to, size string) (Window, error) {
	const op = "analytics.ParseWindow"
	start, startErr := parseInstant(from)
	end, endErr := parseInstant(to)
	bucketSize, sizeErr := bucket.Parse(size)
	if err := errors.Join(startErr, endErr, sizeErr); err != nil {
		return Window{}, errs.Wrap(err, errs.CodeInvalidInput, op)
	}
	if !end.After(start) || end.Sub(start) > MaxRange {
		return Window{}, errs.New(errs.CodeInvalidInput, op, slog.Time("from", start), slog.Time("to", end))
	}
	return Window{From: start, To: end, Size: bucketSize}, nil
}

func parseInstant(raw string) (time.Time, error) {
	for _, layout := range []string{time.RFC3339, time.DateOnly} {
		if at, err := time.Parse(layout, raw); err == nil {
			return at.UTC(), nil
		}
	}
	return time.Time{}, errs.New(errs.CodeInvalidInput, "analytics.parseInstant", slog.String("value", raw))
}
