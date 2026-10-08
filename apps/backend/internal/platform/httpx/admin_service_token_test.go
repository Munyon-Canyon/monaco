package httpx

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/getkin/kin-openapi/routers"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/platform/auth"
)

const mstValue = "mst_c2VjcmV0LXNlcnZpY2UtdG9rZW4tdmFsdWU"

type serviceVerifier struct {
	stubVerifier
	resolve func(raw string) (string, error)
}

func (s serviceVerifier) VerifyServiceToken(_ context.Context, raw string) (string, error) {
	return s.resolve(raw)
}

func grafana(raw string) (string, error) {
	if raw != mstValue {
		return "", errs.New(errs.CodeUnauthorized, "test")
	}
	return "grafana", nil
}

func serveServiceToken(
	t *testing.T, v auth.TokenVerifier, method, role, token string,
) (*harness, *httptest.ResponseRecorder, bool) {
	t.Helper()
	h := newHarness(t)
	route := &routers.Route{
		Path: "/v1/admin/dashboards/money",
		Operation: &openapi3.Operation{
			OperationID: "getAdminMoneyDashboard", Extensions: map[string]any{adminRoleExtension: role},
		},
	}
	reached := false
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		reached = true
		w.WriteHeader(http.StatusNoContent)
	})
	ctx := context.WithValue(context.Background(), routeKey{}, resolved{route: route})
	req := httptest.NewRequestWithContext(ctx, method, "http://example.test"+route.Path, nil)
	req.Header = bearer(token)
	rec := httptest.NewRecorder()
	h.deps.wrap(Admin(v)(next)).ServeHTTP(rec, req)
	return h, rec, reached
}

func TestAdminServiceToken_viewerGetPassesAsAViewerServiceActorAndLogsOnlyTheName(t *testing.T) {
	t.Parallel()
	h, rec, reached := serveServiceToken(t, serviceVerifier{resolve: grafana}, http.MethodGet, "viewer", mstValue)
	if rec.Code != http.StatusNoContent || !reached {
		t.Fatalf("status = %d, reached %v", rec.Code, reached)
	}
	lines := linesNamed(h.logs.lines(t), "admin.request")
	if len(lines) != 1 || lines[0]["admin_id"] != "service:grafana" || lines[0]["role"] != "viewer" ||
		lines[0]["status"] != float64(http.StatusNoContent) {
		t.Fatalf("admin log = %v", lines)
	}
	h.logs.mu.Lock()
	defer h.logs.mu.Unlock()
	if strings.Contains(h.logs.buf.String(), mstValue) || strings.Contains(h.logs.buf.String(), "c2VjcmV0") {
		t.Fatalf("log carries the token: %s", h.logs.buf.String())
	}
}

func TestAdminServiceToken_refusesEveryOtherMethodAndEveryRouteAboveViewer(t *testing.T) {
	t.Parallel()
	verifier := serviceVerifier{resolve: grafana}
	for name, tc := range map[string]struct{ method, role string }{
		"post viewer":   {http.MethodPost, "viewer"},
		"patch viewer":  {http.MethodPatch, "viewer"},
		"delete viewer": {http.MethodDelete, "viewer"},
		"put viewer":    {http.MethodPut, "viewer"},
		"head viewer":   {http.MethodHead, "viewer"},
		"get moderator": {http.MethodGet, "moderator"},
		"get operator":  {http.MethodGet, "operator"},
		"post operator": {http.MethodPost, "operator"},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			h, rec, reached := serveServiceToken(t, verifier, tc.method, tc.role, mstValue)
			if rec.Code != http.StatusForbidden || reached {
				t.Fatalf("status = %d, reached %v, want 403 and not reached", rec.Code, reached)
			}
			assertProblemCode(t, rec, errs.CodeAdminForbidden)
			if lines := linesNamed(h.logs.lines(t), "admin.request"); len(lines) != 0 {
				t.Fatalf("refused request logged as served: %v", lines)
			}
		})
	}
}

func TestAdminServiceToken_unknownOrRevokedTokensAreUnauthorizedOnEveryRoute(t *testing.T) {
	t.Parallel()
	verifier := serviceVerifier{resolve: grafana}
	for _, tc := range []struct{ method, role string }{
		{http.MethodGet, "viewer"}, {http.MethodPost, "viewer"}, {http.MethodGet, "operator"},
	} {
		_, rec, reached := serveServiceToken(t, verifier, tc.method, tc.role, "mst_unknown")
		if rec.Code != http.StatusUnauthorized || reached {
			t.Fatalf("%s %s = %d, reached %v, want 401", tc.method, tc.role, rec.Code, reached)
		}
		assertProblemCode(t, rec, errs.CodeUnauthorized)
	}
}

func TestAdminServiceToken_aVerifierWithoutServiceTokensRejectsThemAndLeavesPrivyAlone(t *testing.T) {
	t.Parallel()
	privy := stubVerifier(func(_ context.Context, raw string) (auth.Actor, error) {
		if raw != "privy.jwt" {
			t.Errorf("Privy verifier saw %q", raw)
		}
		return auth.Actor{Kind: auth.ActorAdmin, ID: "admin", Role: "operator"}, nil
	})
	_, rec, reached := serveServiceToken(t, privy, http.MethodGet, "viewer", mstValue)
	if rec.Code != http.StatusUnauthorized || reached {
		t.Fatalf("service token on a Privy-only verifier = %d, reached %v", rec.Code, reached)
	}
	both := serviceVerifier{stubVerifier: privy, resolve: func(string) (string, error) {
		t.Error("service lookup ran for a Privy token")
		return "", nil
	}}
	_, rec, reached = serveServiceToken(t, both, http.MethodPost, "operator", "privy.jwt")
	if rec.Code != http.StatusNoContent || !reached {
		t.Fatalf("Privy operator POST = %d, reached %v", rec.Code, reached)
	}
}

func TestAdminServiceToken_lookupOutageSurfacesAsUnavailableNotForbidden(t *testing.T) {
	t.Parallel()
	down := serviceVerifier{resolve: func(string) (string, error) {
		return "", errs.New(errs.CodeDBUnavailable, "test")
	}}
	_, rec, reached := serveServiceToken(t, down, http.MethodGet, "viewer", mstValue)
	if rec.Code != http.StatusServiceUnavailable || reached {
		t.Fatalf("status = %d, reached %v, want 503", rec.Code, reached)
	}
}
