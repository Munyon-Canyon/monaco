package robinhood_test

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
	"github.com/monaco/monaco/apps/backend/internal/modules/market/adapters/robinhood"
	"github.com/monaco/monaco/apps/backend/internal/modules/market/app"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpclient"
)

const (
	aaplOnRobinhood = "0xaf3d76f1834a1d425780943c99ea8a608f8a93f9"
	crmOnRobinhood  = "0xd95b44124e475743a7589e68f3d74008a5536d44"
)

func recorded(t *testing.T, status int) *robinhood.Client {
	t.Helper()
	body, err := os.ReadFile("testdata/assets.json")
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /rhj/assets", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write(body)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return robinhood.New(httpclient.New("robinhood", httpclient.WithBaseURL(srv.URL),
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

func TestListings_readsEveryRobinhoodTokenOnRobinhoodChain(t *testing.T) {
	t.Parallel()
	c := recorded(t, http.StatusOK)
	batch, err := c.Listings(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if c.Issuer() != "robinhood" || batch.Issuer != "robinhood" || len(batch.Listings) != 3 || len(batch.Missing) != 0 {
		t.Fatalf("batch = %+v", batch)
	}
	eighteen := uint8(18)
	base := app.IssuerListing{
		Issuer: "robinhood", Network: chain.NetworkRobinhood, ChainID: 4663, Decimals: &eighteen, Tradable: true,
	}
	aapl, crm := base, base
	aapl.Ticker, aapl.Symbol, aapl.TokenAddress = "AAPL", "AAPL", aaplOnRobinhood
	aapl.SharesPerToken = rat(t, "1.000566080061092436")
	crm.Ticker, crm.Symbol, crm.TokenAddress = "CRM", "CRM", crmOnRobinhood
	crm.SharesPerToken, crm.PendingSharesPerToken = rat(t, "1.001148322800714293"), rat(t, "1.002296645601428586")
	for _, want := range []app.IssuerListing{aapl, crm} {
		i := slices.IndexFunc(batch.Listings, func(l app.IssuerListing) bool { return l.Symbol == want.Symbol })
		if i < 0 || !reflect.DeepEqual(batch.Listings[i], want) {
			t.Errorf("%s: got %+v, want %+v", want.Symbol, batch.Listings, want)
		}
	}
}

func TestListings_failsWhenRobinhoodIsDown(t *testing.T) {
	t.Parallel()
	_, err := recorded(t, http.StatusBadGateway).Listings(t.Context())
	if errs.CodeOf(err) != errs.CodeUpstreamUnavailable {
		t.Fatalf("err = %v, want upstream_unavailable", err)
	}
}
