package httpx

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	openapi "github.com/monaco/monaco/apps/backend/api"
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
	c, err := LoadContract([]byte(authSpec))
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

func TestAuth_keepsTheVerifierCodeOnlyForAuthInternalAndOutageKinds(t *testing.T) {
	t.Parallel()
	for code, want := range map[errs.Code]struct {
		status int
		code   string
	}{
		errs.CodeSessionRequired: {http.StatusUnauthorized, "session_required"},
		errs.CodeInternal:        {http.StatusInternalServerError, "internal"},
		errs.CodeInvalidInput:    {http.StatusUnauthorized, "unauthorized"},
		errs.CodeNotFound:        {http.StatusUnauthorized, "unauthorized"},
	} {
		t.Run(string(code), func(t *testing.T) {
			t.Parallel()
			h := newHarness(t)
			handler, actors := authed(t, h, stubVerifier(func(context.Context, string) (auth.Actor, error) {
				return auth.Actor{}, errs.New(code, "test.Verify")
			}))
			rec := serveRaw(t, handler, http.MethodGet, "/v1/me", bearer("any"))
			if p := decodeProblem(t, rec); rec.Code != want.status || string(p.Code) != want.code || len(*actors) != 0 {
				t.Fatalf("got %d %+v with actors %v, want %d %s", rec.Code, p, *actors, want.status, want.code)
			}
		})
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

func TestRestrictedRoutes_matchesTheGoldenList(t *testing.T) {
	t.Parallel()
	got, err := restrictedRoutes(openapi.Spec)
	if err != nil {
		t.Fatal(err)
	}
	text := strings.Join(got, "\n")
	if len(got) > 0 {
		text += "\n"
	}
	want, err := os.ReadFile("testdata/restricted_routes.golden")
	if err != nil {
		t.Fatal(err)
	}
	if text != string(want) {
		t.Fatalf("restricted routes =\n%s\nwant\n%s", text, want)
	}
}

func TestRestrictedRoutes_keepsOnlyABooleanTrue(t *testing.T) {
	t.Parallel()
	got, err := restrictedRoutes([]byte(`openapi: 3.1.0
info: {title: t, version: "1"}
paths:
  /v1/yes:
    delete:
      operationId: deleteYes
      x-allow-restricted: true
      responses: {"200": {description: ok}}
  /v1/no:
    get:
      operationId: getNo
      x-allow-restricted: false
      responses: {"200": {description: ok}}
  /v1/text:
    put:
      operationId: putText
      x-allow-restricted: "true"
      responses: {"200": {description: ok}}
  /v1/plain:
    post:
      operationId: postPlain
      responses: {"200": {description: ok}}
`))
	if err != nil || len(got) != 1 || got[0] != "DELETE /v1/yes" {
		t.Fatalf("restrictedRoutes = %v, %v, want [DELETE /v1/yes]", got, err)
	}
}

func TestRestrictedRoutes_anEmptyPathMapIsAnEmptyList(t *testing.T) {
	t.Parallel()
	got, err := restrictedRoutes([]byte("openapi: 3.1.0\ninfo: {title: t, version: \"1\"}\npaths: {}\n"))
	if err != nil || len(got) != 0 {
		t.Fatalf("restrictedRoutes = %v, %v, want an empty list", got, err)
	}
}

func TestRestrictedRoutes_rejectsASpecThatDoesNotParse(t *testing.T) {
	t.Parallel()
	_, err := restrictedRoutes([]byte("openapi: ["))
	if errs.CodeOf(err) != errs.CodeInvalidInput {
		t.Fatalf("restrictedRoutes = %v, want invalid_input", err)
	}
}

const standingSpec = `openapi: 3.1.0
info: {title: fixture, version: "1"}
security:
  - bearerAuth: []
components:
  securitySchemes:
    bearerAuth: {type: http, scheme: bearer}
paths:
  /v1/open-read:
    get:
      operationId: getOpenRead
      responses: {"204": {description: ok}}
    head:
      operationId: headOpenRead
      responses: {"204": {description: ok}}
  /v1/mutate:
    post:
      operationId: postMutate
      responses: {"204": {description: ok}}
  /v1/marked:
    get:
      operationId: getMarked
      x-allow-restricted: true
      responses: {"204": {description: ok}}
  /v1/cash-out:
    post:
      operationId: postCashOut
      x-allow-restricted: true
      responses: {"204": {description: ok}}
`

func standingRoutes(t *testing.T, h *harness, standing auth.Standing) http.Handler {
	t.Helper()
	c, err := LoadContract([]byte(standingSpec))
	if err != nil {
		t.Fatal(err)
	}
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	v := stubVerifier(func(context.Context, string) (auth.Actor, error) {
		return auth.Actor{Kind: auth.ActorUser, ID: "u-1", Standing: standing}, nil
	})
	mux := http.NewServeMux()
	for _, route := range []string{
		"GET /v1/open-read", "HEAD /v1/open-read", "POST /v1/mutate", "GET /v1/marked", "POST /v1/cash-out",
	} {
		mux.Handle(route, c.resolve(Auth(v)(next)))
	}
	return h.deps.wrap(mux)
}

func TestAuth_standingAllowsOrRefusesByMethodAndMarker(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		standing     auth.Standing
		method, path string
		status       int
		code, op     string
	}{
		{auth.StandingActive, http.MethodGet, "/v1/open-read", http.StatusNoContent, "", ""},
		{auth.StandingActive, http.MethodPost, "/v1/mutate", http.StatusNoContent, "", ""},
		{auth.StandingActive, http.MethodGet, "/v1/marked", http.StatusNoContent, "", ""},
		{auth.StandingActive, http.MethodPost, "/v1/cash-out", http.StatusNoContent, "", ""},
		{auth.StandingSuspended, http.MethodGet, "/v1/open-read", http.StatusNoContent, "", ""},
		{auth.StandingSuspended, http.MethodHead, "/v1/open-read", http.StatusNoContent, "", ""},
		{auth.StandingSuspended, http.MethodPost, "/v1/mutate", http.StatusForbidden, "account_suspended", "postMutate"},
		{auth.StandingSuspended, http.MethodGet, "/v1/marked", http.StatusNoContent, "", ""},
		{auth.StandingSuspended, http.MethodPost, "/v1/cash-out", http.StatusNoContent, "", ""},
		{auth.StandingBanned, http.MethodGet, "/v1/open-read", http.StatusForbidden, "account_banned", "getOpenRead"},
		{auth.StandingBanned, http.MethodHead, "/v1/open-read", http.StatusForbidden, "account_banned", "headOpenRead"},
		{auth.StandingBanned, http.MethodPost, "/v1/mutate", http.StatusForbidden, "account_banned", "postMutate"},
		{auth.StandingBanned, http.MethodGet, "/v1/marked", http.StatusNoContent, "", ""},
		{auth.StandingBanned, http.MethodPost, "/v1/cash-out", http.StatusNoContent, "", ""},
		{auth.StandingDeleted, http.MethodGet, "/v1/marked", http.StatusForbidden, "account_deleted", "getMarked"},
		{auth.StandingDeleted, http.MethodPost, "/v1/cash-out", http.StatusForbidden, "account_deleted", "postCashOut"},
		{auth.StandingDeleted, http.MethodGet, "/v1/open-read", http.StatusForbidden, "account_deleted", "getOpenRead"},
		{auth.StandingDeleted, http.MethodPost, "/v1/mutate", http.StatusForbidden, "account_deleted", "postMutate"},
		{auth.Standing("frozen"), http.MethodPost, "/v1/mutate", http.StatusNoContent, "", ""},
	} {
		t.Run(string(tc.standing)+" "+tc.method+" "+tc.path, func(t *testing.T) {
			t.Parallel()
			h := newHarness(t)
			rec := serveRaw(t, standingRoutes(t, h, tc.standing), tc.method, tc.path, bearer("any"))
			refused := linesNamed(h.logs.lines(t), "httpx.auth.restricted")
			if tc.code == "" {
				if rec.Code != tc.status || len(refused) != 0 {
					t.Fatalf("got %d with %d refusal lines, want %d and none", rec.Code, len(refused), tc.status)
				}
				return
			}
			p := decodeProblem(t, rec)
			if rec.Code != tc.status || string(p.Code) != tc.code || len(refused) != 1 ||
				refused[0]["standing"] != string(tc.standing) || refused[0]["op"] != tc.op ||
				refused[0]["code"] != tc.code {
				t.Fatalf("got %d %+v lines %v, want %d %s op %s", rec.Code, p, refused, tc.status, tc.code, tc.op)
			}
		})
	}
}

func TestAuth_bannedActorMayReadPlatformBalance(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	c, err := LoadContract(openapi.Spec)
	if err != nil {
		t.Fatal(err)
	}
	var got auth.Actor
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got, _ = auth.ActorFrom(r.Context())
		w.WriteHeader(http.StatusOK)
	})
	v := stubVerifier(func(context.Context, string) (auth.Actor, error) {
		return auth.Actor{Kind: auth.ActorUser, ID: "u-1", Standing: auth.StandingBanned}, nil
	})
	mux := http.NewServeMux()
	mux.Handle("GET /v1/me/balance", c.resolve(Auth(v)(next)))
	rec := serveRaw(t, h.deps.wrap(mux), http.MethodGet, "/v1/me/balance", bearer("any"))
	if rec.Code != http.StatusOK || got.Standing != auth.StandingBanned {
		t.Fatalf("got status %d and actor %+v, want 200 and a banned actor", rec.Code, got)
	}
}

