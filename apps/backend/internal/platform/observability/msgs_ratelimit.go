package observability

var RateLimitStoreFailed = Msg{
	Name: "ratelimit.store_failed", Required: []string{"operation", "scope", "code", "err"},
}
