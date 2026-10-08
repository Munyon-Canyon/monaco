package governance_test

import (
	"context"
	"testing"

	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/governance/app"
)

type openFake struct {
	n   int64
	err error
}

func (f openFake) OpenCount(context.Context) (int64, error) { return f.n, f.err }

func openProposalsGauge(t *testing.T, open app.OpenCounter) (int64, bool, error) {
	t.Helper()
	reader := sdkmetric.NewManualReader()
	app.ObserveOpenProposals(sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader)).Meter("test"), open)
	var rm metricdata.ResourceMetrics
	err := reader.Collect(t.Context(), &rm)
	for _, scope := range rm.ScopeMetrics {
		for _, m := range scope.Metrics {
			if g, ok := m.Data.(metricdata.Gauge[int64]); ok && m.Name == "monaco_open_proposals" &&
				len(g.DataPoints) == 1 {
				return g.DataPoints[0].Value, true, err
			}
		}
	}
	return 0, false, err
}

func TestObserveOpenProposals_reportsTheOpenCountOnEachScrape(t *testing.T) {
	t.Parallel()
	if got, ok, err := openProposalsGauge(t, openFake{n: 7}); err != nil || !ok || got != 7 {
		t.Fatalf("monaco_open_proposals = %d, %v, %v, want 7", got, ok, err)
	}
}

func TestObserveOpenProposals_reportsNothingWhenTheCountFails(t *testing.T) {
	t.Parallel()
	got, ok, err := openProposalsGauge(t, openFake{err: errs.New(errs.CodeDBUnavailable, "test")})
	if ok || err == nil {
		t.Fatalf("monaco_open_proposals = %d, %v, %v, want no reading and a scrape error", got, ok, err)
	}
}
