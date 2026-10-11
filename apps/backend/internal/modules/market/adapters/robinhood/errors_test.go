package robinhood_test

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/market/adapters/robinhood"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpclient"
)

func serving(t *testing.T, body string) *robinhood.Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return robinhood.New(httpclient.New("robinhood", httpclient.WithBaseURL(srv.URL),
		httpclient.WithTimeout(10*time.Second)))
}

func TestListings_refusesABodyOrAddressItCannotRead(t *testing.T) {
	t.Parallel()
	for name, body := range map[string]string{
		"bad json": `{"assets":`,
		"bad address": `{"assets":[{"tokenSymbol":"AAPL","deployments":[{"contractAddress":"0x12","chainId":4663}],` +
			`"currentMultiplier":"1"}]}`,
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if _, err := serving(t, body).Listings(t.Context()); errs.CodeOf(err) != errs.CodeDecodeFailed {
				t.Fatalf("err = %v, want decode_failed", err)
			}
		})
	}
}

func TestListings_skipsDeploymentsOnOtherChains(t *testing.T) {
	t.Parallel()
	body := `{"assets":[{"tokenSymbol":"AAPL","deployments":[{"contractAddress":"0x12","chainId":1}],` +
		`"currentMultiplier":"1"}]}`
	batch, err := serving(t, body).Listings(t.Context())
	if err != nil || len(batch.Listings) != 0 {
		t.Fatalf("batch = %+v, %v, want no listings", batch, err)
	}
}

func TestListings_failsWhenRobinhoodIsUnreachable(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.NotFoundHandler())
	srv.Close()
	c := robinhood.New(httpclient.New("robinhood", httpclient.WithBaseURL(srv.URL),
		httpclient.WithTimeout(time.Second)))
	if _, err := c.Listings(t.Context()); err == nil {
		t.Fatal("an unreachable API returned listings")
	}
}

func TestListings_failsWhenRobinhoodRefuses(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	t.Cleanup(srv.Close)
	c := robinhood.New(httpclient.New("robinhood", httpclient.WithBaseURL(srv.URL),
		httpclient.WithTimeout(time.Second)))
	if _, err := c.Listings(t.Context()); errs.CodeOf(err) != errs.CodeUpstreamUnavailable {
		t.Fatalf("err = %v, want upstream_unavailable", err)
	}
}
