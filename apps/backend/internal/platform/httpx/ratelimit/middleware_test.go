package ratelimit_test

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.opentelemetry.io/otel/attribute"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/platform/db/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpx/api"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpx/ratelimit"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

const actorHeader = "X-Test-Actor"

type harness struct {
	handler http.Handler
	reader  *sdkmetric.ManualReader
	logs    *testkit.Logs
}

func headerActor(r *http.Request) (string, bool) {
	id := r.Header.Get(actorHeader)
	return id, id != ""
}

func newHarness(t *testing.T, db sqlc.DBTX, extension string, trustProxyHeaders bool) harness {
	t.Helper()
	reader := sdkmetric.NewManualReader()
	l, err := ratelimit.New(db, testkit.NewClock(epoch()), sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader)))
	if err != nil {
		t.Fatal(err)
	}
	policies, err := ratelimit.Load(spec(authed, thing("post", "[]", extension)+
		"  /v1/others:\n    get:\n      operationId: getOther\n      security: []\n"))
	if err != nil {
		t.Fatal(err)
	}
	limited := ratelimit.Middleware(l, policies, headerActor, trustProxyHeaders)
	done := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) })
	mux := http.NewServeMux()
	mux.Handle("POST /v1/things", limited(done))
	mux.Handle("GET /v1/others", limited(done))
	return harness{handler: mux, reader: reader, logs: &testkit.Logs{}}
}

type call struct {
	method, path, remote, actor string
	forwarded                   []string
}

func (h harness) do(t *testing.T, c call) *httptest.ResponseRecorder {
	t.Helper()
	logger := slog.New(slog.NewJSONHandler(h.logs, &slog.HandlerOptions{Level: slog.LevelDebug}))
	req := httptest.NewRequestWithContext(observability.WithLogger(t.Context(), logger),
		cmpOr(c.method, http.MethodPost), cmpOr(c.path, "/v1/things"), nil)
	req.RemoteAddr = cmpOr(c.remote, "192.0.2.1:4711")
	for _, f := range c.forwarded {
		req.Header.Add("X-Forwarded-For", f)
	}
	if c.actor != "" {
		req.Header.Set(actorHeader, c.actor)
	}
	rec := httptest.NewRecorder()
	h.handler.ServeHTTP(rec, req)
	return rec
}

func cmpOr(v, fallback string) string {
	if v == "" {
		return fallback
	}
	return v
}

func (h harness) statuses(t *testing.T, calls ...call) []int {
	t.Helper()
	out := make([]int, len(calls))
	for i, c := range calls {
		out[i] = h.do(t, c).Code
	}
	return out
}

func (h harness) counter(t *testing.T, name string, attrs ...attribute.KeyValue) int64 {
	t.Helper()
	var rm metricdata.ResourceMetrics
	if err := h.reader.Collect(t.Context(), &rm); err != nil {
		t.Fatal(err)
	}
	want := attribute.NewSet(attrs...)
	var total int64
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			sum, ok := m.Data.(metricdata.Sum[int64])
			if !ok || m.Name != name {
				continue
			}
			for _, p := range sum.DataPoints {
				if p.Attributes.Equals(&want) {
					total += p.Value
				}
			}
		}
	}
	return total
}

func (h harness) lines(t *testing.T, msg string) []map[string]any {
	t.Helper()
	var out []map[string]any
	for _, raw := range bytes.Split(bytes.TrimSpace(h.logs.Bytes()), []byte("\n")) {
		var line map[string]any
		if len(raw) == 0 {
			continue
		}
		if err := json.Unmarshal(raw, &line); err != nil {
			t.Fatalf("log line %q: %v", raw, err)
		}
		if line["msg"] == msg {
			out = append(out, line)
		}
	}
	return out
}

