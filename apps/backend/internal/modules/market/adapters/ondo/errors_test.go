package ondo_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/market/adapters/ondo"
	"github.com/monaco/monaco/apps/backend/internal/platform/config"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpclient"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
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
		"list down":     {text(http.StatusBadGateway, ""), page("")},
		"list refused":  {text(http.StatusForbidden, ""), page("")},
		"list bad json": {text(http.StatusOK, `{"assets":`), page("")},
		"list hangs up": {hangUp, page("")},
		"page hangs up": {ok, hangUp},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if _, err := serving(t, tc.list, tc.page).Listings(t.Context()); err == nil || errs.CodeOf(err) == "" {
				t.Fatalf("err = %v, want a coded error", err)
			}
		})
	}
}

func assetPages(t *testing.T, pages map[string]string) *ondo.Client {
	t.Helper()
	list := make([]string, 0, len(pages))
	mux := http.NewServeMux()
	for symbol, page := range pages {
		paused := strconv.FormatBool(strings.HasPrefix(symbol, "P"))
		list = append(list, `{"symbol":"`+symbol+`","ticker":"T","isTradingPaused":`+paused+`}`)
		mux.HandleFunc("GET /assets/"+strings.ToLower(symbol), text(http.StatusOK, page))
	}
	slices.Sort(list)
	mux.HandleFunc("GET /api/v2/assets", text(http.StatusOK, `{"assets":[`+strings.Join(list, ",")+`]}`))
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return ondo.New(httpclient.New("ondo", httpclient.WithBaseURL(srv.URL), httpclient.WithTimeout(10*time.Second)))
}

func TestListings_skipsAnUnreadableAssetAndAlertsOnlyWhenItTrades(t *testing.T) {
	t.Parallel()
	page := func(symbol, shares, entries string) string {
		return `<script>"sharesMultiplier":"1","symbol":"Other","supportedNetworks":[]` +
			`"sharesMultiplier":"` + shares + `","symbol":"` + symbol + `","supportedNetworks":` + entries + `</script>`
	}
	eth := func(chainID, address, decimals string) string {
		return `[{"network":"ETHEREUM","chainId":` + chainID + `,"address":"` + address + `"` + decimals + `}]`
	}
	const good = "0x14c3abf95cb9c93a8b82c1cdcb76d72cb87b2d4c"
	c := assetPages(t, map[string]string{
		"A1on": page("A1on", "0", `[]`),
		"A2on": page("A2on", "1", `nope`),
		"A3on": page("A3on", "1", `[{"network":"SOLANA","address":"nope","decimals":9}]`),
		"A4on": page("A4on", "1", eth("5", good, `,"decimals":18`)),
		"A5on": page("A5on", "1", eth("1", "0x12", `,"decimals":18`)),
		"A6on": page("A6on", "1", eth("1", good, "")),
		"A7on": page("A7on", "1", eth("1", good, `,"decimals":18`)),
		"P1on": page("P1on", "0", `[]`),
		"P2on": page("P2on", "1", eth("1", good, `,"decimals":18`)),
	})
	var logs testkit.Logs
	ctx := observability.WithLogger(t.Context(), observability.NewLogger(config.Config{Env: config.EnvTest}, &logs))
	batch, err := c.Listings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	kept := make([]string, 0, len(batch.Listings))
	for _, l := range batch.Listings {
		kept = append(kept, fmt.Sprint(l.Symbol, " ", l.Tradable, " ", l.TokenAddress, " ", *l.Decimals))
	}
	if want := []string{"A7on true " + good + " 18", "P2on false " + good + " 18"}; !slices.Equal(kept, want) {
		t.Errorf("kept = %q, want %q", kept, want)
	}
	alerts := logs.Lines("market.listings.asset_skipped", "level", "issuer", "symbol", "reason")
	want := []string{
		"WARN ondo A1on bad multiplier", "WARN ondo A2on bad networks", "WARN ondo A3on bad address",
		"WARN ondo A4on wrong chain id", "WARN ondo A5on bad address", "WARN ondo A6on missing decimals",
	}
	if !slices.Equal(alerts, want) {
		t.Errorf("alerts = %q, want %q", alerts, want)
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
