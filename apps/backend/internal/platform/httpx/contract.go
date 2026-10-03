package httpx

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/getkin/kin-openapi/openapi3filter"
	"github.com/getkin/kin-openapi/routers"
	"github.com/getkin/kin-openapi/routers/legacy"

	"github.com/monaco/monaco/apps/backend/internal/errs"
)

type Contract struct {
	doc     *openapi3.T
	router  routers.Router
	options *openapi3filter.Options
}

type routeKey struct{}

type resolved struct {
	route   *routers.Route
	params  map[string]string
	maxBody int64
}

func LoadContract(spec []byte) (*Contract, error) {
	doc, err := openapi3.NewLoader().LoadFromData(spec)
	if err != nil {
		return nil, errs.Wrap(err, errs.CodeInvalidInput, "httpx.LoadContract")
	}
	doc.Servers = nil
	router, err := legacy.NewRouter(doc)
	if err != nil {
		return nil, errs.Wrap(err, errs.CodeInvalidInput, "httpx.LoadContract")
	}
	options := &openapi3filter.Options{AuthenticationFunc: openapi3filter.NoopAuthenticationFunc, MultiError: true}
	return &Contract{doc: doc, router: router, options: options}, nil
}

func (c *Contract) Document() *openapi3.T { return c.doc }

func (c *Contract) resolve(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		route, params, err := c.router.FindRoute(r)
		if err != nil {
			Problem(w, r, errs.Wrap(err, errs.CodeInternal, "httpx.resolve"))
			return
		}
		maxBody, err := bodyLimit(route.Operation)
		if err != nil {
			Problem(w, r, err)
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), routeKey{}, resolved{route, params, maxBody})))
	})
}

func bodyLimit(operation *openapi3.Operation) (int64, error) {
	raw, ok := operation.Extensions["x-max-body-bytes"]
	if !ok {
		return 0, nil
	}
	encoded, _ := json.Marshal(raw)
	var limit int64
	if err := json.Unmarshal(encoded, &limit); err != nil || limit <= 0 {
		return 0, errs.New(errs.CodeInvalidConfig, "httpx.bodyLimit")
	}
	return limit, nil
}

func (c *contract) limit(defaultLimit int64) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			res, ok := routeFrom(r.Context())
			if !ok {
				Problem(w, r, errs.New(errs.CodeInternal, "httpx.bodyLimit"))
				return
			}
			limit := defaultLimit
			if res.maxBody > 0 {
				limit = res.maxBody
			}
			r.Body = http.MaxBytesReader(w, r.Body, limit)
			next.ServeHTTP(w, r)
		})
	}
}

func routeFrom(ctx context.Context) (resolved, bool) {
	res, ok := ctx.Value(routeKey{}).(resolved)
	return res, ok
}

func (c *Contract) validate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		res, ok := routeFrom(r.Context())
		if !ok {
			Problem(w, r, errs.New(errs.CodeInternal, "httpx.validate"))
			return
		}
		err := openapi3filter.ValidateRequest(r.Context(), &openapi3filter.RequestValidationInput{
			Request: r, PathParams: res.params, Route: res.route, Options: c.options,
		})
		if err != nil {
			invalidRequest(w, r, err)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func requiresAuth(res resolved) bool {
	security := res.route.Spec.Security
	if res.route.Operation.Security != nil {
		security = *res.route.Operation.Security
	}
	return len(security) > 0
}
