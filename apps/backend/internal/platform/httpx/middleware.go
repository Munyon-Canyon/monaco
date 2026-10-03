package httpx

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"regexp"
	"runtime/debug"
	"strings"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/platform/auth"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/config"
	"github.com/monaco/monaco/apps/backend/internal/platform/faultpoint"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
)

const (
	RequestIDHeader = "X-Request-Id"
	FlowHeader      = "X-Monaco-Flow"
)

type Deps struct {
	Logger        *slog.Logger
	Env           config.Env
	Tracer        trace.TracerProvider
	Clock         clock.Clock
	IDs           ids.Generator
	MaxBodyBytes  int64
	Idempotency   IdempotencyStore
	Verifier      auth.TokenVerifier
	AdminVerifier auth.TokenVerifier
	RateLimit     func(http.Handler) http.Handler
	WebOrigins    []string
}

func (d Deps) wrap(next http.Handler) http.Handler {
	return d.wrapBody(next, true)
}

func (d Deps) wrapContract(next http.Handler) http.Handler {
	return d.wrapBody(next, false)
}

func (d Deps) wrapBody(next http.Handler, limitBody bool) http.Handler {
	tracer := d.Tracer.Tracer("github.com/monaco/monaco/apps/backend/internal/platform/httpx")
	acceptableID := regexp.MustCompile(`^[A-Za-z0-9._:-]{1,128}$`)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := d.Clock.Now()
		ctx := propagation.TraceContext{}.Extract(r.Context(), propagation.HeaderCarrier(r.Header))
		ctx, span := tracer.Start(ctx, r.Method, trace.WithSpanKind(trace.SpanKindServer))
		defer span.End()
		id := r.Header.Get(RequestIDHeader)
		if !acceptableID.MatchString(id) {
			id = d.IDs.NewV7().String()
		}
		w.Header().Set(RequestIDHeader, id)
		rec := &recorder{ResponseWriter: w}
		ctx = observability.WithLogger(observability.WithRequestID(ctx, id), d.Logger)
		ctx = faultpoint.WithFlow(ctx, r.Header.Get(FlowHeader))
		ctx, actor := withActorSlot(ctx)
		req := r.WithContext(ctx)
		if limitBody {
			req.Body = http.MaxBytesReader(w, r.Body, d.MaxBodyBytes)
		}
		serveRecovered(ctx, next, rec, req)
		status := rec.statusOr200()
		route := routeOf(req.Pattern)
		span.SetName(r.Method + " " + route)
		span.SetAttributes(attribute.String("http.route", route), attribute.Int("http.response.status_code", status))
		if status >= http.StatusInternalServerError {
			span.SetStatus(codes.Error, http.StatusText(status))
		}
		observability.Info(actor.apply(ctx), observability.HTTPRequest, slog.String("method", r.Method),
			slog.String("route", route), slog.Int("status", status),
			slog.Int64("duration_ms", d.Clock.Now().Sub(start).Milliseconds()))
	})
}

func routeOf(pattern string) string {
	if _, path, ok := strings.Cut(pattern, " "); ok {
		return path
	}
	return pattern
}

func serveRecovered(ctx context.Context, next http.Handler, w *recorder, r *http.Request) {
	defer func() {
		v := recover()
		if v == nil {
			return
		}
		if err, ok := v.(error); ok && errors.Is(err, http.ErrAbortHandler) {
			panic(v)
		}
		code := errs.CodePanic
		op := "httpx.recover"
		attrs := []slog.Attr{slog.String("panic", fmt.Sprint(v)), slog.String("stack", string(debug.Stack()))}
		if faultpoint.IsCrash(v) {
			code = errs.CodeFaultpoint
			op = "faultpoint: crash at " + string(v.(faultpoint.Crash).Name)
			attrs = nil
		}
		err := errs.New(code, op, attrs...)
		if w.status != 0 {
			logProblem(ctx, err, code, errs.HTTPStatus(errs.KindOf(code)))
			return
		}
		Problem(w, r, err)
	}()
	next.ServeHTTP(w, r)
}

type recorder struct {
	http.ResponseWriter
	status int
}

func (r *recorder) WriteHeader(status int) {
	if r.status == 0 {
		r.status = status
	}
	r.ResponseWriter.WriteHeader(status)
}

func (r *recorder) Write(b []byte) (int, error) {
	if r.status == 0 {
		r.status = http.StatusOK
	}
	n, err := r.ResponseWriter.Write(b)
	if err != nil {
		return n, errs.Wrap(err, errs.CodeClientClosed, "httpx.recorder.Write")
	}
	return n, nil
}

func (r *recorder) Unwrap() http.ResponseWriter { return r.ResponseWriter }

func (r *recorder) statusOr200() int {
	if r.status == 0 {
		return http.StatusOK
	}
	return r.status
}
