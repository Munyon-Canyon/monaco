package httpx

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/platform/auth"
	"github.com/monaco/monaco/apps/backend/internal/platform/config"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

type stubVerifier func(ctx context.Context, raw string) (auth.Actor, error)

func (v stubVerifier) Verify(ctx context.Context, raw string) (auth.Actor, error) {
	if v == nil {
		return auth.Actor{}, errs.New(errs.CodeInternal, "stubVerifier.Verify")
	}
	return v(ctx, raw)
}

const authSpec = `openapi: 3.1.0
info: {title: fixture, version: "1"}
security:
  - bearerAuth: []
components:
  securitySchemes:
    bearerAuth: {type: http, scheme: bearer}
paths:
  /v1/me:
    get:
      operationId: getMe
      responses:
        "200": {description: ok}
  /v1/open:
    get:
      operationId: getOpen
      security: []
      responses:
        "200": {description: ok}
`

func devVerifier(t *testing.T, key string, now time.Time) *auth.DevVerifier {
	t.Helper()
	cfg := config.Config{Env: config.EnvTest, Auth: config.Auth{DevTokenKey: key}}
	v, err := auth.NewDevVerifier(cfg, testkit.NewClock(now))
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func authed(t *testing.T, h *harness, v auth.TokenVerifier) (http.Handler, *[]string) {
	t.Helper()
	c, err := loadContract([]byte(authSpec))
	if err != nil {
		t.Fatal(err)
	}
	var actors []string
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		a, _ := auth.ActorFrom(r.Context())
		actors = append(actors, a.Key())
		w.WriteHeader(http.StatusNoContent)
	})
	mux := http.NewServeMux()
	mux.Handle("GET /v1/me", c.resolve(Auth(v)(next)))
	mux.Handle("GET /v1/open", c.resolve(Auth(v)(next)))
	return h.deps.wrap(mux), &actors
}

func bearer(token string) http.Header {
	return http.Header{"Authorization": {"Bearer " + token}}
}

func expectUnauthorized(t *testing.T, h *harness, rec *httptest.ResponseRecorder, reason string) {
	t.Helper()
	p := decodeProblem(t, rec)
	if rec.Code != http.StatusUnauthorized || p.Code != "unauthorized" ||
		p.Message != errs.Message(errs.CodeUnauthorized) || p.Retryable {
		t.Fatalf("got %d %+v, want 401 unauthorized", rec.Code, p)
	}
	line := linesNamed(h.logs.lines(t), "http.problem")[0]
	if detail, _ := line["detail"].(map[string]any); detail["reason"] != reason {
		t.Fatalf("problem line = %v, want reason %q", line, reason)
	}
}

func TestAuth_aValidDevTokenPutsTheUserActorInContextAndTheAccessLog(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	now := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	v := devVerifier(t, "k1", now)
	handler, actors := authed(t, h, v)
	rec := serveRaw(t, handler, http.MethodGet, "/v1/me", bearer(v.Mint("u-1", now.Add(time.Hour))))
	if rec.Code != http.StatusNoContent || len(*actors) != 1 || (*actors)[0] != "user:u-1" {
		t.Fatalf("got %d, actors %v, want 204 and user:u-1", rec.Code, *actors)
	}
	access := linesNamed(h.logs.lines(t), "http.request")
	if len(access) != 1 || access[0]["actor"] != "user:u-1" {
		t.Fatalf("access line = %v, want actor user:u-1", access)
	}
}

func TestAuth_openOperationsSkipTheVerifier(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	handler, actors := authed(t, h, stubVerifier(nil))
	rec := serveRaw(t, handler, http.MethodGet, "/v1/open", nil)
	if rec.Code != http.StatusNoContent || len(*actors) != 1 || (*actors)[0] != ":" {
		t.Fatalf("got %d, actors %v, want 204 with no actor", rec.Code, *actors)
	}
	access := linesNamed(h.logs.lines(t), "http.request")
	if len(access) != 1 || access[0]["actor"] != nil {
		t.Fatalf("access line = %v, want no actor key", access)
	}
	if rec := h.do(
		t,
		mustHandler(t, h.deps, healthOnly{}),
		http.MethodGet,
		"/healthz",
		nil,
	); rec.Code != http.StatusOK {
		t.Fatalf("GET /healthz without a token = %d, want 200: security is [] in the spec", rec.Code)
	}
}

