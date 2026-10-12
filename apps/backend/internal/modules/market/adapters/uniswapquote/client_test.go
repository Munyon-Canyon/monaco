package uniswapquote_test

import (
	"math/big"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/market/adapters/uniswapquote"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	mc "github.com/monaco/monaco/apps/backend/internal/platform/chain/multichain"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpclient"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
	"github.com/monaco/monaco/apps/backend/internal/testkit/fakes/uniswap"
)

func quotedAt() time.Time { return time.Date(2026, 10, 11, 15, 0, 0, 0, time.UTC) }

type harness struct {
	fake   *uniswap.Server
	client *uniswapquote.Client
}

func newHarness(t *testing.T) harness {
	t.Helper()
	fake := uniswap.New()
	srv := httptest.NewServer(fake)
	t.Cleanup(srv.Close)
	swapper, err := chain.ParseEVMAddress("0x000000000000000000000000000000000000dead")
	if err != nil {
		t.Fatal(err)
	}
	hc := httpclient.New("uniswap-quote-test", uniswapquote.Options(srv.URL, 5*time.Second)...)
	return harness{
		fake:   fake,
		client: uniswapquote.New(hc, "test-key", uniswapquote.Stored(swapper), testkit.NewClock(quotedAt())),
	}
}

func listing(t *testing.T, network chain.Network, token string, decimals uint8) mc.Listing {
	t.Helper()
	var id mc.ListingID
	if err := id.UnmarshalText([]byte("0199d2f4-5a10-7000-8000-000000000001")); err != nil {
		t.Fatal(err)
	}
	return mc.Listing{ID: id, Network: network, TokenAddress: token, Decimals: decimals, Venue: mc.VenueUniswap}
}

const (
	aaplon  = "0x14c3abf95cb9c93a8b82c1cdcb76d72cb87b2d4c"
	tslaon  = "0xf6b1117ec07684d3958cad8beb1b302bfd21103f"
	aaplc   = "0xb200000000000000000000c2e324d24d7eecd1fb"
	aaplx   = "0x9d275685dc284c8eb1c79f6aba7a63dc75ec890a"
	solMint = "XsbEhLAtcf6HdfpFZ5xEMdqW8nfAvcsP5bdudRLJzJp"
	hundred = 100_000_000
)

type quoteCase struct {
	name      string
	network   chain.Network
	token     string
	decimals  uint8
	side      mc.Side
	gross     string
	poolFee   uint64
	gas       uint64
	routeID   string
	wantIn    string
	wantOut   string
	wantType  string
	wantChain int64
}

func TestQuote_MapsRecordedQuotesToNetworkCostsAndExpiry(t *testing.T) {
	t.Parallel()
	cases := []quoteCase{
		{
			name: "buy AAPLon on Ethereum through a 1% v3 pool", network: chain.NetworkEthereum, token: aaplon,
			decimals: 18, side: mc.SideBuy, gross: "294521447600868269", poolFee: 1_000_000, gas: 35_171,
			routeID: "01a12a2f-23cd-8000-8bce-0d4e5bfb0100", wantType: "EXACT_INPUT", wantChain: 1,
			wantIn: chain.NetworkEthereum.Cash().Address, wantOut: aaplon,
		},
		{
			name: "buy AAPLc on Base whose first hop is a 0.01% pool", network: chain.NetworkBase, token: aaplc,
			decimals: 8, side: mc.SideBuy, gross: "30683976", poolFee: 10_000, gas: 75_874,
			routeID: "01a12a2f-24b1-8000-b523-0d4e5bfb0100", wantType: "EXACT_INPUT", wantChain: 8453,
			wantIn: chain.NetworkBase.Cash().Address, wantOut: aaplc,
		},
		{
			name:    "buy TSLAon on Ethereum split 35% through a 1% pool and 65% through a 0.3% pool",
			network: chain.NetworkEthereum, token: tslaon, decimals: 18, side: mc.SideBuy,
			gross: "259103886869840668", poolFee: 545_000, gas: 49_737,
			routeID: "01a12a84-1b14-8000-8bce-0d4e5bfb0100", wantType: "EXACT_INPUT", wantChain: 1,
			wantIn: chain.NetworkEthereum.Cash().Address, wantOut: tslaon,
		},
		{
			name: "sell AAPLon for exactly $100 spends tokens", network: chain.NetworkEthereum, token: aaplon,
			decimals: 18, side: mc.SideSell, gross: "300581109425298407", poolFee: 1_000_000, gas: 15_630,
			routeID: "01a12a2f-e07f-8000-92ff-0d4e5bfb0100", wantType: "EXACT_OUTPUT", wantChain: 1,
			wantIn: aaplon, wantOut: chain.NetworkEthereum.Cash().Address,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			h := newHarness(t)
			l := listing(t, tc.network, tc.token, tc.decimals)
			q, err := h.client.Quote(t.Context(), l, tc.side, money.MicrosFromUint64(hundred))
			if err != nil {
				t.Fatalf("Quote: %v", err)
			}
			assertQuote(t, tc, l, q)
			reqs := h.fake.Requests()
			if len(reqs) != 1 {
				t.Fatalf("requests = %d, want 1", len(reqs))
			}
			assertRequest(t, tc, reqs[0])
		})
	}
}

