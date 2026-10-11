package robinhood_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/market/adapters/robinhood"
	"github.com/monaco/monaco/apps/backend/internal/platform/config"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpclient"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
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

func TestListings_refusesABodyItCannotRead(t *testing.T) {
	t.Parallel()
	if _, err := serving(t, `{"assets":`).Listings(t.Context()); errs.CodeOf(err) != errs.CodeDecodeFailed {
		t.Fatalf("err = %v, want decode_failed", err)
	}
}

func TestListings_skipsAnUnreadableAssetAndAlertsOnlyWhenItIsActive(t *testing.T) {
	t.Parallel()
	const active, closed = "ASSET_STATUS_ACTIVE", "ASSET_STATUS_INACTIVE"
	asset := func(symbol, address, current, pending, status string) string {
		return `{"tokenSymbol":"` + symbol + `","deployments":[{"contractAddress":"` + address +
			`","chainId":4663}],"currentMultiplier":"` + current + `","pendingMultiplier":"` + pending +
			`","status":"` + status + `","tokenDecimals":18}`
	}
	body := `{"assets":[` + strings.Join([]string{
		asset("BAD", "0x12", "1", "", active),
		asset("ZERO", aaplOnRobinhood, "0", "", active),
		asset("SPLIT", aaplOnRobinhood, "1", "nope", active),
		asset("GONE", aaplOnRobinhood, "", "", closed),
		asset("CRM", crmOnRobinhood, "1", "", closed),
		asset("AAPL", aaplOnRobinhood, "1", "", active),
	}, ",") + `]}`
	var logs testkit.Logs
	ctx := observability.WithLogger(t.Context(), observability.NewLogger(config.Config{Env: config.EnvTest}, &logs))
	batch, err := serving(t, body).Listings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	kept := make([]string, 0, len(batch.Listings))
	for _, l := range batch.Listings {
		kept = append(kept, fmt.Sprint(l.Symbol, " ", l.Tradable))
	}
	if want := []string{"CRM false", "AAPL true"}; !slices.Equal(kept, want) {
		t.Errorf("kept = %q, want %q", kept, want)
	}
	alerts := logs.Lines("market.listings.asset_skipped", "level", "issuer", "symbol", "reason")
	want := []string{
		"WARN robinhood BAD bad address", "WARN robinhood ZERO bad multiplier",
		"WARN robinhood SPLIT bad pending multiplier",
	}
	if !slices.Equal(alerts, want) {
		t.Errorf("alerts = %q, want %q", alerts, want)
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