func TestAuth_missingOrRejectedCredentialsAre401(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	for name, tc := range map[string]struct {
		header func(v *auth.DevVerifier) http.Header
		reason string
	}{
		"missing header": {func(*auth.DevVerifier) http.Header { return nil }, "no_bearer_token"},
		"basic scheme": {func(*auth.DevVerifier) http.Header {
			return http.Header{"Authorization": {"Basic dXNlcjpwYXNz"}}
		}, "no_bearer_token"},
		"bearer without token": {func(*auth.DevVerifier) http.Header {
			return http.Header{"Authorization": {"Bearer "}}
		}, "no_bearer_token"},
		"expired":   {func(v *auth.DevVerifier) http.Header { return bearer(v.Mint("u-1", now)) }, "expired"},
		"not a jwt": {func(*auth.DevVerifier) http.Header { return bearer("opaque") }, "malformed"},
		"wrong key": {func(*auth.DevVerifier) http.Header {
			return bearer(mustMint(t, "k2", now, "u-1", now.Add(time.Hour)))
		}, "bad_signature"},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			h := newHarness(t)
			v := devVerifier(t, "k1", now)
			handler, actors := authed(t, h, v)
			rec := serveRaw(t, handler, http.MethodGet, "/v1/me", tc.header(v))
			expectUnauthorized(t, h, rec, tc.reason)
			if len(*actors) != 0 {
				t.Fatalf("handler ran with actors %v", *actors)
			}
		})
	}
}

func mustMint(t *testing.T, key string, now time.Time, user string, exp time.Time) string {
	t.Helper()
	return devVerifier(t, key, now).Mint(user, exp)
}

func TestAuth_lowercaseBearerSchemeIsAccepted(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	now := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	v := devVerifier(t, "k1", now)
	handler, actors := authed(t, h, v)
	header := http.Header{"Authorization": {"bearer  " + v.Mint("u-2", now.Add(time.Hour))}}
	if rec := serveRaw(t, handler, http.MethodGet, "/v1/me", header); rec.Code != http.StatusNoContent ||
		len(*actors) != 1 || (*actors)[0] != "user:u-2" {
		t.Fatalf("got %d, actors %v", rec.Code, *actors)
	}
}

func TestAuth_aVerifierOutageIs503NotA401(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	down := errs.New(errs.CodeUpstreamUnavailable, "privy.Verify")
	handler, _ := authed(t, h, stubVerifier(func(context.Context, string) (auth.Actor, error) {
		return auth.Actor{}, down
	}))
	rec := serveRaw(t, handler, http.MethodGet, "/v1/me", bearer("any"))
	if p := decodeProblem(t, rec); rec.Code != http.StatusServiceUnavailable || p.Code != "upstream_unavailable" ||
		!p.Retryable {
		t.Fatalf("got %d %+v, want 503 upstream_unavailable", rec.Code, p)
	}
}

func TestAuth_withoutAResolvedRouteFailsClosed(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) })
	rec := serveRaw(t, h.deps.wrap(Auth(stubVerifier(nil))(next)), http.MethodGet, "/x", bearer("any"))
	if p := decodeProblem(t, rec); rec.Code != http.StatusInternalServerError || p.Code != "internal" {
		t.Fatalf("got %d %+v, want 500 internal", rec.Code, p)
	}
}

func TestAuth_noLogLineCarriesTheToken(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	h := newHarness(t)
	v := devVerifier(t, "k1", now)
	handler, _ := authed(t, h, v)
	valid := v.Mint("u-1", now.Add(time.Hour))
	expired := v.Mint("u-1", now)
	opaque := "opaque-secret-token"
	for _, token := range []string{valid, expired, opaque} {
		serveRaw(t, handler, http.MethodGet, "/v1/me", bearer(token))
	}
	serveRaw(t, handler, http.MethodGet, "/v1/me", http.Header{"Authorization": {"Basic c2VjcmV0"}})
	raw := h.logs.buf.String()
	if strings.Count(raw, "\n") < 6 {
		t.Fatalf("expected access and problem lines for four requests, got %q", raw)
	}
	for _, secret := range []string{valid, expired, opaque, "c2VjcmV0", "Bearer ", "Basic "} {
		if strings.Contains(raw, secret) {
			t.Fatalf("logs carry %q:\n%s", secret, raw)
		}
	}
}

func TestHandler_requiresAVerifier(t *testing.T) {
	t.Parallel()
	d := newHarness(t).deps
	d.Verifier = nil
	if h, err := Handler(d, healthOnly{}); h != nil || errs.CodeOf(err) != errs.CodeInternal {
		t.Fatalf("Handler = %v, %v, want internal and no handler", h, err)
	}
}
