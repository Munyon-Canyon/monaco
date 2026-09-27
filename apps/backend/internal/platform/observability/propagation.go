package observability

import (
	"context"
	"maps"
	"slices"

	"github.com/nats-io/nats.go"
	"go.opentelemetry.io/otel/propagation"
)

type natsCarrier nats.Header

func (c natsCarrier) Get(key string) string { return nats.Header(c).Get(key) }

func (c natsCarrier) Set(key, value string) { nats.Header(c).Set(key, value) }

func (c natsCarrier) Keys() []string { return slices.Sorted(maps.Keys(c)) }

func Inject(ctx context.Context, h nats.Header) {
	propagation.TraceContext{}.Inject(ctx, natsCarrier(h))
}

func Extract(ctx context.Context, h nats.Header) context.Context {
	return propagation.TraceContext{}.Extract(ctx, natsCarrier(h))
}
