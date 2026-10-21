package httpx

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/monaco/monaco/apps/backend/internal/platform/httpx/api/platformapi"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

const readyBound = 50 * time.Millisecond

type pinger func(context.Context) error

func (p pinger) Ping(ctx context.Context) error { return p(ctx) }

func exhausted(ctx context.Context) error {
	<-ctx.Done()
	return ctx.Err()
}

func alive() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
}

func probe(h http.Handler, method, path string) (int, string) {
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequestWithContext(context.Background(), method, path, nil))
	return rec.Code, rec.Body.String()
}

func TestReady_answersOKWhileThePoolHandsOutAConnection(t *testing.T) {
	t.Parallel()
	h := Ready(alive(), pinger(func(context.Context) error { return nil }), readyBound)
	if code, body := probe(h, http.MethodGet, "/readyz"); code != http.StatusOK || body != "ok\n" {
		t.Fatalf("GET /readyz = %d %q, want 200 ok", code, body)
	}
}

func TestReady_failsWithinItsBoundWhenThePoolIsExhaustedWhileLivenessStaysUp(t *testing.T) {
	t.Parallel()
	h := Ready(alive(), pinger(exhausted), readyBound)
	code, body := probe(h, http.MethodGet, "/readyz")
	if code != http.StatusServiceUnavailable || body != "db pool exhausted: no connection within 50ms\n" {
		t.Fatalf("GET /readyz = %d %q, want 503 naming the exhausted pool", code, body)
	}
	if code, _ := probe(h, http.MethodGet, "/healthz"); code != http.StatusNoContent {
		t.Fatalf("GET /healthz = %d, want liveness handled by the wrapped handler", code)
	}
}

func TestReady_failsWhenThePingFails(t *testing.T) {
	t.Parallel()
	h := Ready(alive(), pinger(func(context.Context) error { return errors.New("connection refused") }), readyBound)
	if code, body := probe(h, http.MethodGet, "/readyz"); code != http.StatusServiceUnavailable ||
		body != "db connection refused\n" {
		t.Fatalf("GET /readyz = %d %q, want 503 naming the error", code, body)
	}
}

func TestReady_leavesEveryOtherRequestToTheWrappedHandler(t *testing.T) {
	t.Parallel()
	h := Ready(alive(), pinger(exhausted), readyBound)
	for _, c := range []struct{ method, path string }{
		{http.MethodPost, "/readyz"}, {http.MethodGet, "/readyz/x"}, {http.MethodGet, "/v1/me"},
	} {
		if code, _ := probe(h, c.method, c.path); code != http.StatusNoContent {
			t.Errorf("%s %s = %d, want the wrapped handler's 204", c.method, c.path, code)
		}
	}
}

func TestReady_followsARealPoolFromExhaustedToFree(t *testing.T) {
	t.Parallel()
	cfg := testkit.DB(t).Config()
	cfg.MaxConns = 1
	pool, err := pgxpool.NewWithConfig(t.Context(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	if err := pool.Ping(t.Context()); err != nil {
		t.Fatal(err)
	}
	h := Ready(alive(), pool, 200*time.Millisecond)
	if code, body := probe(h, http.MethodGet, "/readyz"); code != http.StatusOK {
		t.Fatalf("idle pool GET /readyz = %d %q, want 200", code, body)
	}
	held, err := pool.Acquire(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if code, body := probe(h, http.MethodGet, "/readyz"); code != http.StatusServiceUnavailable ||
		body != "db pool exhausted: no connection within 200ms\n" {
		t.Fatalf("exhausted pool GET /readyz = %d %q, want 503 exhausted", code, body)
	}
	held.Release()
	if code, body := probe(h, http.MethodGet, "/readyz"); code != http.StatusOK {
		t.Fatalf("freed pool GET /readyz = %d %q, want 200", code, body)
	}
}

func TestHandler_servesReadinessBesideLivenessOnlyWhenDepsAskForIt(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	live := healthz{fn: func(ctx context.Context) (platformapi.GetHealthzResponseObject, error) {
		return Health{}.GetHealthz(ctx, platformapi.GetHealthzRequestObject{})
	}}
	without := mustHandler(t, h.deps, live)
	if rec := serveRaw(t, without, http.MethodGet, "/readyz", nil); rec.Code != http.StatusNotFound {
		t.Fatalf("GET /readyz without Deps.Ready = %d, want 404", rec.Code)
	}
	h.deps.Ready = pinger(func(context.Context) error { return errors.New("pool exhausted") })
	served := mustHandler(t, h.deps, live)
	rec := serveRaw(t, served, http.MethodGet, "/healthz", nil)
	if rec.Code != http.StatusOK || rec.Body.String() != "ok\n" {
		t.Fatalf("GET /healthz with an exhausted pool = %d %q, want 200 ok", rec.Code, rec.Body)
	}
	if rec = serveRaw(t, served, http.MethodGet, "/readyz", nil); rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("GET /readyz with an exhausted pool = %d, want 503", rec.Code)
	}
}
