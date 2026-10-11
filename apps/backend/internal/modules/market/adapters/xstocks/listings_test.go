package xstocks_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/modules/market/adapters/xstocks"
	"github.com/monaco/monaco/apps/backend/internal/modules/market/app"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/config"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpclient"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

const (
	aaplOnSolana      = "XsbEhLAtcf6HdfpFZ5xEMdqW8nfAvcsP5bdudRLJzJp"
	aaplOnArbitrum    = "0x9d275685dc284c8eb1c79f6aba7a63dc75ec890a"
	aaplWrapperOnEVMs = "0x943bf64d566c32a2bcd41ac92fb63c111cc9de8f"
)

func fixture(t *testing.T, path string) []byte {
	t.Helper()
	body, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		t.Fatal(err)
	}
	return body
}

func TestListings_keepsSolanaEthereumAndArbitrumDeploymentsAcrossPages(t *testing.T) {
	t.Parallel()
	pages := map[string][]byte{
		"0": fixture(t, "testdata/assets_page0.json"),
		"1": fixture(t, "testdata/assets_page1.json"),
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v2/public/assets", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(pages[r.URL.Query().Get("page")])
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	feed := xstocks.ListingFeed{Client: xstocks.New(httpclient.New("xstocks", httpclient.WithBaseURL(srv.URL),
		httpclient.WithTimeout(10*time.Second)))}

	batch, err := feed.Listings(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if feed.Issuer() != "xstocks" || batch.Issuer != "xstocks" || len(batch.Listings) != 9 || len(batch.Missing) != 0 {
		t.Fatalf("batch = %+v", batch)
	}
	eight := uint8(8)
	base := app.IssuerListing{Issuer: "xstocks", Ticker: "AAPL", Symbol: "AAPLx", Tradable: true}
	sol, arb := base, base
	sol.Network, sol.TokenAddress, sol.Decimals = chain.NetworkSolana, aaplOnSolana, &eight
	arb.Network, arb.ChainID, arb.TokenAddress = chain.NetworkArbitrum, 42161, aaplOnArbitrum
	arb.WrapperAddress = aaplWrapperOnEVMs
	for _, want := range []app.IssuerListing{sol, arb} {
		if !slices.ContainsFunc(batch.Listings, func(l app.IssuerListing) bool { return reflect.DeepEqual(l, want) }) {
			t.Errorf("AAPLx on %s: got %+v, want %+v", want.Network, batch.Listings, want)
		}
	}
}

func servedListings(t *testing.T, status int, body string, logs *testkit.Logs) (app.ListingBatch, error) {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v2/public/assets", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	feed := xstocks.ListingFeed{Client: xstocks.New(httpclient.New("xstocks", httpclient.WithBaseURL(srv.URL),
		httpclient.WithTimeout(10*time.Second)))}
	ctx := observability.WithLogger(t.Context(), observability.NewLogger(config.Config{Env: config.EnvTest}, logs))
	return feed.Listings(ctx)
}

func TestListings_skipsAnUnreadableDeploymentAndAlertsOnlyWhenItTrades(t *testing.T) {
	t.Parallel()
	node := func(symbol string, halted bool, deployment string) string {
		return `{"symbol":"` + symbol + `","isTradingHalted":` + strconv.FormatBool(halted) +
			`,"deployments":[` + deployment + `]}`
	}
	arbitrum := `{"address":"` + aaplOnArbitrum + `","network":"Arbitrum"}`
	body := `{"nodes":[` + strings.Join([]string{
		node("Ax", false, `{"address":"not-a-mint","network":"Solana"}`),
		node("Bx", false, `{"address":"0x12","network":"Ethereum"}`),
		node("Cx", false, `{"address":"`+aaplOnArbitrum+`","network":"Arbitrum","wrapperAddressV2":"0x12"}`),
		node("Dx", true, `{"address":"0x12","network":"Ethereum"}`),
		node("Ex", true, arbitrum),
		node("Fx", false, arbitrum),
	}, ",") + `],"page":{"currentPage":0,"hasNextPage":false}}`
	var logs testkit.Logs
	batch, err := servedListings(t, http.StatusOK, body, &logs)
	if err != nil {
		t.Fatal(err)
	}
	kept := make([]string, 0, len(batch.Listings))
	for _, l := range batch.Listings {
		kept = append(kept, fmt.Sprint(l.Symbol, " ", l.Tradable))
	}
	if want := []string{"Ex false", "Fx true"}; !slices.Equal(kept, want) {
		t.Errorf("kept = %q, want %q", kept, want)
	}
	alerts := logs.Lines("market.listings.asset_skipped", "level", "issuer", "symbol", "reason")
	want := []string{
		"WARN xstocks Ax bad address", "WARN xstocks Bx bad address", "WARN xstocks Cx bad wrapper address",
	}
	if !slices.Equal(alerts, want) {
		t.Errorf("alerts = %q, want %q", alerts, want)
	}
}

func TestListings_failsWhenXStocksIsDown(t *testing.T) {
	t.Parallel()
	if _, err := servedListings(t, http.StatusBadGateway, `{}`, &testkit.Logs{}); err == nil {
		t.Fatal("a failed page returned listings")
	}
}
