package xstocks_test

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/market/adapters/xstocks"
	"github.com/monaco/monaco/apps/backend/internal/modules/market/app"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpclient"
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

func servedListings(t *testing.T, status int, body string) (app.ListingBatch, error) {
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
	return feed.Listings(t.Context())
}

func TestListings_refusesADeploymentItCannotParse(t *testing.T) {
	t.Parallel()
	node := func(deployment string) string {
		return `{"nodes":[{"symbol":"AAPLx","name":"Apple xStock","isTradingHalted":false,"deployments":[` +
			deployment + `]}],"page":{"currentPage":0,"hasNextPage":false}}`
	}
	for name, deployment := range map[string]string{
		"bad solana":  `{"address":"not-a-mint","network":"Solana"}`,
		"bad evm":     `{"address":"0x12","network":"Ethereum"}`,
		"bad wrapper": `{"address":"` + aaplOnArbitrum + `","network":"Arbitrum","wrapperAddressV2":"0x12"}`,
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if _, err := servedListings(t, http.StatusOK, node(deployment)); errs.CodeOf(err) != errs.CodeDecodeFailed {
				t.Fatalf("err = %v, want decode_failed", err)
			}
		})
	}
}

func TestListings_failsWhenXStocksIsDown(t *testing.T) {
	t.Parallel()
	if _, err := servedListings(t, http.StatusBadGateway, `{}`); err == nil {
		t.Fatal("a failed page returned listings")
	}
}
