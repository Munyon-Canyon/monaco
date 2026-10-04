package httpx

import (
	"net/http"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/platform/config"
)

const devOnlyExtension = "x-dev-only"

func devOnly(extensions map[string]any) bool {
	only, ok := extensions[devOnlyExtension].(bool)
	return ok && only
}

func refuseDevOnly(env config.Env) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		if env != config.EnvProduction {
			return next
		}
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if res, ok := routeFrom(r.Context()); ok && devOnly(res.route.Operation.Extensions) {
				Problem(w, r, errs.New(errs.CodeNotFound, "httpx.refuseDevOnly"))
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
