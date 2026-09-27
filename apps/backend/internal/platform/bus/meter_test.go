package bus_test

import (
	"context"
	"testing"

	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/metric/noop"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/platform/bus"
	"github.com/monaco/monaco/apps/backend/internal/platform/config"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

type failingMeters struct {
	noop.MeterProvider
	fail string
}

func (p failingMeters) Meter(string, ...metric.MeterOption) metric.Meter {
	return failingMeter{fail: p.fail}
}

type failingMeter struct {
	noop.Meter
	fail string
}

func (m failingMeter) Int64Counter(name string, opts ...metric.Int64CounterOption) (metric.Int64Counter, error) {
	if m.fail == "counter" {
		return nil, errs.New(errs.CodeInternal, "test.Int64Counter")
	}
	return m.Meter.Int64Counter(name, opts...)
}

func (m failingMeter) Float64Histogram(
	name string, opts ...metric.Float64HistogramOption,
) (metric.Float64Histogram, error) {
	if m.fail == "histogram" {
		return nil, errs.New(errs.CodeInternal, "test.Float64Histogram")
	}
	return m.Meter.Float64Histogram(name, opts...)
}

func (m failingMeter) Int64ObservableGauge(
	name string, opts ...metric.Int64ObservableGaugeOption,
) (metric.Int64ObservableGauge, error) {
	if m.fail == "gauge" || m.fail == "gauge:"+name {
		return nil, errs.New(errs.CodeInternal, "test.Int64ObservableGauge")
	}
	return m.Meter.Int64ObservableGauge(name, opts...)
}

func (m failingMeter) RegisterCallback(cb metric.Callback, obs ...metric.Observable) (metric.Registration, error) {
	if m.fail == "callback" {
		return nil, errs.New(errs.CodeInternal, "test.RegisterCallback")
	}
	return m.Meter.RegisterCallback(cb, obs...)
}

func TestConnect_failsWhenTheDropCounterCannotBeCreated(t *testing.T) {
	t.Parallel()
	_, err := bus.Connect(t.Context(), config.NATS{URL: testkit.NATSURL()}, bus.ProcessAPI,
		bus.WithMeterProvider(failingMeters{fail: "counter"}))
	if errs.CodeOf(err) != errs.CodeInternal {
		t.Fatalf("Connect = %v, want internal", err)
	}
}

func TestExportAccountGauges_failsWhenAnInstrumentCannotBeCreated(t *testing.T) {
	t.Parallel()
	for _, fail := range []string{"gauge", "callback"} {
		conn, err := bus.Connect(t.Context(), config.NATS{URL: testkit.NATSURL()}, bus.ProcessWorker,
			bus.WithMeterProvider(failingMeters{fail: fail}))
		if err != nil {
			t.Fatal(err)
		}
		_, err = conn.ExportAccountGauges()
		conn.Close(context.Background())
		if errs.CodeOf(err) != errs.CodeInternal {
			t.Fatalf("ExportAccountGauges with a failing %s = %v, want internal", fail, err)
		}
	}
}
