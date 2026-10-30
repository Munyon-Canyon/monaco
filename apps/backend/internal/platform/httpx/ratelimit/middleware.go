package ratelimit

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpx"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability/boundary"
)

type ActorKey func(*http.Request) (string, bool)

type guard struct {
	limiter           *Limiter
	actorKey          ActorKey
	trustProxyHeaders bool
}

func Middleware(
	l *Limiter, policies Policies, actorKey ActorKey, trustProxyHeaders bool,
) func(http.Handler) http.Handler {
	g := guard{limiter: l, actorKey: actorKey, trustProxyHeaders: trustProxyHeaders}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if rt, ok := policies.routes[r.Pattern]; ok && !g.allow(w, r, rt) {
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func (g guard) allow(w http.ResponseWriter, r *http.Request, rt route) bool {
	for _, lim := range rt.limits {
		id, kind, ok := g.id(r, lim.scope)
		if !ok {
			continue
		}
		d, err := g.limiter.Take(r.Context(), "op:"+rt.operation+":"+string(lim.scope)+":"+id, lim.policy, 1)
		if err != nil {
			g.limiter.failOpen(r.Context(), rt.operation, lim.scope, kind, err)
			return true
		}
		if !d.Allowed {
			g.limiter.refuse(w, r, rt.operation, lim.scope, kind, d.RetryAfter)
			return false
		}
	}
	return true
}

func (g guard) id(r *http.Request, s scope) (id, keyKind string, ok bool) {
	if s == scopeActor {
		id, ok = g.actorKey(r)
		keyKind = "unknown"
		if kind, _, found := strings.Cut(id, ":"); found {
			keyKind = kind
		}
		return id, keyKind, ok
	}
	return clientIP(r, g.trustProxyHeaders), "ip", true
}

func clientIP(r *http.Request, trustProxyHeaders bool) string {
	if trustProxyHeaders {
		entries := strings.Split(strings.Join(r.Header.Values("X-Forwarded-For"), ","), ",")
		if last := strings.TrimSpace(entries[len(entries)-1]); last != "" {
			return last
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func (l *Limiter) failOpen(ctx context.Context, operation string, s scope, keyKind string, err error) {
	if errors.Is(err, context.Canceled) || errors.Is(ctx.Err(), context.Canceled) {
		return
	}
	l.storeErrors.Add(ctx, 1)
	boundary.Warn(ctx, observability.RateLimitStoreFailed, slog.String("operation", operation),
		slog.String("scope", string(s)), slog.String("key_kind", keyKind),
		slog.String("code", string(errs.CodeOf(err))), slog.Any("err", err))
}

func (l *Limiter) refuse(
	w http.ResponseWriter,
	r *http.Request,
	operation string,
	s scope,
	keyKind string,
	retryAfter time.Duration,
) {
	l.rejected.Add(r.Context(), 1, metric.WithAttributes(
		attribute.String("operation", operation), attribute.String("scope", string(s))))
	seconds := max(int64((retryAfter+time.Second-1)/time.Second), 1)
	w.Header().Set("Retry-After", strconv.FormatInt(seconds, 10))
	httpx.Problem(w, r, errs.New(errs.CodeRateLimited, "ratelimit.Middleware",
		slog.String("operation", operation), slog.String("scope", string(s)), slog.String("key_kind", keyKind),
		slog.Int64("retry_after_s", seconds)))
}
