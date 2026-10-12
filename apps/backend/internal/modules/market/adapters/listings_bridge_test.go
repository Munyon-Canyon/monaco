package adapters

import (
	"context"
	"encoding/json"
	"io"
	"math/big"
	"net/http"
	"net/http/httptest"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/market/app"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/multichain"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/config"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpclient"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

func bridgeServer(t *testing.T, status int, body string, seen *bridgeQuoteRequest) PrivyBridgeQuotes {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if user, pass, ok := r.BasicAuth(); !ok || user != "app" || pass != "secret" ||
			r.Header.Get("privy-app-id") != "app" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		raw, _ := io.ReadAll(r.Body)
		if seen != nil {
			_ = json.Unmarshal(raw, seen)
		}
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return NewPrivyBridgeQuotes(httpclient.New("privy-bridge", httpclient.WithBaseURL(srv.URL),
		httpclient.WithTimeout(time.Second)), "app", "secret")
}

func TestBridgeOutReadsPrivysQuote(t *testing.T) {
	t.Parallel()
	var seen bridgeQuoteRequest
	b := bridgeServer(t, http.StatusOK,
		`{"input_amount":"100","estimated_output_amount":"99.925","created_at":"2026-10-11T09:02:20.834Z"}`, &seen)
	got, err := b.BridgeOut(t.Context(), chain.NetworkBase, money.MicrosFromUint64(100_000_000))
	if err != nil || got.Uint64() != 99_925_000 {
		t.Fatalf("BridgeOut = %s, %v, want 99925000", got, err)
	}
	if seen.Source.Chain != "solana" || seen.Destination.Chain != "base" || seen.InputAmount != "100.000000" ||
		seen.Source.Asset != "usdc" || seen.Destination.Asset != "usdc" {
		t.Fatalf("request = %+v", seen)
	}
}

func TestBridgeOutRefusesWhatItCannotUse(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct {
		status int
		body   string
		want   errs.Code
	}{
		"unsupported": {http.StatusBadRequest, `{"error":"usdc is not available"}`, errs.CodeNoRoute},
		"bad json":    {http.StatusOK, `{"estimated`, errs.CodeDecodeFailed},
		"no amount":   {http.StatusOK, `{"estimated_output_amount":"x"}`, errs.CodeDecodeFailed},
		"zero":        {http.StatusOK, `{"estimated_output_amount":"0"}`, errs.CodeDecodeFailed},
		"more out":    {http.StatusOK, `{"estimated_output_amount":"100.5"}`, errs.CodeDecodeFailed},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			_, err := bridgeServer(t, tc.status, tc.body, nil).BridgeOut(t.Context(), chain.NetworkBase,
				money.MicrosFromUint64(100_000_000))
			if errs.CodeOf(err) != tc.want {
				t.Fatalf("err = %v, want %s", err, tc.want)
			}
		})
	}
}

func TestBridgeOutFailsWhenPrivyIsUnreachable(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.NotFoundHandler())
	srv.Close()
	b := NewPrivyBridgeQuotes(httpclient.New("privy-bridge", httpclient.WithBaseURL(srv.URL),
		httpclient.WithTimeout(time.Second)), "app", "secret")
	if _, err := b.BridgeOut(t.Context(), chain.NetworkBase, money.MicrosFromUint64(1)); err == nil {
		t.Fatal("an unreachable Privy returned a quote")
	}
}

type robinhoodRoute struct {
	clk    *testkit.Clock
	mu     sync.Mutex
	inputs map[chain.Network]money.Micros
	units  map[chain.Network]*big.Int
	listed []multichain.Listing
}

func (r *robinhoodRoute) EnabledListings(context.Context, multichain.StockID) ([]multichain.Listing, error) {
	return r.listed, nil
}

func (r *robinhoodRoute) SharePrice(context.Context, multichain.StockID) (money.Micros, error) {
	return money.MicrosFromUint64(250_000_000), nil
}

func (r *robinhoodRoute) Quote(
	_ context.Context, l multichain.Listing, side multichain.Side, amount money.Micros,
) (multichain.Quote, error) {
	r.mu.Lock()
	r.inputs[l.Network] = amount
	r.mu.Unlock()
	now := r.clk.Now()
	return multichain.Quote{
		Side: side, InputMicros: amount, GrossOutput: r.units[l.Network],
		QuotedAt: now, ExpiresAt: now.Add(30 * time.Second), RouteID: "recorded",
	}, nil
}

