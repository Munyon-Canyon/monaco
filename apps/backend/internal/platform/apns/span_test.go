package apns_test

import (
	"fmt"
	"net/http"
	"strings"
	"testing"

	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"

	"github.com/monaco/monaco/apps/backend/internal/platform/apns"
)

func apnsSpans(rec *tracetest.SpanRecorder) []sdktrace.ReadOnlySpan {
	var out []sdktrace.ReadOnlySpan
	for _, s := range rec.Ended() {
		if s.Name() == "apns Send" {
			out = append(out, s)
		}
	}
	return out
}

func spanAttrs(s sdktrace.ReadOnlySpan) map[string]string {
	out := map[string]string{}
	for _, a := range s.Attributes() {
		out[string(a.Key)] = a.Value.String()
	}
	return out
}

func TestSend_recordsOneClientSpanPerPushWithoutTheToken(t *testing.T) {
	t.Parallel()
	rec := tracetest.NewSpanRecorder()
	ctx, parent := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(rec)).
		Tracer("test").
		Start(t.Context(), "parent")
	u := &upstream{replies: []reply{
		accepted(), rejected(http.StatusInternalServerError, "InternalServerError"), unreachable,
	}}
	c := newClient(t, u)
	for range 3 {
		_, _ = c.Send(ctx, push(apns.Production))
	}
	parent.End()

	spans := apnsSpans(rec)
	if len(spans) != 3 {
		t.Fatalf("recorded %d apns spans, want one per push", len(spans))
	}
	want := []struct {
		status string
		failed bool
	}{{"200", false}, {"500", true}, {"", true}}
	for i, s := range spans {
		attrs := spanAttrs(s)
		if s.SpanKind() != trace.SpanKindClient || s.Parent().SpanID() != parent.SpanContext().SpanID() {
			t.Fatalf("span %d kind %v parent %v, want a client span under the caller's", i, s.SpanKind(), s.Parent())
		}
		if attrs["upstream"] != "apns" || attrs["apns.environment"] != "production" ||
			attrs["http.response.status_code"] != want[i].status || (s.Status().Code == codes.Error) != want[i].failed {
			t.Fatalf("span %d attributes %v status %v, want %+v", i, attrs, s.Status().Code, want[i])
		}
		if strings.Contains(fmt.Sprint(attrs), testToken) {
			t.Fatalf("span %d attributes %v carry the device token", i, attrs)
		}
	}
}
