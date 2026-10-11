package ondo_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/market/adapters/ondo"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpclient"
)

const assetList = `{"assets":[{"symbol":"AAPLon","ticker":"AAPL","isTradingPaused":false}]}`

func hangUp(w http.ResponseWriter, _ *http.Request) {
	conn, _, err := w.(http.Hijacker).Hijack()
	if err == nil {
		_, _ = conn.Write([]byte("HTTP/1.1 200 OK\r\nContent-Length: 100\r\n\r\npartial"))
		_ = conn.Close()
	}
}

func text(status int, body string) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}
}

func serving(t *testing.T, list, page http.HandlerFunc) *ondo.Client {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v2/assets", list)
	mux.HandleFunc("GET /assets/", page)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return ondo.New(httpclient.New("ondo", httpclient.WithBaseURL(srv.URL), httpclient.WithTimeout(10*time.Second)))
}

func networks(shares, entries string) string {
	return `<script>"sharesMultiplier":"` + shares + `","symbol":"AAPLon","supportedNetworks":` + entries + `</script>`
}

func TestListings_refusesAListOrPageItCannotRead(t *testing.T) {
	t.Parallel()
	ok := text(http.StatusOK, assetList)
	page := func(body string) http.HandlerFunc { return text(http.StatusOK, body) }
	for name, tc := range map[string]struct{ list, page http.HandlerFunc }{
		"list down":        {text(http.StatusBadGateway, ""), page("")},
		"list refused":     {text(http.StatusForbidden, ""), page("")},
		"list bad json":    {text(http.StatusOK, `{"assets":`), page("")},
		"list hangs up":    {hangUp, page("")},
		"page hangs up":    {ok, hangUp},
		"zero shares":      {ok, page(networks("0", `[]`))},
		"networks garbled": {ok, page(networks("1", `nope`))},
		"bad solana mint":  {ok, page(networks("1", `[{"network":"SOLANA","address":"nope","decimals":9}]`))},
		"wrong chain id":   {ok, page(networks("1", `[{"network":"ETHEREUM","chainId":5,"address":"0x12","decimals":18}]`))},
		"bad evm address":  {ok, page(networks("1", `[{"network":"ETHEREUM","chainId":1,"address":"0x12","decimals":18}]`))},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if _, err := serving(t, tc.list, tc.page).Listings(t.Context()); err == nil || errs.CodeOf(err) == "" {
				t.Fatalf("err = %v, want a coded error", err)
			}
		})
	}
}

func TestListings_skipsNetworksOtherThanEthereumAndSolana(t *testing.T) {
	t.Parallel()
	c := serving(t, text(http.StatusOK, assetList),
		text(http.StatusOK, networks("1", `[{"network":"ARBITRUM","chainId":42161,"address":"0x12","decimals":18}]`)))
	batch, err := c.Listings(t.Context())
	if err != nil || len(batch.Listings) != 0 || len(batch.Missing) != 0 {
		t.Fatalf("batch = %+v, %v, want nothing listed", batch, err)
	}
}

func TestListings_stopsWhenThePageBudgetRunsOut(t *testing.T) {
	t.Parallel()
	symbols := make([]string, 30)
	for i := range symbols {
		symbols[i] = `{"symbol":"S` + strconv.Itoa(i) + `on","ticker":"S","isTradingPaused":false}`
	}
	c := serving(t, text(http.StatusOK, `{"assets":[`+strings.Join(symbols, ",")+`]}`), text(http.StatusNotFound, ""))
	ctx, cancel := context.WithTimeout(t.Context(), 500*time.Millisecond)
	defer cancel()
	if _, err := c.Listings(ctx); err == nil {
		t.Fatal("thirty pages fit a 500ms budget at 10 pages a second")
	}
}

func TestListings_failsWhenOndoIsUnreachable(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.NotFoundHandler())
	srv.Close()
	c := ondo.New(httpclient.New("ondo", httpclient.WithBaseURL(srv.URL), httpclient.WithTimeout(time.Second)))
	if _, err := c.Listings(t.Context()); err == nil {
		t.Fatal("an unreachable API returned listings")
	}
}
