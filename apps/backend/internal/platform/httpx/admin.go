package httpx

import (
	"context"
	"log/slog"
	"net/http"
	"strings"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/admin/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/auth"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
)

const (
	adminRoleExtension = "x-admin-role"
	serviceReadPrefix  = "/v1/admin/dashboards/"
)

type ServiceTokenVerifier interface {
	VerifyServiceToken(ctx context.Context, raw string) (name string, err error)
}

func Admin(v auth.TokenVerifier) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			res, ok := routeFrom(r.Context())
			if !ok || !isAdminRoute(res) {
				next.ServeHTTP(w, r)
				return
			}
			actor, err := adminActor(r, v, res)
			if err != nil {
				Problem(w, r, err)
				return
			}
			rec := &recorder{ResponseWriter: w}
			next.ServeHTTP(rec, r.WithContext(withActor(r.Context(), actor)))
			observability.Info(r.Context(), observability.AdminRequest, slog.String("admin_id", adminID(actor)),
				slog.String("role", actor.Role), slog.String("op", res.route.Operation.OperationID),
				slog.Int("status", rec.statusOr200()))
		})
	}
}

func adminActor(r *http.Request, v auth.TokenVerifier, res resolved) (auth.Actor, error) {
	const op = "httpx.Admin"
	if v == nil {
		return auth.Actor{}, errs.New(errs.CodeAdminForbidden, op)
	}
	required, ok := adminRole(res.route.Operation.Extensions)
	if !ok {
		return auth.Actor{}, errs.New(errs.CodeInternal, op)
	}
	raw, ok := BearerToken(r.Header.Get("Authorization"))
	if !ok {
		return auth.Actor{}, errs.New(errs.CodeUnauthorized, op)
	}
	if domain.IsServiceToken(raw) {
		return serviceActor(r, v, raw, res, required)
	}
	actor, err := v.Verify(r.Context(), raw)
	if err != nil {
		if errs.CodeOf(err) == errs.CodeAdminForbidden {
			return auth.Actor{}, err
		}
		return auth.Actor{}, verifyProblem(err, op)
	}
	if !domain.Role(actor.Role).Allows(required) {
		return auth.Actor{}, errs.New(errs.CodeAdminForbidden, op)
	}
	return actor, nil
}

func serviceActor(
	r *http.Request, v auth.TokenVerifier, raw string, res resolved, required domain.Role,
) (auth.Actor, error) {
	const op = "httpx.Admin.serviceActor"
	sv, ok := v.(ServiceTokenVerifier)
	if !ok {
		return auth.Actor{}, errs.New(errs.CodeUnauthorized, op)
	}
	name, err := sv.VerifyServiceToken(r.Context(), raw)
	if err != nil {
		return auth.Actor{}, verifyProblem(err, op)
	}
	if !serviceMayRead(r.Method, res.route.Path, required) {
		return auth.Actor{}, errs.New(errs.CodeAdminForbidden, op, slog.String("service", name))
	}
	return auth.Actor{Kind: auth.ActorService, ID: name, Role: string(domain.RoleViewer)}, nil
}

func serviceMayRead(method, path string, required domain.Role) bool {
	return method == http.MethodGet && required == domain.RoleViewer && strings.HasPrefix(path, serviceReadPrefix)
}

func adminID(a auth.Actor) string {
	if a.Kind == auth.ActorService {
		return a.Key()
	}
	return a.ID
}

func isAdminRoute(res resolved) bool {
	return strings.HasPrefix(res.route.Path, "/v1/admin/")
}

func adminRole(extensions map[string]any) (domain.Role, bool) {
	raw, ok := extensions[adminRoleExtension].(string)
	role := domain.Role(raw)
	return role, ok && role.Allows(domain.RoleViewer)
}