func TestAuth_bannedActorMayReadUserTransactions(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	c, err := LoadContract(openapi.Spec)
	if err != nil {
		t.Fatal(err)
	}
	var got auth.Actor
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got, _ = auth.ActorFrom(r.Context())
		w.WriteHeader(http.StatusOK)
	})
	v := stubVerifier(func(context.Context, string) (auth.Actor, error) {
		return auth.Actor{Kind: auth.ActorUser, ID: "u-1", Standing: auth.StandingBanned}, nil
	})
	mux := http.NewServeMux()
	mux.Handle("GET /v1/me/txns", c.resolve(Auth(v)(next)))
	rec := serveRaw(t, h.deps.wrap(mux), http.MethodGet, "/v1/me/txns", bearer("any"))
	if rec.Code != http.StatusOK || got.Standing != auth.StandingBanned {
		t.Fatalf("got status %d and actor %+v, want 200 and a banned actor", rec.Code, got)
	}
}

func TestHandler_requiresAVerifier(t *testing.T) {
	t.Parallel()
	d := newHarness(t).deps
	d.Verifier = nil
	if h, err := Handler(
		d,
		mountPlatform(healthOnly{}),
		openapi.Spec,
	); h != nil ||
		errs.CodeOf(err) != errs.CodeInternal {
		t.Fatalf("Handler = %v, %v, want internal and no handler", h, err)
	}
}
