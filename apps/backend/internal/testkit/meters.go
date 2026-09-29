package testkit

import (
	"strings"

	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/metric/noop"

	"github.com/monaco/monaco/apps/backend/internal/errs"
)

type FailingGauges struct {
	noop.MeterProvider
	Prefix string
}

func (p FailingGauges) Meter(string, ...metric.MeterOption) metric.Meter {
	return failingGaugeMeter{prefix: p.Prefix}
}

type failingGaugeMeter struct {
	noop.Meter
	prefix string
}

func (m failingGaugeMeter) Int64ObservableGauge(
	name string, _ ...metric.Int64ObservableGaugeOption,
) (metric.Int64ObservableGauge, error) {
	if strings.HasPrefix(name, m.prefix) {
		return nil, errs.New(errs.CodeInternal, "testkit.FailingGauges")
	}
	return noop.Int64ObservableGauge{}, nil
}

func (m failingGaugeMeter) Int64UpDownCounter(
	name string, _ ...metric.Int64UpDownCounterOption,
) (metric.Int64UpDownCounter, error) {
	if strings.HasPrefix(name, m.prefix) {
		return nil, errs.New(errs.CodeInternal, "testkit.FailingGauges")
	}
	return noop.Int64UpDownCounter{}, nil
}

func (m failingGaugeMeter) Int64Counter(name string, _ ...metric.Int64CounterOption) (metric.Int64Counter, error) {
	if strings.HasPrefix(name, m.prefix) {
		return nil, errs.New(errs.CodeInternal, "testkit.FailingGauges")
	}
	return noop.Int64Counter{}, nil
}
