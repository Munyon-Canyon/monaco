package observability

var (
	HTTPRetry = Msg{
		Name:     "httpclient.retry",
		Required: []string{"upstream", "attempt", "status", "delay"},
	}
	HTTPRequest = Msg{
		Name:     "http.request",
		Required: []string{"method", "route", "status", "duration_ms"},
	}
	HTTPProblem                = Msg{Name: "http.problem", Required: []string{"code", "status", "err", "alert"}}
	HTTPIdempotencyReplayed    = Msg{Name: "http.idempotency.replayed", Required: []string{"idempotency_key", "status"}}
	HTTPIdempotencyReleased    = Msg{Name: "http.idempotency.released", Required: []string{"idempotency_key", "status"}}
	HTTPIdempotencyStoreFailed = Msg{
		Name: "http.idempotency.store_failed", Required: []string{"idempotency_key", "status", "err"},
	}
	HTTPAuthRestricted = Msg{Name: "httpx.auth.restricted", Required: []string{"standing", "op", "code"}}
)
