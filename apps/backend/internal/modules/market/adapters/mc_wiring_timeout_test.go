package adapters

import (
	"net/http"
	"testing"
	"testing/synctest"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpclient"
)

type hangingUpstream struct{}

func (hangingUpstream) RoundTrip(req *http.Request) (*http.Response, error) {
	<-req.Context().Done()
	return nil, req.Context().Err()
}

func givesUpAfter(t *testing.T, name string, opts []httpclient.Option) time.Duration {
	t.Helper()
	var took time.Duration
	synctest.Test(t, func(t *testing.T) {
		c := httpclient.New(name, append(opts, httpclient.WithTransport(hangingUpstream{}))...)
		req, _ := http.NewRequestWithContext(t.Context(), http.MethodGet, "/v1/quote", nil)
		start := clock.Real{}.Now()
		if resp, err := c.Do(t.Context(), req); err == nil {
			_ = resp.Body.Close()
			t.Fatal("a hanging upstream answered")
		}
		took = clock.Real{}.Now().Sub(start)
	})
	return took
}

func TestRoutingQuotesUnderTheQuoteTimeoutNotTheExecutionOne(t *testing.T) {
	t.Parallel()
	deps := routingDeps("uni-key")
	deps.Config.Routing.UniswapTimeout = 7 * time.Second
	deps.Config.Routing.UniswapQuoteTimeout = 3 * time.Second
	opts := map[string][]httpclient.Option{}
	deps.HTTPClient = func(name string, o ...httpclient.Option) *httpclient.Client {
		opts[name] = o
		return httpclient.New(name, o...)
	}
	RoutingVenues(deps)
	if got := givesUpAfter(t, "uniswap", opts["uniswap"]); got != 3*time.Second {
		t.Fatalf("the routing Uniswap client gives up after %v, want MONACO_TIMEOUT_UNISWAP_QUOTE's 3s", got)
	}
	if got := routerRules(deps.Config.Routing).QuoteTimeout; got != 3*time.Second {
		t.Fatalf("each venue's budget is %v, want MONACO_TIMEOUT_UNISWAP_QUOTE's 3s", got)
	}
}
