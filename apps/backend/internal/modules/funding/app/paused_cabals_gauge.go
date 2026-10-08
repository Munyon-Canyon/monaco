package app

import (
	"context"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/funding/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/funding/port"
)

const reasonOpsGlobal = "ops_global"

func ObservePausedCabals(meter metric.Meter, pauses port.Pauses) {
	_, _ = meter.Int64ObservableGauge("monaco_paused_cabals",
		metric.WithUnit("{cabal}"),
		metric.WithDescription("Cabals paused for each reason, and one for a global ops pause."),
		metric.WithInt64Callback(func(ctx context.Context, o metric.Int64Observer) error {
			set, err := pauses.PausedCabals(ctx)
			if err != nil {
				return errs.Wrap(err, errs.CodeOf(err), "funding.PausedCabals.gauge")
			}
			counts := map[string]int64{
				string(domain.PauseReasonExternalDeposit): 0, string(domain.PauseReasonOps): 0, reasonOpsGlobal: 0,
			}
			if set.Global {
				counts[reasonOpsGlobal] = 1
			}
			for _, reasons := range set.Cabals {
				for _, r := range reasons {
					counts[string(r)]++
				}
			}
			for reason, n := range counts {
				o.Observe(n, metric.WithAttributes(attribute.String("reason", reason)))
			}
			return nil
		}))
}
