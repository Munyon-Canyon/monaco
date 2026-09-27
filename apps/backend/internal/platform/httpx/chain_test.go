package httpx

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/platform/db"
)

const chainSpec = `openapi: 3.1.0
info: {title: fixture, version: "1"}
security:
  - bearerAuth: []
components:
  securitySchemes:
    bearerAuth: {type: http, scheme: bearer}
paths:
  /v1/things:
    post:
      operationId: postThing
      requestBody:
        required: true
        content:
          application/json:
            schema:
              type: object
              additionalProperties: false
              required: [amount]
              properties:
                amount: {type: integer, minimum: 1}
      responses:
        "201": {description: created}
`

type countingStore struct {
	IdempotencyStore
	begins atomic.Int32
}

func (s *countingStore) Begin(ctx context.Context, actor, key string, hash []byte) (db.Claim, error) {
	s.begins.Add(1)
	return s.IdempotencyStore.Begin(ctx, actor, key, hash)
}

func chained(t *testing.T, h *harness) (http.Handler, *countingHandler, *countingStore) {
	t.Helper()
	c, err := loadContract([]byte(chainSpec))
	if err != nil {
		t.Fatal(err)
	}
	store := &countingStore{IdempotencyStore: realStore(t, h)}
	h.deps.Idempotency = store
	next := &countingHandler{serve: createsThing}
	var handler http.Handler = next
	for _, mw := range middlewares(h.deps, c) {
		handler = mw(handler)
	}
	mux := http.NewServeMux()
	mux.Handle("POST /v1/things", handler)
	return h.deps.wrap(mux), next, store
}

func postChained(t *testing.T, handler http.Handler, token, key, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/v1/things", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	if key != "" {
		req.Header.Set(IdempotencyKeyHeader, key)
	}
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

func TestMiddlewares_refusalsHappenInChainOrderWithoutAClaim(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	now := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	v := devVerifier(t, "k1", now)
	h.deps.Verifier = v
	handler, next, store := chained(t, h)
	alice := v.Mint("alice", now.Add(time.Hour))

	if rec := postChained(t, handler, "", "k1", `{"amount":5}`); rec.Code != http.StatusUnauthorized ||
		store.begins.Load() != 0 {
		t.Fatalf("no token = %d after %d Begin calls, want 401 and no claim", rec.Code, store.begins.Load())
	}
	if rec := postChained(t, handler, "", "k1", `{"amount":0}`); rec.Code != http.StatusUnauthorized ||
		store.begins.Load() != 0 {
		t.Fatalf("no token and invalid body = %d, want 401: Auth answers before validation", rec.Code)
	}
	if rec := postChained(t, handler, alice, "k1", `{"amount":0}`); rec.Code != http.StatusBadRequest ||
		store.begins.Load() != 0 {
		t.Fatalf("invalid body = %d after %d Begin calls, want 400 and no claim", rec.Code, store.begins.Load())
	}
	if rec := postChained(t, handler, alice, "", `{"amount":5}`); rec.Code != http.StatusBadRequest ||
		next.calls.Load() != 0 {
		t.Fatalf("no key = %d after %d handler calls, want 400 and no run", rec.Code, next.calls.Load())
	}
}

func TestMiddlewares_authRunsBeforeIdempotencySoActorsDoNotShareKeys(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	now := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	v := devVerifier(t, "k1", now)
	h.deps.Verifier = v
	handler, next, _ := chained(t, h)
	alice := v.Mint("alice", now.Add(time.Hour))
	bob := v.Mint("bob", now.Add(time.Hour))
	first := postChained(t, handler, alice, "k1", `{"amount":5}`)
	second := postChained(t, handler, bob, "k1", `{"amount":5}`)
	if first.Code != http.StatusCreated || second.Code != http.StatusCreated || next.calls.Load() != 2 {
		t.Fatalf("alice %d, bob %d, %d handler calls; want two 201s: Auth runs before Idempotency", first.Code,
			second.Code, next.calls.Load())
	}
	sameResponse(t, first, postChained(t, handler, alice, "k1", `{"amount":5}`))
	if next.calls.Load() != 2 {
		t.Fatalf("handler ran %d times, want 2 after alice's replay", next.calls.Load())
	}
}
