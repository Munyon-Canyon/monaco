package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.opentelemetry.io/otel/trace/noop"

	openapi "github.com/monaco/monaco/apps/backend/api"
	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/admin/adapters/authn"
	"github.com/monaco/monaco/apps/backend/internal/modules/admin/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/auth"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/config"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpx"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
	"github.com/monaco/monaco/apps/backend/internal/testkit/fakes"
)

type noPrivy struct{}

func (noPrivy) Verify(context.Context, string) (auth.Actor, error) {
	return auth.Actor{}, errs.New(errs.CodeUnauthorized, "test")
}

var pathParam = regexp.MustCompile(`\{[^}]+\}`)

func privyConfig() config.Config {
	cfg := testkit.Config()
	cfg.Privy = config.Privy{BaseURL: "http://127.0.0.1", VerificationKey: fakes.PrivyVerificationKey()}
	cfg.Timeouts.Privy = time.Second
	return cfg
}

func serviceTokenAPI(t *testing.T, pool *pgxpool.Pool, logs io.Writer) http.Handler {
	t.Helper()
	clk := testkit.NewClock(time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC))
	deps := module.Deps{Pool: pool, Clock: clock.Real{}, IDs: ids.Real{}, Config: privyConfig()}
	h, err := httpx.Handler(httpx.Deps{
		Logger:        observability.NewLogger(config.Config{Env: config.EnvTest}, logs),
		Tracer:        noop.NewTracerProvider(),
		Clock:         clk,
		IDs:           testkit.NewIDs(1),
		MaxBodyBytes:  1 << 20,
		Idempotency:   db.NewIdempotencyStore(pool, clk),
		Verifier:      noPrivy{},
		AdminVerifier: authn.NewAdminVerifier(noPrivy{}, pool),
	}, registered.Build(deps).Mount, openapi.Spec)
	if err != nil {
		t.Fatal(err)
	}
	return testkit.HTTP(t, h)
}

func mintServiceToken(t *testing.T, pool *pgxpool.Pool, name string) string {
	t.Helper()
	minted, hash, err := domain.NewServiceToken(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(t.Context(),
		`INSERT INTO admin_service_tokens (id, name, token_hash, created_at, created_by)
VALUES (gen_random_uuid(), $1, $2, now(), 'test')`, name, hash); err != nil {
		t.Fatal(err)
	}
	return minted
}

func serviceCall(t *testing.T, h http.Handler, method, target, bearer string) *httptest.ResponseRecorder {
	t.Helper()
	var body io.Reader
	if method != http.MethodGet {
		body = strings.NewReader(`{"reason":"duplicate of a fixed bug"}`)
	}
	r := httptest.NewRequestWithContext(t.Context(), method, target, body)
	r.Header.Set("Authorization", "Bearer "+bearer)
	if body != nil {
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("Idempotency-Key", "019cc330-1111-7000-8000-0000000000aa")
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

func problemCodeOf(t *testing.T, w *httptest.ResponseRecorder) string {
	t.Helper()
	var body struct {
		Code string `json:"code"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode %s: %v", w.Body, err)
	}
	return body.Code
}

func TestAdminServiceToken_readsTheFourDashboardsUntilRevoked(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	var logs bytes.Buffer
	h := serviceTokenAPI(t, pool, &logs)
	minted := mintServiceToken(t, pool, "grafana")
	query := "?from=2026-09-01&to=2026-09-15&bucket=week"
	for _, route := range []string{"money", "governance", "safety", "social"} {
		if w := serviceCall(
			t,
			h,
			http.MethodGet,
			"/v1/admin/dashboards/"+route+query,
			minted,
		); w.Code != http.StatusOK {
			t.Fatalf("%s = %d, body %s", route, w.Code, w.Body)
		}
	}
	if !strings.Contains(logs.String(), `"admin_id":"service:grafana"`) || strings.Contains(logs.String(), minted) {
		t.Fatalf("logs must name the token and never carry it: %s", logs.String())
	}
	if _, err := pool.Exec(
		t.Context(),
		`UPDATE admin_service_tokens SET revoked_at = now() WHERE name = 'grafana'`,
	); err != nil {
		t.Fatal(err)
	}
	for _, bearer := range []string{minted, "mst_never-issued"} {
		w := serviceCall(t, h, http.MethodGet, "/v1/admin/dashboards/money"+query, bearer)
		if w.Code != http.StatusUnauthorized || problemCodeOf(t, w) != string(errs.CodeUnauthorized) {
			t.Fatalf("%.8s after revoke = %d, body %s", bearer, w.Code, w.Body)
		}
	}
}

type adminOp struct{ method, path, role string }

func notViewerGets(t *testing.T) []adminOp {
	t.Helper()
	doc, err := openapi3.NewLoader().LoadFromData(openapi.Spec)
	if err != nil {
		t.Fatal(err)
	}
	var out []adminOp
	for path, item := range doc.Paths.Map() {
		for method, op := range item.Operations() {
			role, _ := op.Extensions["x-admin-role"].(string)
			if strings.HasPrefix(path, "/v1/admin/") && (method != http.MethodGet || role != "viewer") {
				out = append(out, adminOp{method, path, role})
			}
		}
	}
	return out
}

func TestAdminServiceToken_everyOtherAdminRouteIsForbidden(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	h := serviceTokenAPI(t, pool, io.Discard)
	minted := mintServiceToken(t, pool, "grafana")
	seen := map[string]int{}
	for _, op := range notViewerGets(t) {
		seen[op.method+" "+op.role]++
		target := pathParam.ReplaceAllString(op.path, "019cc330-1111-7000-8000-000000000001")
		w := serviceCall(t, h, op.method, target, minted)
		if w.Code != http.StatusForbidden || problemCodeOf(t, w) != string(errs.CodeAdminForbidden) {
			t.Errorf("%s %s = %d (role %s), want 403 admin_forbidden", op.method, op.path, w.Code, op.role)
		}
	}
	for _, want := range []string{"POST operator", "GET operator", "GET moderator"} {
		if seen[want] == 0 {
			t.Errorf("no %s route was checked; saw %v", want, seen)
		}
	}
}
