package httpx

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"regexp"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"

	openapi "github.com/monaco/monaco/apps/backend/api"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpx/api"
)

type operation struct{ method, path string }

func specOperations(tb testing.TB) []operation {
	tb.Helper()
	doc, err := openapi3.NewLoader().LoadFromData(openapi.Spec)
	if err != nil {
		tb.Fatal(err)
	}
	param := regexp.MustCompile(`\{[^}]+\}`)
	var ops []operation
	for path, item := range doc.Paths.Map() {
		for method := range item.Operations() {
			ops = append(ops, operation{method, param.ReplaceAllString(path, "p")})
		}
	}
	return ops
}

type unimplemented struct{ api.StrictServerInterface }

func decodedOK(api.StrictHandlerFunc, string) api.StrictHandlerFunc {
	return func(context.Context, http.ResponseWriter, *http.Request, any) (any, error) { return nil, nil }
}

func FuzzRequestBodies(f *testing.F) {
	ops := specOperations(f)
	if len(ops) == 0 {
		f.Fatal("api/openapi.yaml declares no operations")
	}
	for _, seed := range []string{"", "{}", "{", "null", "[]", `{"a":` + "\x00" + `}`, "\xff\xfe", `{"amount":-1e309}`} {
		f.Add(uint8(0), []byte(seed))
	}
	f.Fuzz(func(t *testing.T, pick uint8, body []byte) {
		op := ops[int(pick)%len(ops)]
		h := newHarness(t)
		h.deps.MaxBodyBytes = 1 << 10
		req := httptest.NewRequestWithContext(t.Context(), op.method, op.path, bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		served, err := handler(h.deps, unimplemented{}, openapi.Spec, []api.StrictMiddlewareFunc{decodedOK})
		if err != nil {
			t.Fatal(err)
		}
		served.ServeHTTP(rec, req)
		if rec.Code >= 200 && rec.Code < 300 {
			return
		}
		var p api.Problem
		if err := json.Unmarshal(rec.Body.Bytes(), &p); err != nil || rec.Code != http.StatusBadRequest ||
			p.Code != api.InvalidInput {
			t.Fatalf("%s %s with %q = %d %s, want 2xx or 400 invalid_input", op.method, op.path, body, rec.Code,
				rec.Body)
		}
	})
}
