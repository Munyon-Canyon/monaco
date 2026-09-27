package httpx

import (
	"context"
	"net/http"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/platform/config"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpx/api"
)

func Handler(d Deps, ssi api.StrictServerInterface) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		Problem(w, r, errs.New(errs.CodeNotFound, "httpx.route"))
	})
	strict := api.NewStrictHandlerWithOptions(ssi, nil, api.StrictHTTPServerOptions{
		RequestErrorHandlerFunc:  invalidRequest,
		ResponseErrorHandlerFunc: Problem,
	})
	api.HandlerWithOptions(strict, api.StdHTTPServerOptions{BaseRouter: mux, ErrorHandlerFunc: invalidRequest})
	return d.wrap(mux)
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
