package observability

import (
	"context"
	"log/slog"

	"go.opentelemetry.io/otel/trace"

	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

type fieldsKey struct{}

type fields struct {
	requestID string
	eventID   string
	actor     string
	module    string
	op        string
	consumer  string
	delivery  uint64
}

func WithRequestID(ctx context.Context, id string) context.Context {
	return with(ctx, func(f *fields) { f.requestID = id })
}

func WithEventID(ctx context.Context, id ids.EventID) context.Context {
	return with(ctx, func(f *fields) { f.eventID = id.String() })
}

func EventIDFrom(ctx context.Context) string {
	f, _ := ctx.Value(fieldsKey{}).(fields)
	return f.eventID
}

func WithConsumer(ctx context.Context, consumer string, delivery uint64) context.Context {
	return with(ctx, func(f *fields) { f.consumer, f.delivery = consumer, delivery })
}

func ConsumerFrom(ctx context.Context) string {
	f, _ := ctx.Value(fieldsKey{}).(fields)
	return f.consumer
}

func WithActor(ctx context.Context, actor string) context.Context {
	return with(ctx, func(f *fields) { f.actor = actor })
}

func ActorFrom(ctx context.Context) string {
	f, _ := ctx.Value(fieldsKey{}).(fields)
	return f.actor
}

func WithModule(ctx context.Context, module string) context.Context {
	return with(ctx, func(f *fields) { f.module = module })
}

func WithOp(ctx context.Context, op string) context.Context {
	return with(ctx, func(f *fields) { f.op = op })
}

func with(ctx context.Context, set func(*fields)) context.Context {
	f, _ := ctx.Value(fieldsKey{}).(fields)
	set(&f)
	return context.WithValue(ctx, fieldsKey{}, f)
}

func joinKeys(ctx context.Context) []slog.Attr {
	var out []slog.Attr
	if sc := trace.SpanContextFromContext(ctx); sc.IsValid() {
		out = append(out, slog.String("trace_id", sc.TraceID().String()), slog.String("span_id", sc.SpanID().String()))
	}
	f, _ := ctx.Value(fieldsKey{}).(fields)
	for _, kv := range [...]struct{ key, value string }{
		{"request_id", f.requestID},
		{"event_id", f.eventID},
		{"actor", f.actor},
		{"module", f.module},
		{"op", f.op},
		{"consumer", f.consumer},
	} {
		if kv.value != "" {
			out = append(out, slog.String(kv.key, kv.value))
		}
	}
	if f.consumer != "" {
		out = append(out, slog.Uint64("delivery", f.delivery))
	}
	return out
}
