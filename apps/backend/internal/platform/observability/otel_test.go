package observability

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"go.opentelemetry.io/otel"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/platform/config"
)

type otlpRequest struct {
	auth string
	body string
}

type otlpCollector struct {
	mu       sync.Mutex
	requests map[string][]otlpRequest
}

func (c *otlpCollector) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.requests[r.URL.Path] = append(c.requests[r.URL.Path], otlpRequest{r.Header.Get("Authorization"), string(body)})
}

func TestSetup_exportsTracesMetricsAndLogsToTheEndpoint(t *testing.T) {
	t.Parallel()
	collector := &otlpCollector{requests: map[string][]otlpRequest{}}
	srv := httptest.NewServer(collector)
	defer srv.Close()
	cfg := config.Config{Env: config.EnvProduction, OTel: config.OTel{
		Endpoint: srv.URL + "/", Headers: " Authorization=Basic%20abc , ", ServiceName: "monaco-api",
	}}
	shutdown, err := Setup(t.Context(), cfg)
	if err != nil {
		t.Fatal(err)
	}

	ctx, span := otel.Tracer("observability_test").Start(t.Context(), "fund")
	counter, err := otel.Meter("observability_test").Int64Counter("poller_errors_total")
	if err != nil {
		t.Fatal(err)
	}
	counter.Add(ctx, 1)
	NewLogger(cfg, io.Discard).InfoContext(ctx, "otel.bridge_probe", slog.String("email", "a@b.c"))
	span.End()
	if err := shutdown(t.Context()); err != nil {
		t.Fatal(err)
	}

	collector.mu.Lock()
	defer collector.mu.Unlock()
	for _, path := range []string{"/v1/traces", "/v1/metrics", "/v1/logs"} {
		reqs := collector.requests[path]
		if len(reqs) == 0 {
			t.Errorf("no export to %s", path)
			continue
		}
		if reqs[0].auth != "Basic abc" || !strings.Contains(reqs[0].body, "monaco-api") {
			t.Errorf("%s export auth=%q, service name present=%v", path, reqs[0].auth,
				strings.Contains(reqs[0].body, "monaco-api"))
		}
	}
	var sb strings.Builder
	for _, r := range collector.requests["/v1/logs"] {
		sb.WriteString(r.body)
	}
	logs := sb.String()
	if !strings.Contains(logs, "otel.bridge_probe") || strings.Contains(logs, "a@b.c") {
		t.Errorf("bridged log export: has probe=%v, leaks email=%v",
			strings.Contains(logs, "otel.bridge_probe"), strings.Contains(logs, "a@b.c"))
	}
	if err := shutdown(t.Context()); errs.CodeOf(err) != errs.CodeUpstreamUnavailable {
		t.Fatalf("second shutdown = %v, want upstream_unavailable", err)
	}
}

func TestSetup_withoutEndpointIsNoop(t *testing.T) {
	t.Parallel()
	shutdown, err := Setup(t.Context(), config.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := shutdown(t.Context()); err != nil {
		t.Fatalf("noop shutdown = %v", err)
	}
}

func TestSetup_rejectsMalformedEndpointOrHeaders(t *testing.T) {
	t.Parallel()
	for _, otelCfg := range []config.OTel{
		{Endpoint: "://missing-scheme"},
		{Endpoint: "ftp://collector:4318"},
		{Endpoint: "http://"},
		{Endpoint: "http://collector:4318", Headers: "Authorization"},
		{Endpoint: "http://collector:4318", Headers: "=Basic"},
		{Endpoint: "http://collector:4318", Headers: "Authorization=%zz"},
	} {
		shutdown, err := Setup(t.Context(), config.Config{OTel: otelCfg})
		if shutdown != nil || errs.CodeOf(err) != errs.CodeInvalidInput {
			t.Errorf("Setup(%+v) = %v, want invalid_input and no shutdown", otelCfg, err)
		}
	}
}
