package observability

import (
	"slices"
	"testing"

	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

func TestInjectThenExtract_preservesTraceAndSpanIDs(t *testing.T) {
	t.Parallel()
	traceID, err := trace.TraceIDFromHex("4bf92f3577b34da6a3ce929d0e0e4736")
	if err != nil {
		t.Fatal(err)
	}
	spanID, err := trace.SpanIDFromHex("00f067aa0ba902b7")
	if err != nil {
		t.Fatal(err)
	}
	sent := trace.NewSpanContext(
		trace.SpanContextConfig{TraceID: traceID, SpanID: spanID, TraceFlags: trace.FlagsSampled},
	)
	h := propagation.HeaderCarrier{}
	Inject(trace.ContextWithSpanContext(t.Context(), sent), h)

	if got := h.Get("traceparent"); got != "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01" {
		t.Fatalf("traceparent header = %q", got)
	}
	got := trace.SpanContextFromContext(Extract(t.Context(), h))
	if got.TraceID() != traceID || got.SpanID() != spanID || !got.IsSampled() || !got.IsRemote() {
		t.Fatalf("extracted %+v, want trace %s span %s sampled and remote", got, traceID, spanID)
	}
	if keys := h.Keys(); !slices.Equal(keys, []string{"Traceparent"}) {
		t.Fatalf("carrier keys = %v, want [traceparent]", keys)
	}
}

func TestExtract_withoutHeadersYieldsNoSpan(t *testing.T) {
	t.Parallel()
	if sc := trace.SpanContextFromContext(Extract(t.Context(), propagation.HeaderCarrier{})); sc.IsValid() {
		t.Fatalf("extracted %+v from empty headers, want invalid span context", sc)
	}
}
