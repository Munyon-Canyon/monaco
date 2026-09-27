package httpx

import (
	"context"
	"log/slog"
	"net/http"

	openapi "github.com/monaco/monaco/apps/backend/api"
	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/platform/config"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpx/api"
)

func Handler(d Deps, ssi api.StrictServerInterface) (http.Handler, error) {
	return handler(d, ssi, openapi.Spec, nil)
}

func handler(
	d Deps, ssi api.StrictServerInterface, spec []byte, mws []api.StrictMiddlewareFunc,
) (http.Handler, error) {
	if d.Idempotency == nil || d.Verifier == nil {
		return nil, errs.New(
			errs.CodeInternal,
			"httpx.Handler",
			slog.String("missing", "Deps.Idempotency or Deps.Verifier"),
		)
	}
	c, err := loadContract(spec)
	if err != nil {
		return nil, err
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		Problem(w, r, errs.New(errs.CodeNotFound, "httpx.route"))
	})
	strict := api.NewStrictHandlerWithOptions(ssi, mws, api.StrictHTTPServerOptions{
		RequestErrorHandlerFunc:  invalidRequest,
		ResponseErrorHandlerFunc: Problem,
	})
	api.HandlerWithOptions(strict, api.StdHTTPServerOptions{
		BaseRouter:       mux,
		ErrorHandlerFunc: invalidRequest,
		Middlewares:      []api.MiddlewareFunc{Idempotency(d.Idempotency), c.validate, Auth(d.Verifier), c.resolve},
	})
	return d.wrap(mux), nil
}

func invalidRequest(w http.ResponseWriter, r *http.Request, err error) {
	Problem(w, r, errs.Wrap(err, errs.CodeInvalidInput, "httpx.decodeRequest"))
}

func NewServer(h http.Handler, t config.Timeouts) *http.Server {
	return &http.Server{
		Handler:           h,
		ReadHeaderTimeout: t.HTTPServerRead,
		ReadTimeout:       t.HTTPServerRead,
		WriteTimeout:      t.HTTPServerWrite,
	}
}

type Health struct{}

func (Health) GetHealthz(context.Context, api.GetHealthzRequestObject) (api.GetHealthzResponseObject, error) {
	return api.GetHealthz200TextResponse("ok\n"), nil
}
