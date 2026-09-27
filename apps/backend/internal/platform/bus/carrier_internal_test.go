package bus

import (
	"slices"
	"testing"

	"github.com/nats-io/nats.go"
	"go.opentelemetry.io/otel/trace"

	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
)

const w3cParent = "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01"

func TestNATSCarrier_extractsTraceparentInAnyCase(t *testing.T) {
	t.Parallel()
	for _, key := range []string{"traceparent", "Traceparent", "TraceParent", "TRACEPARENT"} {
		h := nats.Header{}
		h[key] = []string{w3cParent}
		sc := trace.SpanContextFromContext(observability.Extract(t.Context(), natsCarrier(h)))
		if !sc.IsValid() || !sc.IsRemote() || sc.TraceID().String() != "4bf92f3577b34da6a3ce929d0e0e4736" {
			t.Fatalf("header %q extracted %+v, want the remote span", key, sc)
		}
	}
}

func TestNATSCarrier_prefersTheLowercaseKey(t *testing.T) {
	t.Parallel()
	h := nats.Header{"Traceparent": {"garbage"}, "traceparent": {w3cParent}}
	if got := natsCarrier(h).Get("traceparent"); got != w3cParent {
		t.Fatalf("Get = %q, want the lowercase entry", got)
	}
	if keys := natsCarrier(h).Keys(); !slices.Equal(keys, []string{"Traceparent", "traceparent"}) {
		t.Fatalf("Keys = %v", keys)
	}
	if got := natsCarrier(nats.Header{}).Get("traceparent"); got != "" {
		t.Fatalf("Get on empty headers = %q", got)
	}
}

func TestNATSCarrier_skipsAnOtherCaseKeyWithNoValue(t *testing.T) {
	t.Parallel()
	if got := natsCarrier(nats.Header{"TRACEPARENT": {}}).Get("traceparent"); got != "" {
		t.Fatalf("Get with only an empty TRACEPARENT entry = %q, want \"\"", got)
	}
}
