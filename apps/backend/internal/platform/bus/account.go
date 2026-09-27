package bus

import (
	"context"
	"math"

	"github.com/nats-io/nats.go/jetstream"
	"go.opentelemetry.io/otel/metric"

	"github.com/monaco/monaco/apps/backend/internal/errs"
)

type accountGauge struct {
	name, unit, desc string
	read             func(*jetstream.AccountInfo) int64
	gauge            metric.Int64ObservableGauge
}

func accountGauges() []accountGauge {
	return []accountGauge{
		{
			name: "monaco_bus_account_storage_bytes", unit: "By", desc: "JetStream file storage the account uses.",
			read: func(a *jetstream.AccountInfo) int64 { return int64(min(a.Store, math.MaxInt64)) },
		},
		{
			name: "monaco_bus_account_storage_limit_bytes", unit: "By",
			desc: "JetStream file storage limit of the account; -1 is unlimited.",
			read: func(a *jetstream.AccountInfo) int64 { return a.Limits.MaxStore },
		},
		{
			name: "monaco_bus_account_streams", unit: "{stream}", desc: "Streams in the account.",
			read: func(a *jetstream.AccountInfo) int64 { return int64(a.Streams) },
		},
		{
			name: "monaco_bus_account_consumers", unit: "{consumer}", desc: "Consumers in the account.",
			read: func(a *jetstream.AccountInfo) int64 { return int64(a.Consumers) },
		},
	}
}

func (c *Conn) ExportAccountGauges() (func() error, error) {
	const op = "bus.ExportAccountGauges"
	gauges := accountGauges()
	observables := make([]metric.Observable, 0, len(gauges))
	for i, g := range gauges {
		inst, err := c.meter.Int64ObservableGauge(g.name, metric.WithUnit(g.unit), metric.WithDescription(g.desc))
		if err != nil {
			return nil, errs.Wrap(err, errs.CodeInternal, op)
		}
		gauges[i].gauge = inst
		observables = append(observables, inst)
	}
	reg, err := c.meter.RegisterCallback(func(ctx context.Context, o metric.Observer) error {
		info, err := c.js.AccountInfo(ctx)
		if err != nil {
			return errs.Wrap(err, errs.CodeUpstreamUnavailable, op)
		}
		for _, g := range gauges {
			o.ObserveInt64(g.gauge, g.read(info))
		}
		return nil
	}, observables...)
	if err != nil {
		return nil, errs.Wrap(err, errs.CodeInternal, op)
	}
	return reg.Unregister, nil
}
