package httpx

import (
	"context"
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"

	openapi "github.com/monaco/monaco/apps/backend/api"
	"github.com/monaco/monaco/apps/backend/internal/platform/config"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpx/api"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpx/api/identityapi"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpx/api/platformapi"
)

type devOnlyOperation struct{ method, path string }

func devOnlyOperations(t *testing.T) []devOnlyOperation {
	t.Helper()
	doc, err := openapi3.NewLoader().LoadFromData(openapi.Spec)
	if err != nil {
		t.Fatal(err)
	}
	var ops []devOnlyOperation
	for path, item := range doc.Paths.Map() {
		for method, operation := range item.Operations() {
			if devOnly(operation.Extensions) {
				ops = append(ops, devOnlyOperation{method, path})
			}
		}
	}
	slices.SortFunc(ops, func(a, b devOnlyOperation) int { return strings.Compare(a.method+a.path, b.method+b.path) })
	return ops
}

func mustIdentityHandler(t *testing.T, d Deps) http.Handler {
	t.Helper()
	var unreached identityapi.StrictServerInterface
	h, err := Handler(d, func(m api.Mount) { identityapi.Mount(unreached, m) }, openapi.Spec)
	if err != nil {
		t.Fatal(err)
	}
	return h
}

func TestDevOnlyRoutes(t *testing.T) {
	t.Parallel()
	ops := devOnlyOperations(t)
	want := []devOnlyOperation{
		{http.MethodDelete, "/v1/dev/me/x-link"},
		{http.MethodPost, "/v1/dev/me/x-link"},
	}
	if !slices.Equal(ops, want) {
		t.Fatalf("x-dev-only operations = %v, want %v", ops, want)
	}
	h := newHarness(t)
	h.deps.Env = config.EnvProduction
	handler := mustIdentityHandler(t, h.deps)
	header := http.Header{"Authorization": {"Bearer token"}, "Idempotency-Key": {"k1"}}
	for _, op := range ops {
		resp := h.do(t, handler, op.method, op.path, header)
		if p := decodeProblem(t, resp); resp.Code != http.StatusNotFound || p.Code != "not_found" {
			t.Fatalf("%s %s under production = %d %+v, want 404 not_found", op.method, op.path, resp.Code, p)
		}
	}
	refused := linesNamed(h.logs.lines(t), "http.problem")
	if len(refused) != len(ops) {
		t.Fatalf("got %d http.problem lines, want %d", len(refused), len(ops))
	}
	for _, line := range refused {
		if line["err"] != "httpx.refuseDevOnly: not_found" {
			t.Fatalf("problem line = %v, want the production guard to answer before auth", line)
		}
	}
}

func TestDevOnlyRoutes_reachAuthOutsideProduction(t *testing.T) {
	t.Parallel()
	for _, env := range []config.Env{config.EnvLocal, config.EnvTest, config.EnvStaging} {
		h := newHarness(t)
		h.deps.Env = env
		handler := mustIdentityHandler(t, h.deps)
		for _, op := range devOnlyOperations(t) {
			resp := h.do(t, handler, op.method, op.path, http.Header{"Idempotency-Key": {"k1"}})
			if p := decodeProblem(t, resp); resp.Code != http.StatusUnauthorized || p.Code != "unauthorized" {
				t.Fatalf("%s %s under %s = %d %+v, want 401 unauthorized", op.method, op.path, env, resp.Code, p)
			}
		}
	}
}

func TestDevOnlyRoutes_productionLeavesOtherRoutesAlone(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	h.deps.Env = config.EnvProduction
	handler := mustHandler(t, h.deps, healthz{fn: func(context.Context) (platformapi.GetHealthzResponseObject, error) {
		return platformapi.GetHealthz200TextResponse("ok\n"), nil
	}})
	if resp := h.do(t, handler, http.MethodGet, "/healthz", nil); resp.Code != http.StatusOK {
		t.Fatalf("GET /healthz under production = %d, want 200", resp.Code)
	}
}
