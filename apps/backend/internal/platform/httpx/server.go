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
	"github.com/monaco/monaco/apps/backend/internal/platform/httpx/api/platformapi"
)

func Handler(d Deps, mount func(api.Mount), spec []byte) (http.Handler, error) {
	c, err := LoadContract(spec)
	if err != nil {
		return nil, err
	}
	return HandlerFor(d, mount, c)
}

func HandlerFor(d Deps, mount func(api.Mount), c *Contract) (http.Handler, error) {
	if d.Idempotency == nil || d.Verifier == nil {
		return nil, errs.New(
			errs.CodeInternal,
			"httpx.Handler",
			slog.String("missing", "Deps.Idempotency or Deps.Verifier"),
		)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		Problem(w, r, errs.New(errs.CodeNotFound, "httpx.route"))
	})
	mount(api.Mount{Mux: mux, Middlewares: middlewares(d, c), InvalidRequest: invalidRequest, Problem: Problem})
	return d.wrapContract(mux), nil
}

func middlewares(d Deps, c *Contract) []func(http.Handler) http.Handler {
	mws := []func(http.Handler) http.Handler{Idempotency(d.Idempotency), c.validate}
	if d.RateLimit != nil {
		mws = append(mws, d.RateLimit)
	}
	return append(mws, Auth(d.Verifier), c.limit(d.MaxBodyBytes), refuseDevOnly(d.Env), c.resolve)
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

type Health struct{}

func (Health) GetHealthz(
	context.Context, platformapi.GetHealthzRequestObject,
) (platformapi.GetHealthzResponseObject, error) {
	return platformapi.GetHealthz200TextResponse("ok\n"), nil
}
