package httpx

import (
	"context"
	"log/slog"
	"net/http"
	"strings"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/platform/auth"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpx/api"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
)

type actorSlotKey struct{}

type actorSlot struct{ key string }

func withActorSlot(ctx context.Context) (context.Context, *actorSlot) {
	slot := &actorSlot{}
	return context.WithValue(ctx, actorSlotKey{}, slot), slot
}

func (s *actorSlot) apply(ctx context.Context) context.Context {
	if s.key == "" {
		return ctx
	}
	return observability.WithActor(ctx, s.key)
}

func withActor(ctx context.Context, a auth.Actor) context.Context {
	if slot, ok := ctx.Value(actorSlotKey{}).(*actorSlot); ok {
		slot.key = a.Key()
	}
	return auth.WithActor(ctx, a)
}

func ActorKey(r *http.Request) (string, bool) {
	a, ok := auth.ActorFrom(r.Context())
	return a.Key(), ok
}

func Auth(v auth.TokenVerifier) api.MiddlewareFunc {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			const op = "httpx.Auth"
			res, ok := routeFrom(r.Context())
			if !ok {
				Problem(w, r, errs.New(errs.CodeInternal, op, slog.String("missing", "route")))
				return
			}
			if !requiresAuth(res) {
				next.ServeHTTP(w, r)
				return
			}
			raw, ok := bearerToken(r.Header.Get("Authorization"))
			if !ok {
				Problem(w, r, errs.New(errs.CodeUnauthorized, op, slog.String("reason", "no_bearer_token")))
				return
			}
			actor, err := v.Verify(r.Context(), raw)
			if err != nil {
				Problem(w, r, verifyProblem(err, op))
				return
			}
			next.ServeHTTP(w, r.WithContext(withActor(r.Context(), actor)))
		})
	}
}

func bearerToken(header string) (string, bool) {
	scheme, token, ok := strings.Cut(header, " ")
	token = strings.TrimSpace(token)
	if !ok || !strings.EqualFold(scheme, "Bearer") || token == "" {
		return "", false
	}
	return token, true
}

func verifyProblem(err error, op string) error {
	kind := errs.KindOf(errs.CodeOf(err))
	if kind == errs.KindUnauthorized || kind == errs.KindUnavailable || kind == errs.KindInternal {
		return err
	}
	return errs.Wrap(err, errs.CodeUnauthorized, op)
}
