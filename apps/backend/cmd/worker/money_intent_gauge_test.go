package main

import (
	"errors"
	"testing"
	"time"

	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

func collectIntentGauge(t *testing.T, reader progressReader) (int64, error) {
	t.Helper()
	manual := sdkmetric.NewManualReader()
	meter := sdkmetric.NewMeterProvider(sdkmetric.WithReader(manual)).Meter("test")
	if err := exportMoneyIntentGauge(meter, reader); err != nil {
		t.Fatal(err)
	}
	var rm metricdata.ResourceMetrics
	if err := manual.Collect(t.Context(), &rm); err != nil {
		return 0, err
	}
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			if gauge, ok := m.Data.(metricdata.Gauge[int64]); ok && m.Name == "monaco_money_intent_oldest_seconds" &&
				len(gauge.DataPoints) == 1 {
				return gauge.DataPoints[0].Value, nil
			}
		}
	}
	return -1, nil
}

func TestMoneyIntentGauge_reportsTheOldestIntentInWholeSeconds(t *testing.T) {
	t.Parallel()
	got, err := collectIntentGauge(t, stubProgress{p: progress{MoneyIntentOldest: 301500 * time.Millisecond}})
	if err != nil || got != 301 {
		t.Fatalf("gauge = %d, %v, want 301", got, err)
	}
}

func TestMoneyIntentGauge_reportsNothingWhenProgressCannotBeRead(t *testing.T) {
	t.Parallel()
	if got, err := collectIntentGauge(t, stubProgress{err: errors.New("db down")}); err == nil {
		t.Fatalf("collect = %d, want the callback error while progress is unreadable", got)
	}
}

func TestMoneyIntentGauge_failsWhenTheMeterCannotCreateIt(t *testing.T) {
	t.Parallel()
	meter := testkit.FailingGauges{Prefix: "monaco_money"}.Meter("test")
	if err := exportMoneyIntentGauge(meter, stubProgress{}); errs.CodeOf(err) != errs.CodeInternal {
		t.Fatalf("exportMoneyIntentGauge = %v, want internal", err)
	}
}
