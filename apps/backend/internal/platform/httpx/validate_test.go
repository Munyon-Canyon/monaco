package httpx

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/errs"
)

const bodySpec = `openapi: 3.1.0
info: {title: fixture, version: "1"}
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
        "204": {description: created}
`

func validatedThings(t *testing.T, h *harness) (http.Handler, *[]string) {
	t.Helper()
	validate, err := requestValidator([]byte(bodySpec))
	if err != nil {
		t.Fatal(err)
	}
	var reached []string
	mux := http.NewServeMux()
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Error(err)
		}
		reached = append(reached, string(body))
		w.WriteHeader(http.StatusNoContent)
	})
	mux.Handle("POST /v1/things", validate(next))
	mux.Handle("POST /v1/unspecified", validate(next))
	return h.deps.wrap(mux), &reached
}

func postJSON(t *testing.T, handler http.Handler, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

func TestRequestValidator_rejectsSchemaInvalidBodiesBeforeTheHandler(t *testing.T) {
	t.Parallel()
	for name, body := range map[string]string{
		"empty object":     `{}`,
		"below minimum":    `{"amount":0}`,
		"wrong type":       `{"amount":"5"}`,
		"extra field":      `{"amount":5,"admin":true}`,
		"not json":         `{"amount":`,
		"missing body":     ``,
		"array not object": `[5]`,
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			h := newHarness(t)
			handler, reached := validatedThings(t, h)
			rec := postJSON(t, handler, "/v1/things", body)
			p := decodeProblem(t, rec)
			if rec.Code != http.StatusBadRequest || p.Code != "invalid_input" ||
				p.Message != errs.Message(errs.CodeInvalidInput) {
				t.Fatalf("POST %s = %d %+v, want 400 invalid_input", body, rec.Code, p)
			}
			if len(*reached) != 0 {
				t.Fatalf("handler ran with %q", *reached)
			}
			if strings.Contains(rec.Body.String(), "amount") || strings.Contains(rec.Body.String(), "admin") {
				t.Fatalf("problem body leaks validation detail: %s", rec.Body)
			}
		})
	}
}

func TestRequestValidator_passesAValidBodyThroughIntact(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	handler, reached := validatedThings(t, h)
	rec := postJSON(t, handler, "/v1/things", `{"amount":5}`)
	if rec.Code != http.StatusNoContent || len(*reached) != 1 || (*reached)[0] != `{"amount":5}` {
		t.Fatalf("got %d, handler saw %q, want 204 and the original body", rec.Code, *reached)
	}
}

func TestRequestValidator_aRouteMissingFromTheSpecIsInternal(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	handler, reached := validatedThings(t, h)
	rec := postJSON(t, handler, "/v1/unspecified", `{}`)
	if p := decodeProblem(
		t,
		rec,
	); rec.Code != http.StatusInternalServerError || p.Code != "internal" ||
		len(*reached) != 0 {
		t.Fatalf("got %d %+v, want 500 internal before the handler", rec.Code, p)
	}
}

func TestHandler_refusesASpecItCannotLoadOrRoute(t *testing.T) {
	t.Parallel()
	for name, spec := range map[string]string{
		"not yaml":       "openapi: [",
		"invalid schema": "openapi: 3.1.0\ninfo: {title: x, version: \"1\"}\npaths:\n  x:\n    get: {responses: {\"200\": {description: ok}}}\n",
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			h, err := handler(newHarness(t).deps, unimplemented{}, []byte(spec), nil)
			if h != nil || errs.CodeOf(err) != errs.CodeInvalidInput {
				t.Fatalf("handler = %v, %v, want invalid_input and no handler", h, err)
			}
		})
	}
}