func assertQuote(t *testing.T, tc quoteCase, l mc.Listing, q mc.Quote) {
	t.Helper()
	gross, _ := new(big.Int).SetString(tc.gross, 10)
	want := mc.Quote{
		ListingID: l.ID, Network: tc.network, Venue: mc.VenueUniswap, Side: tc.side,
		InputMicros: money.MicrosFromUint64(hundred), GrossOutput: gross,
		Costs: []mc.Cost{
			{Kind: mc.CostPoolFee, Micros: money.MicrosFromUint64(tc.poolFee)},
			{Kind: mc.CostGas, Micros: money.MicrosFromUint64(tc.gas)},
		},
		QuotedAt: quotedAt(), ExpiresAt: quotedAt().Add(30 * time.Second), RouteID: tc.routeID,
	}
	if !reflect.DeepEqual(q, want) {
		t.Errorf("quote = %+v\nwant    %+v", q, want)
	}
}

func assertRequest(t *testing.T, tc quoteCase, r uniswap.Request) {
	t.Helper()
	want := uniswap.Request{
		Type: tc.wantType, Amount: "100000000", TokenInChainID: tc.wantChain, TokenOutChainID: tc.wantChain,
		TokenIn: tc.wantIn, TokenOut: tc.wantOut, Swapper: "0x000000000000000000000000000000000000dead",
		RoutingPreference: "BEST_PRICE", Protocols: []string{"V2", "V3", "V4"}, APIKey: "test-key",
	}
	if !reflect.DeepEqual(r, want) {
		t.Errorf("request = %+v, want %+v", r, want)
	}
}

func TestQuote_NoRouteFoundIsNoRoute(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	_, err := h.client.Quote(t.Context(), listing(t, chain.NetworkArbitrum, aaplx, 18), mc.SideBuy,
		money.MicrosFromUint64(hundred))
	if errs.CodeOf(err) != errs.CodeNoRoute {
		t.Fatalf("err = %v, want %s", err, errs.CodeNoRoute)
	}
}

func TestQuote_ThrottledOrDownUpstreamIsUnavailable(t *testing.T) {
	t.Parallel()
	for _, status := range []int{http.StatusTooManyRequests, http.StatusInternalServerError, http.StatusBadGateway} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			t.Parallel()
			h := newHarness(t)
			h.fake.FailNext(status)
			_, err := h.client.Quote(t.Context(), listing(t, chain.NetworkEthereum, aaplon, 18), mc.SideBuy,
				money.MicrosFromUint64(hundred))
			if errs.CodeOf(err) != errs.CodeUpstreamUnavailable {
				t.Fatalf("err = %v, want %s", err, errs.CodeUpstreamUnavailable)
			}
		})
	}
}

func TestQuote_RejectsSolanaListingsWithoutCalling(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	_, err := h.client.Quote(t.Context(), listing(t, chain.NetworkSolana, solMint, 8),
		mc.SideBuy, money.MicrosFromUint64(hundred))
	if errs.CodeOf(err) != errs.CodeInvalidInput || len(h.fake.Requests()) != 0 {
		t.Fatalf("err = %v, requests = %d, want invalid_input and no call", err, len(h.fake.Requests()))
	}
}
