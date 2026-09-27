package testkit

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"maps"
	"net/http"
	"net/http/httptest"
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

func HTTP(tb testing.TB, h http.Handler) http.Handler {
	tb.Helper()
	c := loadContract(tb)
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

func loadContract(tb testing.TB) contract {
	tb.Helper()
	doc, err := openapi3.NewLoader().LoadFromData(openapi.Spec)
	if err != nil {
		tb.Fatalf("load api/openapi.yaml: %v", err)
	}
	if err := doc.Validate(context.WithoutCancel(tb.Context())); err != nil {
		tb.Fatalf("api/openapi.yaml is not a valid OpenAPI document: %v", err)
	}
	doc.Servers = nil
	router, err := legacy.NewRouter(doc)
	if err != nil {
		tb.Fatalf("route api/openapi.yaml: %v", err)
	}
	return contract{doc: doc, router: router}
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
