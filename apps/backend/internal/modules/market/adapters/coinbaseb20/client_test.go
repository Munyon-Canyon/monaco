package coinbaseb20_test

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
	"github.com/monaco/monaco/apps/backend/internal/modules/market/adapters/coinbaseb20"
	"github.com/monaco/monaco/apps/backend/internal/modules/market/app"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpclient"
)

const (
	aaplOnBase  = "0xb200000000000000000000c2e324d24d7eecd1fb"
	googlOnBase = "0xb2000000000000000000002d0ba3164cc74f58b7"
)

func recorded(t *testing.T, status int) *coinbaseb20.Client {
	t.Helper()
	body, err := os.ReadFile("testdata/tokenized-stocks.json")
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/tokenized-stocks", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write(body)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return coinbaseb20.New(httpclient.New("coinbaseb20", httpclient.WithBaseURL(srv.URL),
		httpclient.WithTimeout(10*time.Second)))
}

func rat(t *testing.T, decimal string) *big.Rat {
	t.Helper()
	r, ok := new(big.Rat).SetString(decimal)
	if !ok {
		t.Fatalf("rat %q", decimal)
	}
	return r
}

func TestListings_readsEveryCoinbaseStockOnBase(t *testing.T) {
	t.Parallel()
	c := recorded(t, http.StatusOK)
	batch, err := c.Listings(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if c.Issuer() != "coinbase" || batch.Issuer != "coinbase" || len(batch.Listings) != 3 || len(batch.Missing) != 0 {
		t.Fatalf("batch = %+v", batch)
	}
	eight := uint8(8)
	base := app.IssuerListing{
		Issuer: "coinbase", Network: chain.NetworkBase, ChainID: 8453, Decimals: &eight, Tradable: true,
	}
	aapl, googl := base, base
	aapl.Ticker, aapl.Symbol, aapl.TokenAddress, aapl.SharesPerToken = "AAPL", "AAPLc", aaplOnBase, rat(t, "1")
	googl.Ticker, googl.Symbol, googl.TokenAddress = "GOOGL", "GOOGLc", googlOnBase
	googl.SharesPerToken = rat(t, "1.0003771186767842")
	for _, want := range []app.IssuerListing{aapl, googl} {
		i := slices.IndexFunc(batch.Listings, func(l app.IssuerListing) bool { return l.Symbol == want.Symbol })
		if i < 0 || !reflect.DeepEqual(batch.Listings[i], want) {
			t.Errorf("%s: got %+v, want %+v", want.Symbol, batch.Listings, want)
		}
	}
}

func TestListings_failsWhenCoinbaseIsDown(t *testing.T) {
	t.Parallel()
	_, err := recorded(t, http.StatusServiceUnavailable).Listings(t.Context())
	if errs.CodeOf(err) != errs.CodeUpstreamUnavailable {
		t.Fatalf("err = %v, want upstream_unavailable", err)
	}
}
