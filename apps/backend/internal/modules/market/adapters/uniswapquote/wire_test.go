package uniswapquote_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sync/atomic"
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/market/adapters/uniswapquote"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	mc "github.com/monaco/monaco/apps/backend/internal/platform/chain/multichain"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpclient"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

func stubClient(t *testing.T, status int, body string) *uniswapquote.Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return clientAt(t, srv.URL)
}

func clientAt(t *testing.T, url string) *uniswapquote.Client {
	t.Helper()
	swapper, err := chain.ParseEVMAddress("0x000000000000000000000000000000000000dead")
	if err != nil {
		t.Fatal(err)
	}
	hc := httpclient.New("uniswap-quote-test", uniswapquote.Options(url, 5*time.Second)...)
	return uniswapquote.New(hc, "test-key", swapper, testkit.NewClock(quotedAt()))
}

type wireQuote struct {
	Routing string `json:"routing"`
	Quote   struct {
		QuoteID   string                 `json:"quoteId"`
		Input     map[string]string      `json:"input"`
		Output    map[string]string      `json:"output"`
		GasFeeUSD string                 `json:"gasFeeUSD"`
		Route     [][]map[string]*string `json:"route"`
	} `json:"quote"`
}

func ptr(s string) *string { return &s }

func baseQuote() wireQuote {
	var w wireQuote
	w.Routing = "CLASSIC"
	w.Quote.QuoteID = "q-1"
	w.Quote.Input = map[string]string{"amount": "100000000"}
	w.Quote.Output = map[string]string{"amount": "5000"}
	w.Quote.GasFeeUSD = "0.5"
	w.Quote.Route = [][]map[string]*string{{{"fee": ptr("3000")}}}
	return w
}

func encode(t *testing.T, w wireQuote) string {
	t.Helper()
	raw, err := json.Marshal(w)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func buy(t *testing.T, c *uniswapquote.Client) (mc.Quote, error) {
	t.Helper()
	return c.Quote(t.Context(), listing(t, chain.NetworkEthereum, aaplon, 18), mc.SideBuy,
		money.MicrosFromUint64(hundred))
}

func costs(pool, gas uint64) []mc.Cost {
	return []mc.Cost{
		{Kind: mc.CostPoolFee, Micros: money.MicrosFromUint64(pool)},
		{Kind: mc.CostGas, Micros: money.MicrosFromUint64(gas)},
	}
}

func TestQuote_WeighsSplitRoutesByFirstHopAmountAndDefaultsToTheV2Fee(t *testing.T) {
	t.Parallel()
	w := baseQuote()
	w.Quote.Route = [][]map[string]*string{
		{{"fee": ptr("10000"), "amountIn": ptr("25000000")}, {"fee": ptr("500")}},
		{{"amountIn": ptr("75000000")}},
	}
	w.Quote.GasFeeUSD = "0.0000001"
	q, err := buy(t, stubClient(t, http.StatusOK, encode(t, w)))
	if err != nil {
		t.Fatalf("Quote: %v", err)
	}
	if want := costs(250_000+225_000, 1); !reflect.DeepEqual(q.Costs, want) {
		t.Errorf("costs = %+v, want %+v", q.Costs, want)
	}
	if q.GrossOutput.String() != "5000" || q.RouteID != "q-1" {
		t.Errorf("gross = %s, route = %s", q.GrossOutput, q.RouteID)
	}
}

func TestQuote_MalformedQuotesAreDecodeFailed(t *testing.T) {
	t.Parallel()
	cases := map[string]func(*wireQuote){
		"unparsable cash leg":    func(w *wireQuote) { w.Quote.Input["amount"] = "1.5" },
		"zero token leg":         func(w *wireQuote) { w.Quote.Output["amount"] = "0" },
		"non-numeric token leg":  func(w *wireQuote) { w.Quote.Output["amount"] = "lots" },
		"no route":               func(w *wireQuote) { w.Quote.Route = nil },
		"empty split":            func(w *wireQuote) { w.Quote.Route = [][]map[string]*string{{}} },
		"fractional fee":         func(w *wireQuote) { w.Quote.Route[0][0]["fee"] = ptr("0.3") },
		"split without amountIn": func(w *wireQuote) { w.Quote.Route = append(w.Quote.Route, w.Quote.Route[0]) },
		"split with signed amountIn": func(w *wireQuote) {
			hop := map[string]*string{"amountIn": ptr("-5")}
			w.Quote.Route = [][]map[string]*string{{hop}, {hop}}
		},
		"fee overflowing micros": func(w *wireQuote) { w.Quote.Route[0][0]["fee"] = ptr("1000000000000000000000000") },
		"gas in exponent form":   func(w *wireQuote) { w.Quote.GasFeeUSD = "1e3" },
		"gas missing":            func(w *wireQuote) { w.Quote.GasFeeUSD = "" },
		"gas overflowing micros": func(w *wireQuote) { w.Quote.GasFeeUSD = "99999999999999999999" },
		"dutch auction routing":  func(w *wireQuote) { w.Routing = "DUTCH_V2" },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			w := baseQuote()
			mutate(&w)
			_, err := buy(t, stubClient(t, http.StatusOK, encode(t, w)))
			if errs.CodeOf(err) != errs.CodeDecodeFailed {
				t.Fatalf("err = %v, want %s", err, errs.CodeDecodeFailed)
			}
		})
	}
}

