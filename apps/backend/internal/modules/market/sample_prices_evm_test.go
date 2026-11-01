package market_test

import (
	"context"
	"testing"
	"time"

	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/market/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/market/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/multichain"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/testkit/marketfake"
)

type evmPrices struct {
	perShare map[string]money.Micros
	sell     map[string]money.Micros
	err      error
	asked    []multichain.Listing
	during   func()
}

func (e *evmPrices) Prices(
	_ context.Context, listings []multichain.Listing,
) (map[multichain.ListingID]multichain.Price, error) {
	e.asked = append(e.asked, listings...)
	if e.during != nil {
		e.during()
	}
	out := map[multichain.ListingID]multichain.Price{}
	for _, l := range listings {
		if p, ok := e.perShare[l.TokenAddress]; ok {
			out[l.ID] = multichain.Price{PerShareMicros: p, AsOf: time.Unix(1_800_000_000, 0)}
		}
	}
	return out, e.err
}

func (e *evmPrices) SellPrices(
	_ context.Context, listings []multichain.Listing,
) (map[multichain.ListingID]multichain.Price, error) {
	out := map[multichain.ListingID]multichain.Price{}
	for _, l := range listings {
		if p, ok := e.sell[l.TokenAddress]; ok {
			out[l.ID] = multichain.Price{PerShareMicros: p, AsOf: time.Unix(1_800_000_000, 0)}
		}
	}
	return out, nil
}

type evmRig struct {
	*sampleRig
	quotes *quotes
	reader *sdkmetric.ManualReader
	token  string
	key    string
}

func newEVMRig(t *testing.T, source *evmPrices) *evmRig {
	t.Helper()
	return newEVMRigHot(
		t,
		source,
		250_000_000,
		func(_ int, held []chain.SolanaAddress) ([]chain.SolanaAddress, error) {
			return held, nil
		},
	)
}

func newEVMRigHot(
	t *testing.T, source *evmPrices, solanaMicros uint64,
	read func(call int, held []chain.SolanaAddress) ([]chain.SolanaAddress, error),
) *evmRig {
	t.Helper()
	calls := 0
	aapl := marketfake.AAPLx()
	q := &quotes{}
	q.quote(aapl, solanaMicros)
	var key string
	hot := []app.HotMints{func(context.Context) ([]chain.SolanaAddress, error) {
		calls++
		return read(calls, []chain.SolanaAddress{
			aapl.Mint.Address(), chain.SolanaAddress(key),
			chain.SolanaAddress("base:" + chain.NetworkBase.Cash().Address), "dogechain:0x1",
		})
	}}
	r := newHotSampleRig(t, fixed(q), hot, aapl)
	token := seedListing(t, r.pool, aapl, chain.NetworkBase)
	key = "base:" + token
	reader := sdkmetric.NewManualReader()
	gaps, err := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader)).Meter("market_test").
		Int64Counter("market_listing_price_gap_total")
	if err != nil {
		t.Fatal(err)
	}
	if source.perShare == nil {
		source.perShare = map[string]money.Micros{}
	}
	r.poller.WithEVM(app.EVMSampling{Source: source, Gaps: gaps})
	return &evmRig{sampleRig: r, quotes: q, reader: reader, token: token, key: key}
}

func (r *evmRig) gapCount(t *testing.T) int64 {
	t.Helper()
	var rm metricdata.ResourceMetrics
	if err := r.reader.Collect(t.Context(), &rm); err != nil {
		t.Fatal(err)
	}
	var n int64
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			if sum, ok := m.Data.(metricdata.Sum[int64]); ok && m.Name == "market_listing_price_gap_total" {
				for _, dp := range sum.DataPoints {
					n += dp.Value
				}
			}
		}
	}
	return n
}

func TestSamplePrices_storesAHeldEVMListingPricePerShareUnderItsLedgerKey(t *testing.T) {
	t.Parallel()
	source := &evmPrices{}
	r := newEVMRig(t, source)
	source.perShare[r.token] = money.MicrosFromUint64(252_000_000)

	report, err := r.poller.Tick(t.Context())
	if err != nil || attr(report.Attrs, "evm_priced") != "1" || attr(report.Attrs, "evm_gaps") != "" {
		t.Fatalf("Tick = %+v, %v, want the one held EVM listing priced and no gap at 0.8%%", report, err)
	}
	wantAskedOnly(t, source.asked, r.token)
	wantPoints(t, r.pool,
		pricePoint{marketfake.AAPLx().Mint.String(), r.bucket, 250_000_000, "jupiter"},
		pricePoint{r.key, r.bucket, 252_000_000, "uniswap"},
	)
	if n := r.gapCount(t); n != 0 {
		t.Fatalf("gap metric = %d, want 0", n)
	}
	wantListingPrice(t, r, 252_000_000)
}

