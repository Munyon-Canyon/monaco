package httpx

import (
	"context"
	"log/slog"
	"net/http"
	"slices"
	"strings"

	"github.com/getkin/kin-openapi/openapi3"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/platform/auth"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
)

const allowRestrictedExtension = "x-allow-restricted"

func allowRestricted(extensions map[string]any) bool {
	allowed, ok := extensions[allowRestrictedExtension].(bool)
	return ok && allowed
}

func restrictedRoutes(spec []byte) ([]string, error) {
	doc, err := openapi3.NewLoader().LoadFromData(spec)
	if err != nil {
		return nil, errs.Wrap(err, errs.CodeInvalidInput, "httpx.restrictedRoutes")
	}
	var lines []string
	for path, item := range doc.Paths.Map() {
		for method, operation := range item.Operations() {
			if allowRestricted(operation.Extensions) {
				lines = append(lines, method+" "+path)
			}
		}
	}
	slices.Sort(lines)
	return lines, nil
}

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

func Auth(v auth.TokenVerifier) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler { return actorPassthrough(next, authVerify(v)(next)) }
}

func actorPassthrough(next, verified http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, ok := auth.ActorFrom(r.Context()); ok {
			next.ServeHTTP(w, r)
			return
		}
		verified.ServeHTTP(w, r)
	})
}

func authVerify(v auth.TokenVerifier) func(http.Handler) http.Handler {
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
			raw, ok := BearerToken(r.Header.Get("Authorization"))
			if !ok {
				Problem(w, r, errs.New(errs.CodeUnauthorized, op, slog.String("reason", "no_bearer_token")))
				return
			}
			actor, err := v.Verify(r.Context(), raw)
			if err != nil {
				Problem(w, r, verifyProblem(err, op))
				return
			}
			if code, blocked := standingBlock(actor.Standing, r.Method, res); blocked {
				observability.Info(r.Context(), observability.HTTPAuthRestricted,
					slog.String("standing", string(actor.Standing)),
					slog.String("op", res.route.Operation.OperationID),
					slog.String("code", string(code)),
				)
				Problem(w, r, errs.New(code, op))
				return
			}
			next.ServeHTTP(w, r.WithContext(withActor(r.Context(), actor)))
		})
	}
}

func standingBlock(standing auth.Standing, method string, res resolved) (errs.Code, bool) {
	switch standing {
	case auth.StandingActive:
		return "", false
	case auth.StandingSuspended:
		if method == http.MethodGet || method == http.MethodHead || allowRestricted(res.route.Operation.Extensions) {
			return "", false
		}
		return errs.CodeAccountSuspended, true
	case auth.StandingBanned:
		if allowRestricted(res.route.Operation.Extensions) {
			return "", false
		}
		return errs.CodeAccountBanned, true
	case auth.StandingDeleted:
		return errs.CodeAccountDeleted, true
	default:
		return "", false
	}
}

func BearerToken(header string) (string, bool) {
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