func routeRobinhood(
	t *testing.T, bridge PrivyBridgeQuotes,
) (multichain.Comparison, *robinhoodRoute, *testkit.Logs, error) {
	t.Helper()
	g := testkit.NewIDs(5203)
	id := func() multichain.ListingID {
		var out multichain.ListingID
		_ = out.UnmarshalText([]byte(g.NewV7().String()))
		return out
	}
	f := &robinhoodRoute{
		clk:    testkit.NewClock(clock.Real{}.Now().Truncate(time.Second)),
		inputs: map[chain.Network]money.Micros{},
		units: map[chain.Network]*big.Int{
			chain.NetworkSolana:    big.NewInt(2_000_000),
			chain.NetworkRobinhood: new(big.Int).Mul(big.NewInt(205), big.NewInt(100_000_000_000_000)),
		},
		listed: []multichain.Listing{
			{
				ID: id(), Issuer: "xstocks", Symbol: "AAPLx", Network: chain.NetworkSolana, Decimals: 8,
				SharesPerToken: big.NewRat(1, 1), Venue: multichain.VenueJupiter,
			},
			{
				ID: id(), Issuer: "robinhood", Symbol: "AAPL", Network: chain.NetworkRobinhood, Decimals: 18,
				SharesPerToken: big.NewRat(1, 1), Venue: multichain.VenueUniswap,
			},
		},
	}
	var logs testkit.Logs
	r := app.NewRouter(f, f, f, bridge, f.clk, app.RouterRules{
		MinEdgeBps: 10, MaxImpactBps: 100, QuoteTimeout: time.Second, MaxQuoteAge: 15 * time.Second,
	})
	var stock multichain.StockID
	_ = stock.UnmarshalText([]byte(g.NewV7().String()))
	ctx := observability.WithLogger(t.Context(), observability.NewLogger(config.Config{Env: config.EnvTest}, &logs))
	c, err := r.Best(ctx, stock, multichain.SideBuy, money.MicrosFromUint64(5_000_000))
	return c, f, &logs, err
}

func TestRouterRanksARobinhoodListingOnItsUSDGBridgeQuote(t *testing.T) {
	t.Parallel()
	var seen bridgeQuoteRequest
	b := bridgeServer(t, http.StatusOK,
		`{"input_amount":"5","estimated_output_amount":"4.968995","created_at":"2026-10-11T15:52:06.041Z"}`, &seen)
	c, f, _, err := routeRobinhood(t, b)
	if err != nil || len(c.Ranked) != 2 || c.Winner != f.listed[1].ID {
		t.Fatalf("comparison = %+v, %v, want the Robinhood listing to win", c.Ranked, err)
	}
	if seen.Destination != (bridgeLeg{Chain: "robinhood", Asset: "usdg"}) ||
		seen.Source != (bridgeLeg{Chain: "solana", Asset: "usdc"}) {
		t.Fatalf("bridge quote request = %+v, want solana usdc to robinhood usdg", seen)
	}
	if got := f.inputs[chain.NetworkRobinhood].Uint64(); got != 4_968_995 {
		t.Fatalf("Uniswap quoted %d micros of USDG, want the 4968995 Privy bridges", got)
	}
	var bridged uint64
	for _, cost := range c.Ranked[0].Costs {
		if cost.Kind == multichain.CostBridge {
			bridged += cost.Micros.Uint64()
		}
	}
	if bridged != 31_005 {
		t.Fatalf("bridge cost = %d micros, want 31005", bridged)
	}
}

func TestRouterDropsARobinhoodListingWhoseBridgeFails(t *testing.T) {
	t.Parallel()
	b := bridgeServer(t, http.StatusBadRequest,
		`{"error":"Asset \"usdc\" is not available on chain \"eip155:4663\".","code":"invalid_data"}`, nil)
	c, f, logs, err := routeRobinhood(t, b)
	if err != nil || len(c.Ranked) != 1 || c.Winner != f.listed[0].ID {
		t.Fatalf("comparison = %+v, %v, want only the Solana listing", c.Ranked, err)
	}
	want := []string{f.listed[1].ID.String() + " no_route"}
	if got := logs.Lines("market.router.venue_dropped", "listing_id", "reason"); !slices.Equal(got, want) {
		t.Fatalf("venue_dropped = %v, want %v", got, want)
	}
}