func wantAskedOnly(t *testing.T, asked []multichain.Listing, token string) {
	t.Helper()
	if len(asked) != 1 || asked[0].TokenAddress != token || asked[0].SharesPerToken.String() != "5/4" {
		t.Fatalf("asked %+v, want only the held listing with its shares per token, not the EVM cash", asked)
	}
}

func wantListingPrice(t *testing.T, r *evmRig, micros uint64) {
	t.Helper()
	book := app.NewPriceBook(r.pool, r.clock)
	keys := []app.ListingKey{app.ListingKey(r.key), "base:0xunknown"}
	prices, err := book.ListingPrices(t.Context(), keys, r.clock.Now())
	if err != nil {
		t.Fatal(err)
	}
	got, ok := prices[app.ListingKey(r.key)]
	if len(prices) != 1 || !ok || got.Price == nil || got.Price.Micros.Uint64() != micros ||
		got.AssetID == nil || *got.AssetID != marketfake.AAPLx().ID {
		t.Fatalf("ListingPrices = %+v, want the held listing at %d micros a share, linked to AAPLx", prices, micros)
	}
}

func TestSamplePrices_flagsAnEVMListingMoreThanTwoPercentFromItsSolanaPrice(t *testing.T) {
	t.Parallel()
	source := &evmPrices{}
	r := newEVMRig(t, source)
	source.perShare[r.token] = money.MicrosFromUint64(255_250_000)

	report, err := r.poller.Tick(t.Context())
	if err != nil || attr(report.Attrs, "evm_gaps") != "AAPLxon@base" {
		t.Fatalf("Tick = %+v, %v, want AAPLxon@base flagged at 2.1%% off Solana", report, err)
	}
	if n := r.gapCount(t); n != 1 {
		t.Fatalf("gap metric = %d, want 1", n)
	}
}

func TestSamplePrices_aFailedEVMQuoteLeavesTheSolanaSampleAndTheTickGreen(t *testing.T) {
	t.Parallel()
	source := &evmPrices{err: errs.New(errs.CodeNotImplemented, "test")}
	r := newEVMRig(t, source)

	report, err := r.poller.Tick(t.Context())
	if err != nil || attr(report.Attrs, "evm_priced") != "0" || attr(report.Attrs, "evm_missing") != "1" ||
		attr(report.Attrs, "evm_error") != string(errs.CodeNotImplemented) {
		t.Fatalf("Tick = %+v, %v, want the EVM failure reported and the tick green", report, err)
	}
	wantPoints(t, r.pool, pricePoint{marketfake.AAPLx().Mint.String(), r.bucket, 250_000_000, "jupiter"})
}

func TestGapBps_isTheDistanceFromTheSolanaPrice(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		evm, solana uint64
		want        int64
	}{
		{evm: 102_000_000, solana: 100_000_000, want: 200},
		{evm: 97_990_000, solana: 100_000_000, want: 201},
		{evm: 100_000_000, solana: 100_000_000, want: 0},
	} {
		if got := app.GapBps(money.MicrosFromUint64(c.evm), money.MicrosFromUint64(c.solana)); got != c.want {
			t.Fatalf("GapBps(%d, %d) = %d, want %d", c.evm, c.solana, got, c.want)
		}
	}
}

func TestSamplePrices_aFailedReadOfTheHeldEVMListingsFailsTheTick(t *testing.T) {
	t.Parallel()
	down := errs.New(errs.CodeDBUnavailable, "test.held")
	source := &evmPrices{}
	r := newEVMRigHot(
		t,
		source,
		250_000_000,
		func(call int, held []chain.SolanaAddress) ([]chain.SolanaAddress, error) {
			if call > 1 {
				return nil, down
			}
			return held, nil
		},
	)
	if _, err := r.poller.Tick(t.Context()); errs.CodeOf(err) != errs.CodeDBUnavailable || len(source.asked) != 0 {
		t.Fatalf("Tick = %v after %d asks, want db_unavailable before asking", err, len(source.asked))
	}
}

func TestSamplePrices_aFailedListingReadFailsTheTickBeforeAsking(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(t.Context())
	source := &evmPrices{}
	r := newEVMRigHot(
		t,
		source,
		250_000_000,
		func(call int, held []chain.SolanaAddress) ([]chain.SolanaAddress, error) {
			if call > 1 {
				cancel()
			}
			return held, nil
		},
	)
	if _, err := r.poller.Tick(ctx); err == nil || len(source.asked) != 0 {
		t.Fatalf("Tick = %v after %d asks, want the listing read error before asking", err, len(source.asked))
	}
}

