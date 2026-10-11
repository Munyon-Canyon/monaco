package ondo_test

import (
	"math/big"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/market/adapters/ondo"
	"github.com/monaco/monaco/apps/backend/internal/modules/market/app"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpclient"
)

type page struct {
	status  int
	fixture string
}

func recorded(t *testing.T, tslaon page) *ondo.Client {
	t.Helper()
	routes := map[string]page{
		"/api/v2/assets": {http.StatusOK, "assets.json"},
		"/assets/aaplon": {http.StatusOK, "aaplon.html"},
		"/assets/tslaon": tslaon,
	}
	mux := http.NewServeMux()
	for path, p := range routes {
		body, err := os.ReadFile("testdata/" + p.fixture)
		if err != nil {
			t.Fatal(err)
		}
		mux.HandleFunc("GET "+path, func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "text/html")
			w.WriteHeader(p.status)
			_, _ = w.Write(body)
		})
	}
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return ondo.New(httpclient.New("ondo", httpclient.WithBaseURL(srv.URL),
		httpclient.WithTimeout(10*time.Second)))
}

const (
	aaplOnEthereum = "0x14c3abf95cb9c93a8b82c1cdcb76d72cb87b2d4c"
	aaplOnSolana   = "123mYEnRLM2LLYsJW3K6oyYh8uP1fngj732iG638ondo"
)

func TestListings_readsEthereumAndSolanaFromEachAssetPage(t *testing.T) {
	t.Parallel()
	c := recorded(t, page{http.StatusOK, "tslaon.html"})
	batch, err := c.Listings(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if c.Issuer() != "ondo" || batch.Issuer != "ondo" || len(batch.Listings) != 4 || len(batch.Missing) != 0 {
		t.Fatalf("batch = %+v", batch)
	}
	shares, _ := new(big.Rat).SetString("1.003376073740221058")
	eighteen, nine := uint8(18), uint8(9)
	base := app.IssuerListing{Issuer: "ondo", Ticker: "AAPL", Symbol: "AAPLon", SharesPerToken: shares, Tradable: true}
	eth, sol := base, base
	eth.Network, eth.ChainID, eth.TokenAddress, eth.Decimals = chain.NetworkEthereum, 1, aaplOnEthereum, &eighteen
	sol.Network, sol.TokenAddress, sol.Decimals = chain.NetworkSolana, aaplOnSolana, &nine
	for _, want := range []app.IssuerListing{eth, sol} {
		if !slices.ContainsFunc(batch.Listings, func(l app.IssuerListing) bool { return reflect.DeepEqual(l, want) }) {
			t.Errorf("AAPLon on %s: got %+v, want %+v", want.Network, batch.Listings, want)
		}
	}
}

func TestListings_failsClosedOnAPageWithoutSupportedNetworks(t *testing.T) {
	t.Parallel()
	for name, tslaon := range map[string]page{
		"block removed": {http.StatusOK, "tslaon_without_networks.html"},
		"page gone":     {http.StatusNotFound, "tslaon.html"},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			batch, err := recorded(t, tslaon).Listings(t.Context())
			if err != nil {
				t.Fatal(err)
			}
			if !slices.Equal(batch.Missing, []string{"TSLAon"}) || len(batch.Listings) != 2 {
				t.Fatalf("batch = %+v, want AAPLon listed and TSLAon missing", batch)
			}
			for _, l := range batch.Listings {
				if l.Symbol != "AAPLon" {
					t.Fatalf("listed %+v from a page without networks", l)
				}
			}
		})
	}
}

func TestListings_failsWhenAnAssetPageErrors(t *testing.T) {
	t.Parallel()
	_, err := recorded(t, page{http.StatusInternalServerError, "tslaon.html"}).Listings(t.Context())
	if errs.CodeOf(err) != errs.CodeUpstreamUnavailable {
		t.Fatalf("err = %v, want upstream_unavailable", err)
	}
}
