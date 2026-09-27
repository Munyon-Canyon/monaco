package httpx

import (
	"net/http"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/getkin/kin-openapi/openapi3filter"
	"github.com/getkin/kin-openapi/routers/legacy"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpx/api"
)

func requestValidator(spec []byte) (api.MiddlewareFunc, error) {
	doc, err := openapi3.NewLoader().LoadFromData(spec)
	if err != nil {
		return nil, errs.Wrap(err, errs.CodeInvalidInput, "httpx.requestValidator")
	}
	doc.Servers = nil
	router, err := legacy.NewRouter(doc)
	if err != nil {
		return nil, errs.Wrap(err, errs.CodeInvalidInput, "httpx.requestValidator")
	}
	options := &openapi3filter.Options{AuthenticationFunc: openapi3filter.NoopAuthenticationFunc, MultiError: true}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			route, params, err := router.FindRoute(r)
			if err != nil {
				Problem(w, r, errs.Wrap(err, errs.CodeInternal, "httpx.validateRequest"))
				return
			}
			err = openapi3filter.ValidateRequest(r.Context(), &openapi3filter.RequestValidationInput{
				Request: r, PathParams: params, Route: route, Options: options,
			})
			if err != nil {
				invalidRequest(w, r, err)
				return
			}
			next.ServeHTTP(w, r)
		})
	}, nil
}
