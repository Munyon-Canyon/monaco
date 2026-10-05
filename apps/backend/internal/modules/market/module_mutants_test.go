package market_test

import (
	"net/http"
	"testing"
	"testing/synctest"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/modules/market"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpclient"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
)

func TestModule_retriesTesseraAndPreStocksThreeTimesWithinTheBackoffCeilings(t *testing.T) {
	t.Parallel()
	for _, provider := range []string{"tessera", "prestocks"} {
		t.Run(provider, func(t *testing.T) {
			t.Parallel()
			synctest.Test(t, func(t *testing.T) {
				calls, waited, err := retryAgainst503(t, provider)
				if err == nil {
					t.Fatal("a 503 on every attempt succeeded")
				}
				if calls != 3 || waited <= 0 || waited > 750*time.Millisecond {
					t.Fatalf("%d attempts waited %v, want 3 attempts and between 0 and 250ms + 500ms of backoff",
						calls, waited)
				}
			})
		})
	}
}

func retryAgainst503(t *testing.T, provider string) (int32, time.Duration, error) {
	t.Helper()
	rt := &unavailable{}
	var client *httpclient.Client
	market.New(module.Deps{
		Config: moduleConfig(),
		HTTPClient: func(name string, opts ...httpclient.Option) *httpclient.Client {
			c := httpclient.New(name, append(opts, httpclient.WithTransport(rt))...)
			if name == provider {
				client = c
			}
			return c
		},
	}).Pollers()
	if client == nil {
		t.Fatalf("Pollers built no %s client", provider)
	}
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, "/assets", nil)
	if err != nil {
		t.Fatal(err)
	}
	start := clock.Real{}.Now()
	resp, err := client.Do(t.Context(), req)
	waited := clock.Real{}.Now().Sub(start)
	if resp != nil {
		_ = resp.Body.Close()
	}
	return rt.calls.Load(), waited, err
}
