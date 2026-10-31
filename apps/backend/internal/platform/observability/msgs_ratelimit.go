package observability

var RateLimitStoreFailed = Msg{
	Name: "ratelimit.store_failed", Required: []string{"operation", "scope", "code", "err"},
}

var RateLimitKeySource = Msg{
	Name: "ratelimit.key_source", Required: []string{"source"},
}