func keys(t *testing.T, pool *pgxpool.Pool) []string {
	t.Helper()
	rows, err := pool.Query(t.Context(), `SELECT key FROM rate_limit_buckets ORDER BY key`)
	if err != nil {
		t.Fatal(err)
	}
	out, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func TestMiddleware_Refused(t *testing.T) {
	t.Parallel()
	h := newHarness(t, testkit.DB(t), "{ip: {rate: 1, per: 1m, burst: 1}}", false)
	if got := h.do(t, call{}).Code; got != http.StatusNoContent {
		t.Fatalf("first call = %d, want 204", got)
	}
	rec := h.do(t, call{})
	var p api.Problem
	if err := json.Unmarshal(rec.Body.Bytes(), &p); err != nil {
		t.Fatalf("429 body %q: %v", rec.Body.Bytes(), err)
	}
	t.Logf("429 Retry-After: %s\n%s", rec.Header().Get("Retry-After"), rec.Body.Bytes())
	if rec.Code != http.StatusTooManyRequests || rec.Header().Get("Content-Type") != "application/problem+json" ||
		rec.Header().Get("Retry-After") != "60" {
		t.Fatalf("second call = %d %v, want 429 problem+json with Retry-After 60", rec.Code, rec.Header())
	}
	if p.Code != "rate_limited" || p.Status != http.StatusTooManyRequests || !p.Retryable ||
		p.Message != errs.Message(errs.CodeRateLimited) {
		t.Fatalf("problem = %+v, want rate_limited 429 retryable", p)
	}
	rejected := h.counter(t, "monaco_ratelimit_rejected_total",
		attribute.String("operation", "postThing"), attribute.String("scope", "ip"))
	if rejected != 1 {
		t.Fatalf("monaco_ratelimit_rejected_total{postThing,ip} = %d, want 1", rejected)
	}
}

func TestMiddleware_RetryAfterRoundsUpToWholeSeconds(t *testing.T) {
	t.Parallel()
	h := newHarness(t, testkit.DB(t), "{ip: {rate: 3, per: 1s, burst: 1}}", false)
	h.do(t, call{})
	if got := h.do(t, call{}).Header().Get("Retry-After"); got != "1" {
		t.Fatalf("Retry-After for a 333ms wait = %q, want 1", got)
	}
}

func TestMiddleware_TrustProxyHeaders(t *testing.T) {
	t.Parallel()
	const ip = "{ip: {rate: 1, per: 1m, burst: 1}}"
	t.Run("enabled keys on the rightmost entry", func(t *testing.T) {
		t.Parallel()
		pool := testkit.DB(t)
		h := newHarness(t, pool, ip, true)
		got := h.statuses(t,
			call{forwarded: []string{"198.51.100.7, 203.0.113.9"}},
			call{forwarded: []string{"198.51.100.8, 203.0.113.9"}},
			call{forwarded: []string{"198.51.100.7", "203.0.113.10"}},
			call{forwarded: []string{" "}},
			call{remote: "192.0.2.1:9999"},
		)
		if want := []int{204, 429, 204, 204, 429}; !slices.Equal(got, want) {
			t.Fatalf("statuses = %v, want %v", got, want)
		}
		want := []string{"op:postThing:ip:192.0.2.1", "op:postThing:ip:203.0.113.10", "op:postThing:ip:203.0.113.9"}
		if got := keys(t, pool); !slices.Equal(got, want) {
			t.Fatalf("bucket keys = %v, want %v", got, want)
		}
	})
	t.Run("disabled keys on the socket address", func(t *testing.T) {
		t.Parallel()
		pool := testkit.DB(t)
		h := newHarness(t, pool, ip, false)
		got := h.statuses(t,
			call{forwarded: []string{"203.0.113.9"}},
			call{forwarded: []string{"203.0.113.10"}, remote: "192.0.2.1:9999"},
			call{remote: "192.0.2.2"},
		)
		if want := []int{204, 429, 204}; !slices.Equal(got, want) {
			t.Fatalf("statuses = %v, want %v", got, want)
		}
		want := []string{"op:postThing:ip:192.0.2.1", "op:postThing:ip:192.0.2.2"}
		if got := keys(t, pool); !slices.Equal(got, want) {
			t.Fatalf("bucket keys = %v, want %v", got, want)
		}
	})
}

func TestMiddleware_ActorScopeKeysOnTheActorAndSkipsAnonymousCallers(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	h := newHarness(t, pool, "{actor: {rate: 1, per: 1m, burst: 1}, ip: {rate: 3, per: 1m, burst: 3}}", false)
	got := h.statuses(t, call{actor: "u1"}, call{actor: "u1"}, call{actor: "u2"}, call{}, call{}, call{actor: "u3"})
	if want := []int{204, 429, 204, 204, 429, 429}; !slices.Equal(got, want) {
		t.Fatalf("statuses = %v, want %v", got, want)
	}
	want := []string{
		"op:postThing:actor:u1", "op:postThing:actor:u2", "op:postThing:actor:u3", "op:postThing:ip:192.0.2.1",
	}
	if got := keys(t, pool); !slices.Equal(got, want) {
		t.Fatalf("bucket keys = %v, want %v", got, want)
	}
	for scope, n := range map[string]int64{"actor": 1, "ip": 2} {
		got := h.counter(t, "monaco_ratelimit_rejected_total",
			attribute.String("operation", "postThing"), attribute.String("scope", scope))
		if got != n {
			t.Errorf("monaco_ratelimit_rejected_total{postThing,%s} = %d, want %d", scope, got, n)
		}
	}
}

func TestMiddleware_StoreDownFailsOpen(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	pool.Close()
	h := newHarness(t, pool, "{actor: {rate: 1, per: 1m, burst: 1}, ip: {rate: 1, per: 1m, burst: 1}}", false)
	if got := h.do(t, call{actor: "u1"}).Code; got != http.StatusNoContent {
		t.Fatalf("call with the store down = %d, want 204", got)
	}
	lines := h.lines(t, "ratelimit.store_failed")
	if len(lines) != 1 || lines[0]["level"] != "WARN" || lines[0]["code"] != "db_unavailable" ||
		lines[0]["operation"] != "postThing" || lines[0]["scope"] != "actor" {
		t.Fatalf("store_failed lines = %v, want one WARN with code db_unavailable", lines)
	}
	if got := h.counter(t, "monaco_ratelimit_errors_total"); got != 1 {
		t.Fatalf("monaco_ratelimit_errors_total = %d, want 1", got)
	}
}

func TestMiddleware_RoutesWithoutAPolicyNeverTouchTheStore(t *testing.T) {
	t.Parallel()
	h := newHarness(t, failingDB{take: errs.New(errs.CodeInternal, "test.down")},
		"{ip: {rate: 1, per: 1m, burst: 1}}", false)
	for range 3 {
		if got := h.do(t, call{method: http.MethodGet, path: "/v1/others"}).Code; got != http.StatusNoContent {
			t.Fatalf("GET /v1/others = %d, want 204", got)
		}
	}
	if lines := h.lines(t, "ratelimit.store_failed"); len(lines) != 0 {
		t.Fatalf("store_failed lines = %v, want none", lines)
	}
}

func TestNew_failsWhenACounterCannotBeCreated(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"monaco_ratelimit_rejected_total", "monaco_ratelimit_errors_total"} {
		l, err := ratelimit.New(nil, testkit.NewClock(epoch()), testkit.FailingGauges{Prefix: name})
		if l != nil || errs.CodeOf(err) != errs.CodeInternal {
			t.Fatalf("New with %s failing = %v, %v, want internal", name, l, err)
		}
	}
}
