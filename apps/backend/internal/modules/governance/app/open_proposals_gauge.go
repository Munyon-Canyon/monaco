package app

import (
	"context"

	"go.opentelemetry.io/otel/metric"

	"github.com/monaco/monaco/apps/backend/internal/errs"
)

type OpenCounter interface {
	OpenCount(ctx context.Context) (int64, error)
}

func ObserveOpenProposals(meter metric.Meter, open OpenCounter) {
	_, _ = meter.Int64ObservableGauge("monaco_open_proposals",
		metric.WithUnit("{proposal}"),
		metric.WithDescription("Proposals open for voting."),
		metric.WithInt64Callback(func(ctx context.Context, o metric.Int64Observer) error {
			n, err := open.OpenCount(ctx)
			if err != nil {
				return errs.Wrap(err, errs.CodeDBUnavailable, "governance.OpenProposals.gauge")
			}
			o.Observe(n)
			return nil
		}))
}
