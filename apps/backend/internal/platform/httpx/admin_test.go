package httpx

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/getkin/kin-openapi/routers"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/platform/auth"
)

type adminCase struct {
	path       string
	extensions map[string]any
	header     http.Header
	verifier   auth.TokenVerifier
	status     int
	code       errs.Code
}

func TestAdmin_gatesRoutesAndLogsAuthorizedRequests(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]adminCase{
		"missing role": {
			path: "/v1/admin/me", header: bearer("token"), verifier: stubVerifier(nil),
			status: http.StatusInternalServerError, code: errs.CodeInternal,
		},
		"missing bearer": {
			path: "/v1/admin/me", extensions: map[string]any{adminRoleExtension: "viewer"}, verifier: stubVerifier(nil),
			status: http.StatusUnauthorized, code: errs.CodeUnauthorized,
		},
		"missing verifier": {
			path: "/v1/admin/me", extensions: map[string]any{adminRoleExtension: "viewer"}, header: bearer("token"),
			status: http.StatusForbidden, code: errs.CodeAdminForbidden,
		},
		"rejected token": {
			path: "/v1/admin/me", extensions: map[string]any{adminRoleExtension: "viewer"}, header: bearer("token"),
			verifier: stubVerifier(func(context.Context, string) (auth.Actor, error) {
				return auth.Actor{}, errs.New(errs.CodeUnauthorized, "test")
			}), status: http.StatusUnauthorized, code: errs.CodeUnauthorized,
		},
		"ungranted admin": {
			path: "/v1/admin/me", extensions: map[string]any{adminRoleExtension: "viewer"}, header: bearer("token"),
			verifier: stubVerifier(func(context.Context, string) (auth.Actor, error) {
				return auth.Actor{}, errs.New(errs.CodeAdminForbidden, "test")
			}), status: http.StatusForbidden, code: errs.CodeAdminForbidden,
		},
		"viewer on operator": {
			path: "/v1/admin/admins", extensions: map[string]any{adminRoleExtension: "operator"}, header: bearer("token"),
			verifier: stubVerifier(func(context.Context, string) (auth.Actor, error) {
				return auth.Actor{Kind: auth.ActorAdmin, ID: "admin", Role: "viewer"}, nil
			}), status: http.StatusForbidden, code: errs.CodeAdminForbidden,
		},
		"operator succeeds": {
			path: "/v1/admin/me", extensions: map[string]any{adminRoleExtension: "viewer"}, header: bearer("token"),
			verifier: stubVerifier(func(context.Context, string) (auth.Actor, error) {
				return auth.Actor{Kind: auth.ActorAdmin, ID: "admin", Role: "operator"}, nil
			}), status: http.StatusNoContent,
		},
		"non admin bypasses": {
			path: "/v1/me", verifier: stubVerifier(nil), status: http.StatusNoContent,
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			testAdminCase(t, name, tc)
		})
	}
}

func testAdminCase(t *testing.T, name string, tc adminCase) {
	t.Helper()
	h := newHarness(t)
	route := &routers.Route{
		Path:      tc.path,
		Operation: &openapi3.Operation{OperationID: "getAdmin", Extensions: tc.extensions},
	}
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) })
	handler := h.deps.wrap(Admin(tc.verifier)(next))
	ctx := context.WithValue(context.Background(), routeKey{}, resolved{route: route})
	req := httptest.NewRequestWithContext(ctx, http.MethodGet, "http://example.test"+tc.path, nil)
	req.Header = tc.header
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != tc.status {
		t.Fatalf("status = %d, want %d", rec.Code, tc.status)
	}
	if tc.code != "" {
		assertProblemCode(t, rec, tc.code)
	}
	if name == "operator succeeds" {
		assertAdminLog(t, h)
	}
}

func assertProblemCode(t *testing.T, rec *httptest.ResponseRecorder, want errs.Code) {
	t.Helper()
	if got := string(decodeProblem(t, rec).Code); got != string(want) {
		t.Fatalf("problem = %s, want %s", got, want)
	}
}

func assertAdminLog(t *testing.T, h *harness) {
	t.Helper()
	lines := linesNamed(h.logs.lines(t), "admin.request")
	if len(lines) != 1 || lines[0]["admin_id"] != "admin" || lines[0]["role"] != "operator" ||
		lines[0]["status"] != float64(http.StatusNoContent) || lines[0]["request_id"] == nil {
		t.Fatalf("admin log = %v", lines)
	}
}

func TestAdminRouteHelpers(t *testing.T) {
	t.Parallel()
	if isAdminRoute(resolved{route: &routers.Route{Path: "/v1/me"}}) {
		t.Fatal("non-admin route is admin")
	}
	if !isAdminRoute(resolved{route: &routers.Route{Path: "/v1/admin/me"}}) {
		t.Fatal("admin route is not admin")
	}
	for _, extensions := range []map[string]any{nil, {adminRoleExtension: "invalid"}, {adminRoleExtension: "viewer"}} {
		_, ok := adminRole(extensions)
		if ok != (extensions != nil && extensions[adminRoleExtension] == "viewer") {
			t.Fatalf("adminRole(%v) = %v", extensions, ok)
		}
	}
}
