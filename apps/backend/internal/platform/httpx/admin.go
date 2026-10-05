package httpx

import (
	"log/slog"
	"net/http"
	"strings"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/admin/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/auth"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
)

const adminRoleExtension = "x-admin-role"

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
			observability.Info(r.Context(), observability.AdminRequest, slog.String("admin_id", actor.ID),
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

func isAdminRoute(res resolved) bool {
	return strings.HasPrefix(res.route.Path, "/v1/admin/")
}

func adminRole(extensions map[string]any) (domain.Role, bool) {
	raw, ok := extensions[adminRoleExtension].(string)
	role := domain.Role(raw)
	return role, ok && role.Allows(domain.RoleViewer)
}
