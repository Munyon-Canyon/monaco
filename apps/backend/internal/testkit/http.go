package testkit

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/getkin/kin-openapi/openapi3filter"
	"github.com/getkin/kin-openapi/routers"
	"github.com/getkin/kin-openapi/routers/legacy"

	openapi "github.com/monaco/monaco/apps/backend/api"
)

type contract struct {
	doc    *openapi3.T
	router routers.Router
}

var parsedSpec = sync.OnceValues(func() (*openapi3.T, error) {
	doc, err := openapi3.NewLoader().LoadFromData(openapi.Spec)
	if err != nil {
		return nil, fmt.Errorf("load api/openapi.yaml: %w", err)
	}
	if err := doc.Validate(context.Background()); err != nil {
		return nil, fmt.Errorf("api/openapi.yaml is not a valid OpenAPI document: %w", err)
	}
	doc.Servers = nil
	return doc, nil
})

var contractOnce sync.Map

func contractOf(doc *openapi3.T) (contract, error) {
	build, _ := contractOnce.LoadOrStore(doc, sync.OnceValues(func() (contract, error) {
		router, err := legacy.NewRouter(doc)
		if err != nil {
			return contract{}, fmt.Errorf("route api/openapi.yaml: %w", err)
		}
		return contract{doc: doc, router: router}, nil
	}))
	return build.(func() (contract, error))()
}

func HTTP(tb testing.TB, h http.Handler) http.Handler {
	tb.Helper()
	doc, err := parsedSpec()
	if err != nil {
		tb.Fatal(err)
	}
	return HTTPAgainst(tb, doc, h)
}

func HTTPAgainst(tb testing.TB, doc *openapi3.T, h http.Handler) http.Handler {
	tb.Helper()
	c, err := contractOf(doc)
	if err != nil {
		tb.Fatal(err)
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, r)
		if v := c.violation(r, rec); v != "" {
			tb.Errorf("contract: %s %s answered %d, which the spec does not allow: %s",
				r.Method, r.URL.Path, rec.Code, v)
		}
		maps.Copy(w.Header(), rec.Header())
		w.WriteHeader(rec.Code)
		_, _ = w.Write(rec.Body.Bytes())
	})
}

func (c contract) violation(r *http.Request, rec *httptest.ResponseRecorder) string {
	route, params, err := c.router.FindRoute(r)
	if unrouted := new(routers.RouteError); errors.As(err, &unrouted) {
		return c.problemViolation(rec)
	}
	if err != nil {
		return err.Error()
	}
	err = openapi3filter.ValidateResponse(r.Context(), &openapi3filter.ResponseValidationInput{
		RequestValidationInput: &openapi3filter.RequestValidationInput{Request: r, PathParams: params, Route: route},
		Status:                 rec.Code,
		Header:                 rec.Header(),
		Body:                   io.NopCloser(bytes.NewReader(rec.Body.Bytes())),
		Options:                &openapi3filter.Options{IncludeResponseStatus: true, MultiError: true},
	})
	if err != nil {
		return err.Error()
	}
	return ""
}

func (c contract) problemViolation(rec *httptest.ResponseRecorder) string {
	if ct := rec.Header().Get("Content-Type"); ct != "application/problem+json" {
		return "an undeclared route must answer application/problem+json, got " + ct
	}
	var body any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		return err.Error()
	}
	if err := c.doc.Components.Schemas["Problem"].Value.VisitJSON(body, openapi3.EnableJSONSchema2020()); err != nil {
		return err.Error()
	}
	return ""
}
