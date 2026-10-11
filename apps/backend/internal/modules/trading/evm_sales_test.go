package trading_test

import (
	"context"
	"math/big"
	"net/http"
	"slices"
	"testing"
	"testing/synctest"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/trading/adapters"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/multichain"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/config"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpclient"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

type saleQuoter struct {
	gross func(micros uint64) *big.Int
	fail  error
	asked []uint64
}

func (q *saleQuoter) Quote(
	_ context.Context, _ multichain.Listing, side multichain.Side, amount money.Micros,
) (multichain.Quote, error) {
	if side != multichain.SideSell {
		return multichain.Quote{}, errs.New(errs.CodeInvalidInput, "saleQuoter")
	}
	q.asked = append(q.asked, amount.Uint64())
	if q.fail != nil && len(q.asked) == 2 {
		return multichain.Quote{}, q.fail
	}
	return multichain.Quote{GrossOutput: q.gross(amount.Uint64())}, nil
}

type saleListings struct {
	listing multichain.Listing
}

func (s saleListings) Listing(context.Context, multichain.ListingID) (multichain.Listing, error) {
	return multichain.Listing{}, errs.New(errs.CodeInvalidInput, "saleListings")
}

func (s saleListings) ListingByAsset(_ context.Context, asset string) (multichain.Listing, error) {
	if asset != "base:0xtoken" {
		return multichain.Listing{}, errs.New(errs.CodeAssetNotFound, "saleListings")
	}
	return s.listing, nil
}

func linearGross(unitsPerDollar int64, impactBps int64) func(uint64) *big.Int {
	return func(micros uint64) *big.Int {
		units := new(big.Int).Mul(new(big.Int).SetUint64(micros), big.NewInt(unitsPerDollar))
		units.Quo(units, big.NewInt(1_000_000))
		bump := new(big.Int).Mul(units, big.NewInt(impactBps))
		return units.Add(units, bump.Quo(bump, big.NewInt(10_000)))
	}
}

func enabledListing() multichain.Listing {
	return multichain.Listing{Symbol: "AAPLx", Network: chain.NetworkBase, TokenAddress: "0xtoken", Enabled: true}
}

func TestEVMSales_pricesHeldUnitsByScalingTheSellQuoteDownToWhatTheyCover(t *testing.T) {
	t.Parallel()
	q := &saleQuoter{gross: linearGross(1_000, 0)}
	sales := adapters.EVMSales{Listings: saleListings{listing: enabledListing()}, Quoter: q}

	sale, err := sales.Sale(t.Context(), "base:0xtoken", 50_000)

	if err != nil || sale.ProceedsMicros != 50_000_000 || sale.Listing.Symbol != "AAPLx" {
		t.Fatalf("Sale = %+v, %v; want 50 USDC for 50k units at 1k units a dollar", sale, err)
	}
	if !slices.Equal(q.asked, []uint64{1_000_000, 50_000_000}) {
		t.Fatalf("asked %v, want a $1 probe then the estimate", q.asked)
	}
}

func TestEVMSales_shrinksTheEstimateWhenImpactNeedsMoreUnitsThanHeld(t *testing.T) {
	t.Parallel()
	impact := func(micros uint64) *big.Int {
		if micros == 1_000_000 {
			return big.NewInt(1_000)
		}
		return linearGross(1_000, 100)(micros)
	}
	sales := adapters.EVMSales{Listings: saleListings{listing: enabledListing()}, Quoter: &saleQuoter{gross: impact}}

	sale, err := sales.Sale(t.Context(), "base:0xtoken", 50_000)

	if err != nil || sale.ProceedsMicros != 49_504_950 {
		t.Fatalf("Sale = %+v, %v; want the estimate scaled by 50000/50500", sale, err)
	}
}

func TestEVMSales_sellsNothingForADisabledListingOrNoUnits(t *testing.T) {
	t.Parallel()
	disabled := enabledListing()
	disabled.Enabled = false
	for name, c := range map[string]struct {
		listing multichain.Listing
		units   uint64
		gross   func(uint64) *big.Int
	}{
		"disabled":      {disabled, 50_000, linearGross(1_000, 0)},
		"no units":      {enabledListing(), 0, linearGross(1_000, 0)},
		"dust estimate": {enabledListing(), 1, linearGross(10_000_000, 0)},
	} {
		q := &saleQuoter{gross: c.gross}
		sale, err := adapters.EVMSales{Listings: saleListings{listing: c.listing}, Quoter: q}.
			Sale(t.Context(), "base:0xtoken", c.units)
		if err != nil || sale.ProceedsMicros != 0 || len(q.asked) > 1 {
			t.Errorf("%s: Sale = %+v, %v after %v; want zero proceeds", name, sale, err, q.asked)
		}
	}
}

func TestEVMSales_surfacesListingQuoteAndDecodeFailures(t *testing.T) {
	t.Parallel()
	none := func(uint64) *big.Int { return nil }
	zero := func(uint64) *big.Int { return new(big.Int) }
	one := func(uint64) *big.Int { return big.NewInt(1) }
	down := &saleQuoter{gross: linearGross(1_000, 0), fail: errs.New(errs.CodeUpstreamUnavailable, "uniswap")}
	for _, c := range []struct {
		name   string
		asset  string
		units  uint64
		quoter *saleQuoter
		want   errs.Code
	}{
		{"unknown asset", "base:0xother", 50_000, &saleQuoter{}, errs.CodeAssetNotFound},
		{"nil gross", "base:0xtoken", 50_000, &saleQuoter{gross: none}, errs.CodeDecodeFailed},
		{"zero gross", "base:0xtoken", 50_000, &saleQuoter{gross: zero}, errs.CodeDecodeFailed},
		{"overflow", "base:0xtoken", 1 << 62, &saleQuoter{gross: one}, errs.CodeDecodeFailed},
		{"requote down", "base:0xtoken", 50_000, down, errs.CodeUpstreamUnavailable},
	} {
		sales := adapters.EVMSales{Listings: saleListings{listing: enabledListing()}, Quoter: c.quoter}
		if _, err := sales.Sale(t.Context(), c.asset, c.units); errs.CodeOf(err) != c.want {
			t.Errorf("%s: err = %v, want %s", c.name, err, c.want)
		}
	}
}

func TestMultichainExecutionVenue_buildsOneRPCClientPerConfiguredEVMNetworkOnlyWithAKey(t *testing.T) {
	t.Parallel()
	cfg := testkit.Config()
	cfg.Timeouts.RPC = time.Second
	cfg.Routing = config.Routing{UniswapBaseURL: "https://uniswap.test", UniswapTimeout: time.Second}
	cfg.EVM = config.EVM{RPCURLBase: "https://base.test", RPCURLArbitrum: "https://arb.test"}
	var built []string
	recording := func(name string, opts ...httpclient.Option) *httpclient.Client {
		built = append(built, name)
		return httpclient.New(name, opts...)
	}
	d := module.Deps{Config: cfg, Clock: testkit.NewClock(time.Unix(0, 0)), HTTPClient: recording}

	if _, ok := adapters.MultichainExecutionVenue(d); ok || len(built) != 0 {
		t.Fatalf("keyless venue ok = %v after building %v, want unwired", ok, built)
	}
	d.Config.Routing.UniswapAPIKey = "key"
	venue, ok := adapters.MultichainExecutionVenue(d)
	slices.Sort(built)
	if !ok || venue == nil || !slices.Equal(built, []string{"evm-rpc-arbitrum", "evm-rpc-base", "uniswap-swap"}) {
		t.Fatalf("venue ok = %v, built %v; want base and arbitrum RPC plus uniswap", ok, built)
	}
	d.HTTPClient = nil
	if _, ok := adapters.MultichainExecutionVenue(d); !ok {
		t.Fatal("venue without an injected client factory is unwired, want the default factory")
	}
}

type hangingUpstream struct{}

func (hangingUpstream) RoundTrip(req *http.Request) (*http.Response, error) {
	<-req.Context().Done()
	return nil, req.Context().Err()
}

func TestMultichainExecutionVenue_swapsUnderTheExecutionTimeoutNotTheQuoteOne(t *testing.T) {
	t.Parallel()
	cfg := testkit.Config()
	cfg.Timeouts.RPC = time.Second
	cfg.Routing = config.Routing{
		UniswapAPIKey: "key", UniswapBaseURL: "https://uniswap.test",
		UniswapTimeout: 7 * time.Second, UniswapQuoteTimeout: 3 * time.Second,
	}
	opts := map[string][]httpclient.Option{}
	recording := func(name string, o ...httpclient.Option) *httpclient.Client {
		opts[name] = o
		return httpclient.New(name, o...)
	}
	adapters.MultichainExecutionVenue(
		module.Deps{Config: cfg, Clock: testkit.NewClock(time.Unix(0, 0)), HTTPClient: recording},
	)
	var took time.Duration
	synctest.Test(t, func(t *testing.T) {
		c := httpclient.New(
			"uniswap-swap",
			append(opts["uniswap-swap"], httpclient.WithTransport(hangingUpstream{}))...)
		req, _ := http.NewRequestWithContext(t.Context(), http.MethodGet, "/v1/swap", nil)
		start := clock.Real{}.Now()
		if resp, err := c.Do(t.Context(), req); err == nil {
			_ = resp.Body.Close()
			t.Fatal("a hanging upstream answered")
		}
		took = clock.Real{}.Now().Sub(start)
	})
	if took != 7*time.Second {
		t.Fatalf("the execution Uniswap client gives up after %v, want MONACO_TIMEOUT_UNISWAP's 7s", took)
	}
}