type countingBridge struct {
	calls   int
	arrived func(money.Micros) money.Micros
	err     error
}

func (b *countingBridge) BridgeOut(_ context.Context, _ chain.Network, amount money.Micros) (money.Micros, error) {
	b.calls++
	if b.err != nil {
		return money.Micros{}, b.err
	}
	return b.arrived(amount), nil
}

func keep999(amount money.Micros) money.Micros {
	return money.MicrosFromUint64(amount.Uint64() * 999 / 1000)
}

func usd(dollars uint64) money.Micros { return money.MicrosFromUint64(dollars * 1_000_000) }

func TestCachedBridgeOutAsksPrivyOncePerBucketWithinTTL(t *testing.T) {
	t.Parallel()
	inner := &countingBridge{arrived: keep999}
	c := NewCachedBridgeQuotes(inner, testkit.NewClock(time.Date(2026, 10, 12, 9, 0, 0, 0, time.UTC)))
	for _, amount := range []money.Micros{usd(3), usd(4), money.MicrosFromUint64(3_500_001)} {
		if _, err := c.BridgeOut(t.Context(), chain.NetworkBase, amount); err != nil {
			t.Fatal(err)
		}
	}
	if inner.calls != 1 {
		t.Fatalf("Privy calls for $3, $4 and $3.500001 on base = %d, want 1", inner.calls)
	}
	if _, err := c.BridgeOut(t.Context(), chain.NetworkBase, usd(5)); err != nil {
		t.Fatal(err)
	}
	for _, amount := range []money.Micros{money.MicrosFromUint64(400_000), usd(1)} {
		if _, err := c.BridgeOut(t.Context(), chain.NetworkEthereum, amount); err != nil {
			t.Fatal(err)
		}
	}
	if inner.calls != 3 {
		t.Fatalf("Privy calls after a new bucket and $0.40 then $1 on a new network = %d, want 3", inner.calls)
	}
}

func TestCachedBridgeOutAsksPrivyAgainAt60s(t *testing.T) {
	t.Parallel()
	inner := &countingBridge{arrived: keep999}
	clk := testkit.NewClock(time.Date(2026, 10, 12, 9, 0, 0, 0, time.UTC))
	c := NewCachedBridgeQuotes(inner, clk)
	for _, step := range []time.Duration{0, 59 * time.Second, time.Second} {
		clk.Advance(step)
		if _, err := c.BridgeOut(t.Context(), chain.NetworkBase, usd(10)); err != nil {
			t.Fatal(err)
		}
	}
	if inner.calls != 2 {
		t.Fatalf("Privy calls at 0s, 59s and 60s = %d, want 2", inner.calls)
	}
}

func TestCachedBridgeOutNeverCachesAnError(t *testing.T) {
	t.Parallel()
	inner := &countingBridge{err: errs.New(errs.CodeNoRoute, "test")}
	c := NewCachedBridgeQuotes(inner, testkit.NewClock(time.Date(2026, 10, 12, 9, 0, 0, 0, time.UTC)))
	if _, err := c.BridgeOut(t.Context(), chain.NetworkBase, usd(10)); errs.CodeOf(err) != errs.CodeNoRoute {
		t.Fatalf("first err = %v, want no_route", err)
	}
	inner.err = nil
	inner.arrived = keep999
	got, err := c.BridgeOut(t.Context(), chain.NetworkBase, usd(10))
	if err != nil || inner.calls != 2 || got.Uint64() != 9_990_000 {
		t.Fatalf("retry = %s, %v after %d calls, want 9990000 after 2", got, err, inner.calls)
	}
}

func TestCachedBridgeOutRoundsTheCachedRatioDown(t *testing.T) {
	t.Parallel()
	inner := &countingBridge{arrived: func(money.Micros) money.Micros { return money.MicrosFromUint64(2_997_001) }}
	c := NewCachedBridgeQuotes(inner, testkit.NewClock(time.Date(2026, 10, 12, 9, 0, 0, 0, time.UTC)))
	if _, err := c.BridgeOut(t.Context(), chain.NetworkBase, usd(3)); err != nil {
		t.Fatal(err)
	}
	got, err := c.BridgeOut(t.Context(), chain.NetworkBase, usd(4))
	if err != nil || inner.calls != 1 || got.Uint64() != 3_996_001 {
		t.Fatalf("cached $4 = %s, %v after %d calls, want 3996001 after 1", got, err, inner.calls)
	}
}
