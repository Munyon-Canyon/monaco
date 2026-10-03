package httpx

import (
	"net/http"
	"slices"
)

const (
	CORSExtension = "x-cors"
	corsWeb       = "web"
	corsHeaders   = "Authorization, Content-Type, Idempotency-Key"
	corsMaxAge    = "600"
)

func (c *Contract) cors(origins []string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")
			requested := r.Header.Get("Access-Control-Request-Method")
			preflight := r.Method == http.MethodOptions && requested != ""
			method := r.Method
			if preflight {
				method = requested
			}
			if origin == "" || !slices.Contains(origins, origin) || !c.webOperation(r, method) {
				next.ServeHTTP(w, r)
				return
			}
			h := w.Header()
			h.Set("Access-Control-Allow-Origin", origin)
			h.Add("Vary", "Origin")
			if !preflight {
				next.ServeHTTP(w, r)
				return
			}
			h.Set("Access-Control-Allow-Methods", method)
			h.Set("Access-Control-Allow-Headers", corsHeaders)
			h.Set("Access-Control-Max-Age", corsMaxAge)
			w.WriteHeader(http.StatusNoContent)
		})
	}
}

func (c *Contract) webOperation(r *http.Request, method string) bool {
	probe := r.Clone(r.Context())
	probe.Method = method
	route, _, err := c.router.FindRoute(probe)
	if err != nil {
		return false
	}
	mode, ok := route.Operation.Extensions[CORSExtension].(string)
	return ok && mode == corsWeb
}