func TestQuote_NonJSONBodyIsDecodeFailed(t *testing.T) {
	t.Parallel()
	_, err := buy(t, stubClient(t, http.StatusOK, "<html>gateway</html>"))
	if errs.CodeOf(err) != errs.CodeDecodeFailed {
		t.Fatalf("err = %v, want %s", err, errs.CodeDecodeFailed)
	}
}

func TestQuote_TruncatedBodyIsDecodeFailed(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Length", "4096")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"routing":"CLASSIC","quote":{`))
		w.(http.Flusher).Flush()
		conn, _, err := w.(http.Hijacker).Hijack()
		if err == nil {
			_ = conn.Close()
		}
	}))
	t.Cleanup(srv.Close)
	_, err := buy(t, clientAt(t, srv.URL))
	if errs.CodeOf(err) != errs.CodeDecodeFailed {
		t.Fatalf("err = %v, want %s", err, errs.CodeDecodeFailed)
	}
}

func TestQuote_RejectedRequestIsInvalidInput(t *testing.T) {
	t.Parallel()
	_, err := buy(t, stubClient(t, http.StatusBadRequest, `{"errorCode":"RequestValidationError"}`))
	if errs.CodeOf(err) != errs.CodeInvalidInput {
		t.Fatalf("err = %v, want %s", err, errs.CodeInvalidInput)
	}
}

func TestQuote_UnreachableUpstreamFails(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.NotFoundHandler())
	url := srv.URL
	srv.Close()
	_, err := buy(t, clientAt(t, url))
	if err == nil || errs.CodeOf(err) == errs.CodeDecodeFailed {
		t.Fatalf("err = %v, want a transport failure", err)
	}
}

func TestQuote_CancelledContextIsUnavailableWithoutCalling(t *testing.T) {
	t.Parallel()
	var called atomic.Bool
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { called.Store(true) }))
	t.Cleanup(srv.Close)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	_, err := clientAt(t, srv.URL).Quote(ctx, listing(t, chain.NetworkEthereum, aaplon, 18), mc.SideBuy,
		money.MicrosFromUint64(hundred))
	if errs.CodeOf(err) != errs.CodeUpstreamUnavailable || called.Load() {
		t.Fatalf("err = %v, called = %v, want %s and no call", err, called.Load(), errs.CodeUpstreamUnavailable)
	}
}

func TestQuote_UnknownSideZeroAmountAndMissingTokenAreInvalidInput(t *testing.T) {
	t.Parallel()
	c := stubClient(t, http.StatusOK, encode(t, baseQuote()))
	eth := listing(t, chain.NetworkEthereum, aaplon, 18)
	noToken := listing(t, chain.NetworkEthereum, "", 18)
	cases := map[string]struct {
		l      mc.Listing
		side   mc.Side
		amount uint64
	}{
		"unknown side": {eth, mc.Side("short"), hundred},
		"zero amount":  {eth, mc.SideBuy, 0},
		"no token":     {noToken, mc.SideBuy, hundred},
	}
	for name, tc := range cases {
		_, err := c.Quote(t.Context(), tc.l, tc.side, money.MicrosFromUint64(tc.amount))
		if errs.CodeOf(err) != errs.CodeInvalidInput {
			t.Errorf("%s: err = %v, want %s", name, err, errs.CodeInvalidInput)
		}
	}
}
