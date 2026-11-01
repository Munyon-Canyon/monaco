package app

import (
	"context"

	"go.opentelemetry.io/otel/metric"

	"github.com/monaco/monaco/apps/backend/internal/errs"
)

func ObserveMoneyIntents(meter metric.Meter, q Queues) {
	_, _ = meter.Int64ObservableGauge(
		"monaco_oldest_unresolved_money_intent_seconds",
		metric.WithUnit("s"),
		metric.WithDescription(
			"Age of the oldest withdrawal, fund transfer or cash-out job that has not reached a final state.",
		),
		metric.WithInt64Callback(func(ctx context.Context, o metric.Int64Observer) error {
			age, err := q.OldestMoneyIntent(ctx)
			if err != nil {
				return errs.Wrap(err, errs.CodeOf(err), "admin.ObserveMoneyIntents")
			}
			o.Observe(int64(age.Seconds()))
			return nil
		}),
	)
}