func TestSamplePrices_aFailedEVMInsertFailsTheTick(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(t.Context())
	source := &evmPrices{during: cancel}
	r := newEVMRig(t, source)
	source.perShare[r.token] = money.MicrosFromUint64(252_000_000)
	if _, err := r.poller.Tick(ctx); err == nil {
		t.Fatal("Tick whose EVM insert fails = nil, want an error")
	}
}

func (r *evmRig) nextTick(t *testing.T, after time.Duration) {
	t.Helper()
	r.clock.Advance(after)
	r.bucket = r.clock.Now().UTC().Truncate(domain.SampleBucket)
}

func (r *evmRig) unlink(t *testing.T) {
	t.Helper()
	if _, err := r.pool.Exec(t.Context(),
		`UPDATE asset_listings SET asset_id = NULL WHERE token_address = $1`, r.token); err != nil {
		t.Fatal(err)
	}
}

func TestSamplePrices_dropsAnEVMPriceFifteenPercentOffTheLastSolanaSampleWhenTheSiblingWasNotAskedThisTick(
	t *testing.T,
) {
	t.Parallel()
	source := &evmPrices{}
	r := newEVMRig(t, source)
	source.perShare[r.token] = money.MicrosFromUint64(252_000_000)
	if _, err := r.poller.Tick(t.Context()); err != nil {
		t.Fatal(err)
	}

	r.nextTick(t, 2*time.Minute)
	r.quotes.mu.Lock()
	r.quotes.prices = nil
	r.quotes.mu.Unlock()
	source.perShare[r.token] = money.MicrosFromUint64(287_500_000)
	report, err := r.poller.Tick(t.Context())
	if err != nil || attr(report.Attrs, "evm_priced") != "0" || attr(report.Attrs, "evm_missing") != "1" {
		t.Fatalf("Tick = %+v, %v, want the 15%% high quote dropped against the stored Solana sample", report, err)
	}
	wantPoints(t, r.pool,
		pricePoint{marketfake.AAPLx().Mint.String(), r.bucket.Add(-2 * time.Minute), 250_000_000, "jupiter"},
		pricePoint{r.key, r.bucket.Add(-2 * time.Minute), 252_000_000, "uniswap"},
	)
}

func TestSamplePrices_doesNotStoreAnEVMPriceWhenTheSolanaSampleIsOlderThanTheFreshnessWindow(t *testing.T) {
	t.Parallel()
	source := &evmPrices{}
	r := newEVMRig(t, source)
	source.perShare[r.token] = money.MicrosFromUint64(252_000_000)
	if _, err := r.poller.Tick(t.Context()); err != nil {
		t.Fatal(err)
	}

	r.nextTick(t, multichain.ListingPriceMaxAge+time.Minute)
	r.quotes.mu.Lock()
	r.quotes.prices = nil
	r.quotes.mu.Unlock()
	report, err := r.poller.Tick(t.Context())
	if err != nil || attr(report.Attrs, "evm_priced") != "0" {
		t.Fatalf("Tick = %+v, %v, want no EVM price stored without a fresh Solana sample or a sell quote", report, err)
	}
	if got := len(pricePoints(t, r.pool)); got != 2 {
		t.Fatalf("price_points = %d, want only the first tick's two", got)
	}
}

func TestSamplePrices_storesAnUnlinkedEVMListingOnlyWhenItsSellQuoteAgrees(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		name   string
		sell   map[string]money.Micros
		stored bool
	}{
		{name: "agreeing sell quote", sell: map[string]money.Micros{"": money.MicrosFromUint64(251_000_000)}, stored: true},
		{name: "sell quote far below", sell: map[string]money.Micros{"": money.MicrosFromUint64(200_000_000)}},
		{name: "no sell quote"},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			source := &evmPrices{}
			r := newEVMRig(t, source)
			r.unlink(t)
			source.perShare[r.token] = money.MicrosFromUint64(252_000_000)
			source.sell = map[string]money.Micros{}
			for range c.sell {
				source.sell[r.token] = c.sell[""]
			}
			report, err := r.poller.Tick(t.Context())
			want := "0"
			if c.stored {
				want = "1"
			}
			if err != nil || attr(report.Attrs, "evm_priced") != want {
				t.Fatalf("Tick = %+v, %v, want evm_priced %s", report, err, want)
			}
		})
	}
}
