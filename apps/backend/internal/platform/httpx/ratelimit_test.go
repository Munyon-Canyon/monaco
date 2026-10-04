package httpx_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"go.opentelemetry.io/otel/metric/noop"
	tracenoop "go.opentelemetry.io/otel/trace/noop"

	openapi "github.com/monaco/monaco/apps/backend/api"
	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/platform/auth"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/config"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpx"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpx/api"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpx/api/platformapi"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpx/ratelimit"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpx/sse"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

type noVerifier struct{}

func (noVerifier) Verify(context.Context, string) (auth.Actor, error) {
	return auth.Actor{}, errs.New(errs.CodeUnauthorized, "test.noVerifier")
}

func plant(t *testing.T, anchor, extension string) []byte {
	t.Helper()
	if n := bytes.Count(openapi.Spec, []byte(anchor)); n != 1 {
		t.Fatalf("anchor %q appears %d times in api/openapi.yaml, want 1", anchor, n)
	}
	return bytes.Replace(openapi.Spec, []byte(anchor), []byte(anchor+extension), 1)
}

func TestHandler_ratelimitRefusesTheOverLimitRequest(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	spec := plant(t, "      operationId: getHealthz\n", "      x-rate-limit: {ip: {rate: 1, per: 1m, burst: 1}}\n")
	policies, err := ratelimit.Load(spec)
	if err != nil {
		t.Fatal(err)
	}
	limiter, err := ratelimit.New(pool, testkit.NewClock(time.Now()), noop.NewMeterProvider())
	if err != nil {
		t.Fatal(err)
	}
	h, err := httpx.Handler(httpx.Deps{
		Logger:       observability.NewLogger(config.Config{Env: config.EnvTest}, io.Discard),
		Tracer:       tracenoop.NewTracerProvider(),
		Clock:        clock.Real{},
		IDs:          ids.Real{},
		MaxBodyBytes: 1 << 20,
		Idempotency:  db.NewIdempotencyStore(pool, clock.Real{}),
		Verifier:     noVerifier{},
		RateLimit:    ratelimit.Middleware(limiter, policies, httpx.ActorKey, false),
	}, func(m api.Mount) {
		platformapi.Mount(struct {
			httpx.Health
			sse.Stream
		}{}, m)
	}, spec)
	if err != nil {
		t.Fatal(err)
	}
	get := func() *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		testkit.HTTP(t, h).ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/healthz", nil))
		return rec
	}
	if rec := get(); rec.Code != http.StatusOK {
		t.Fatalf("first GET /healthz = %d %s, want 200", rec.Code, rec.Body)
	}
	rec := get()
	var p api.Problem
	if err := json.Unmarshal(rec.Body.Bytes(), &p); err != nil {
		t.Fatalf("second GET /healthz body %q: %v", rec.Body, err)
	}
	if rec.Code != http.StatusTooManyRequests || p.Code != api.ErrorCode(errs.CodeRateLimited) ||
		rec.Header().Get("Retry-After") != "60" {
		t.Fatalf("second GET /healthz = %d %s Retry-After %q, want 429 rate_limited with Retry-After 60",
			rec.Code, rec.Body, rec.Header().Get("Retry-After"))
	}
}
