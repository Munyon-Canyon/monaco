package funding_test

import (
	"maps"
	"testing"

	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/funding"
	"github.com/monaco/monaco/apps/backend/internal/modules/funding/app"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
	"github.com/monaco/monaco/apps/backend/internal/testkit/fakes"
)

func pausedCabalsGauge(t *testing.T, pauses funding.Pauses) (map[string]int64, error) {
	t.Helper()
	reader := sdkmetric.NewManualReader()
	app.ObservePausedCabals(sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader)).Meter("test"), pauses)
	var rm metricdata.ResourceMetrics
	err := reader.Collect(t.Context(), &rm)
	got := map[string]int64{}
	for _, scope := range rm.ScopeMetrics {
		for _, m := range scope.Metrics {
			if g, ok := m.Data.(metricdata.Gauge[int64]); ok && m.Name == "monaco_paused_cabals" {
				for _, p := range g.DataPoints {
					reason, _ := p.Attributes.Value("reason")
					got[reason.AsString()] = p.Value
				}
			}
		}
	}
	return got, err
}

func TestPausedCabalsGauge_CountsCabalsPerReasonAndAGlobalPauseOnce(t *testing.T) {
	t.Parallel()
	env := newPauseEnv(t)
	first, second := testkit.NewCabal(t, env.pool), testkit.NewCabal(t, env.pool)
	m := pausesOf(env)
	got, err := pausedCabalsGauge(t, m.Pauses())
	if want := map[string]int64{"external_deposit": 0, "ops": 0, "ops_global": 0}; err != nil ||
		!maps.Equal(got, want) {
		t.Fatalf("gauge with nothing paused = %v, %v, want %v", got, err, want)
	}
	mustPause(t, env, first.ID, funding.PauseReasonExternalDeposit)
	mustPause(t, env, first.ID, funding.PauseReasonOps)
	mustPause(t, env, second.ID, funding.PauseReasonOps)
	mustPause(t, env, second.ID, funding.PauseReasonOps)
	if _, err := m.PauseFromOps(opsContext(t), nil, "maintenance"); err != nil {
		t.Fatal(err)
	}
	got, err = pausedCabalsGauge(t, m.Pauses())
	if want := map[string]int64{"external_deposit": 1, "ops": 2, "ops_global": 1}; err != nil ||
		!maps.Equal(got, want) {
		t.Fatalf("gauge = %v, %v, want %v", got, err, want)
	}
}

func TestPausedCabalsGauge_ReportsNothingWhenThePausesCannotBeRead(t *testing.T) {
	t.Parallel()
	pauses := fakes.NewPauses()
	pauses.Fail("PausedCabals", errs.New(errs.CodeDBUnavailable, "test"))
	got, err := pausedCabalsGauge(t, pauses)
	if err == nil || len(got) != 0 {
		t.Fatalf("gauge = %v, %v, want no reading and a scrape error", got, err)
	}
}
