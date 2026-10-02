package httpx

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/platform/config"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpx/api"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpx/sse"
)

func Handler(d Deps, ssi api.StrictServerInterface, spec []byte) (http.Handler, error) {
	return handler(d, ssi, spec, nil)
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
		Middlewares:      middlewares(d, c),
	})
	return d.wrap(mux), nil
}

func middlewares(d Deps, c *contract) []api.MiddlewareFunc {
	mws := []api.MiddlewareFunc{Idempotency(d.Idempotency), c.validate}
	if d.RateLimit != nil {
		mws = append(mws, d.RateLimit)
	}
	return append(mws, Auth(d.Verifier), c.resolve)
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

func Serve(ctx context.Context, ln net.Listener, srv *http.Server, shutdownTimeout time.Duration) error {
	shutdown := make(chan error, 1)
	stop := context.AfterFunc(ctx, func() {
		shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), shutdownTimeout)
		defer cancel()
		shutdown <- srv.Shutdown(shutdownCtx)
	})
	defer stop()
	if err := srv.Serve(ln); !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("serve: %w", err)
	}
	if err := <-shutdown; err != nil {
		return fmt.Errorf("shutdown: %w", err)
	}
	return nil
}

type Routes struct {
	Health
	sse.Stream
	IdentityRoutes
	SystemRoutes
}

type IdentityRoutes interface {
	PostAuthSession(context.Context, api.PostAuthSessionRequestObject) (api.PostAuthSessionResponseObject, error)
	GetMe(context.Context, api.GetMeRequestObject) (api.GetMeResponseObject, error)
	GetHandleAvailability(
		context.Context, api.GetHandleAvailabilityRequestObject,
	) (api.GetHandleAvailabilityResponseObject, error)
}

type SystemRoutes interface {
	PostSystemPing(context.Context, api.PostSystemPingRequestObject) (api.PostSystemPingResponseObject, error)
	GetSystemPing(context.Context, api.GetSystemPingRequestObject) (api.GetSystemPingResponseObject, error)
}

var _ api.StrictServerInterface = Routes{}

type Health struct{}

func (Health) GetHealthz(context.Context, api.GetHealthzRequestObject) (api.GetHealthzResponseObject, error) {
	return api.GetHealthz200TextResponse("ok\n"), nil
}
