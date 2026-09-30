package market_test

import (
	"bytes"
	"cmp"
	"context"
	"crypto/sha256"
	"encoding/json"
	"log/slog"
	"math"
	"net/http"
	"net/http/httptest"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/market"
	"github.com/monaco/monaco/apps/backend/internal/modules/market/adapters/jupiterprices"
	"github.com/monaco/monaco/apps/backend/internal/modules/market/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/market/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/jupiter"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/config"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
	"github.com/monaco/monaco/apps/backend/internal/testkit/fakes"
	"github.com/monaco/monaco/apps/backend/internal/testkit/marketfake"
)

const jupiterPriceRoute = "/jupiter/price/v3"

type quotes struct {
	mu     sync.Mutex
	prices map[domain.Mint]money.Micros
	err    error
	calls  int
	during func()
}

func (q *quotes) Prices(_ context.Context, mints []domain.Mint) (map[domain.Mint]money.Micros, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.calls++
	if q.during != nil {
		q.during()
	}
	out := map[domain.Mint]money.Micros{}
	for _, m := range mints {
		if p, ok := q.prices[m]; ok {
			out[m] = p
		}
	}
	return out, q.err
}

func (q *quotes) quote(a market.Asset, micros uint64) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.prices == nil {
		q.prices = map[domain.Mint]money.Micros{}
	}
	q.prices[a.Mint] = usd(micros)
}

type tickRecorder struct {
	mu   sync.Mutex
	sent []events.PriceTick
	err  error
}

func (r *tickRecorder) PublishCore(_ context.Context, m events.Core) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.err != nil {
		return r.err
	}
	tick, _ := m.(events.PriceTick)
	r.sent = append(r.sent, tick)
	return nil
}

func (r *tickRecorder) published() []events.PriceTick {
	r.mu.Lock()
	defer r.mu.Unlock()
	return slices.Clone(r.sent)
}

type sampleRig struct {
	pool   *pgxpool.Pool
	clock  *testkit.Clock
	bucket time.Time
	ticks  *tickRecorder
	poller *app.SamplePrices
}

func newSampleRig(t *testing.T, source func(clock.Clock) app.PriceSource, assets ...market.Asset) *sampleRig {
	t.Helper()
	pool := testkit.DB(t)
	bucket := clock.Real{}.Now().UTC().Truncate(domain.SampleBucket)
	rows := make([][]any, len(assets))
	for i, a := range assets {
		rows[i] = []any{
			a.ID.UUID(), a.Symbol, a.Mint.String(), int16(a.Decimals), string(a.Issuer), string(a.Kind),
			a.DisplayName, a.IssuerTradable, a.CompanyKey, bucket, bucket,
		}
	}
	columns := []string{
		"id", "symbol", "mint", "decimals", "issuer", "kind", "display_name", "issuer_tradable", "company_key",
		"first_seen_at", "updated_at",
	}
	if _, err := pool.CopyFrom(t.Context(), pgx.Identifier{"assets"}, columns, pgx.CopyFromRows(rows)); err != nil {
		t.Fatal(err)
	}
	clk := testkit.NewClock(bucket.Add(37 * time.Second))
	ids := testkit.NewIDs(11)
	ticks := &tickRecorder{}
	return &sampleRig{
		pool: pool, clock: clk, bucket: bucket, ticks: ticks,
		poller: app.NewSamplePrices(db.New(pool, ids, clk), pool, clk, source(clk), ticks, 2*time.Minute),
	}
}

func fixed(q *quotes) func(clock.Clock) app.PriceSource {
	return func(clock.Clock) app.PriceSource { return q }
}

type jupiterFake struct {
	fakes *fakes.Server
	calls atomic.Int32
	url   string
}

func fakeJupiter(t *testing.T, steps ...fakes.Step) *jupiterFake {
	t.Helper()
	j := &jupiterFake{fakes: fakes.New()}
	for _, step := range steps {
		raw, err := json.Marshal(step)
		if err != nil {
			t.Fatal(err)
		}
		rec := httptest.NewRecorder()
		j.fakes.ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/_script",
			bytes.NewReader(raw)))
		if rec.Code != http.StatusNoContent {
			t.Fatalf("script %s = %d %q", raw, rec.Code, rec.Body.String())
		}
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == jupiterPriceRoute {
			j.calls.Add(1)
		}
		j.fakes.ServeHTTP(w, r)
	}))
	t.Cleanup(srv.Close)
	j.url = srv.URL
	return j
}

func (j *jupiterFake) source(quoteTimeout time.Duration) func(clock.Clock) app.PriceSource {
	return func(c clock.Clock) app.PriceSource {
		cfg := config.Config{
			Jupiter: config.Jupiter{
				SwapBaseURL: j.url + "/jupiter/swap/v2", PriceBaseURL: j.url + jupiterPriceRoute, APIKey: "test-key",
			},
			Timeouts: config.Timeouts{JupiterQuote: quoteTimeout, JupiterExecute: time.Minute},
		}
		return jupiterprices.New(jupiter.New(cfg, c))
	}
}

type pricePoint struct {
	Mint   string
	TS     time.Time
	Micros int64
	Source string
}

func pricePoints(t *testing.T, pool *pgxpool.Pool) []pricePoint {
	t.Helper()
	rows, err := pool.Query(
		t.Context(),
		`SELECT mint, ts, price_micros, source FROM price_points ORDER BY mint COLLATE "C", ts`,
	)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []pricePoint
	for rows.Next() {
		var p pricePoint
		if err := rows.Scan(&p.Mint, &p.TS, &p.Micros, &p.Source); err != nil {
			t.Fatal(err)
		}
		out = append(out, p)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}

func wantPoints(t *testing.T, pool *pgxpool.Pool, want ...pricePoint) {
	t.Helper()
	got := pricePoints(t, pool)
	slices.SortFunc(want, func(a, b pricePoint) int {
		return cmp.Or(strings.Compare(a.Mint, b.Mint), a.TS.Compare(b.TS))
	})
	same := slices.EqualFunc(got, want, func(a, b pricePoint) bool {
		return a.Mint == b.Mint && a.TS.Equal(b.TS) && a.Micros == b.Micros && a.Source == b.Source
	})
	if !same {
		t.Fatalf("price_points = %+v, want %+v", got, want)
	}
}

func attr(attrs []slog.Attr, key string) string {
	for _, a := range attrs {
		if a.Key == key {
			return a.Value.String()
		}
	}
	return ""
}

func tickPrice(a market.Asset, micros uint64, observed time.Time) events.TickPrice {
	return events.TickPrice{
		Mint:        a.Mint.Address(),
		AssetID:     a.ID.UUID(),
		PriceMicros: usd(micros),
		ObservedAt:  observed,
	}
}

func wantTick(t *testing.T, got events.PriceTick, asOf time.Time, want ...events.TickPrice) {
	t.Helper()
	same := slices.EqualFunc(got.Prices, want, func(a, b events.TickPrice) bool {
		return a.Mint == b.Mint && a.AssetID == b.AssetID && a.PriceMicros == b.PriceMicros &&
			a.ObservedAt.Equal(b.ObservedAt)
	})
	if got.V != 1 || !got.AsOf.Equal(asOf) || !same {
		t.Fatalf("price.tick = %+v, want v1 as of %v with %+v", got, asOf, want)
	}
}

func TestSamplePrices_writesEachPricedMintInItsBucketPausedMintsIncluded(t *testing.T) {
	t.Parallel()
	aapl, tsla, jpst := marketfake.AAPLx(), marketfake.TSLAx(), marketfake.JPSTx()
	q := &quotes{}
	q.quote(aapl, 254_371_234)
	q.quote(jpst, 50_120_000)
	r := newSampleRig(t, fixed(q), aapl, tsla, jpst)
	if r.poller.Name() != "market.prices" || r.poller.Interval() != 2*time.Minute {
		t.Fatalf("poller %s every %s, want market.prices every 2m", r.poller.Name(), r.poller.Interval())
	}
	report, err := r.poller.Tick(t.Context())
	if err != nil || report.Scanned != 3 || report.Changed != 2 || attr(report.Attrs, "priced") != "2" ||
		attr(report.Attrs, "missing") != "1" {
		t.Fatalf("Tick = %+v, %v, want 3 scanned, 2 written and TSLAx counted missing", report, err)
	}
	wantPoints(t, r.pool,
		pricePoint{aapl.Mint.String(), r.bucket, 254_371_234, "jupiter"},
		pricePoint{jpst.Mint.String(), r.bucket, 50_120_000, "jupiter"},
	)
	sent := r.ticks.published()
	if len(sent) != 1 {
		t.Fatalf("published %d price.tick messages, want 1", len(sent))
	}
	wantTick(t, sent[0], r.bucket, tickPrice(aapl, 254_371_234, r.bucket), tickPrice(jpst, 50_120_000, r.bucket))
}

func TestSamplePrices_aRestartInTheSameBucketLeavesOneRowPerMint(t *testing.T) {
	t.Parallel()
	aapl := marketfake.AAPLx()
	q := &quotes{}
	q.quote(aapl, 200_000_000)
	r := newSampleRig(t, fixed(q), aapl)
	if _, err := r.poller.Tick(t.Context()); err != nil {
		t.Fatal(err)
	}
	r.clock.Advance(time.Minute)
	q.quote(aapl, 201_000_000)
	report, err := r.poller.Tick(t.Context())
	if err != nil || report.Changed != 0 {
		t.Fatalf("second tick in the bucket = %+v, %v, want nothing written", report, err)
	}
	wantPoints(t, r.pool, pricePoint{aapl.Mint.String(), r.bucket, 200_000_000, "jupiter"})
	if sent := r.ticks.published(); len(sent) != 2 {
		t.Fatalf("published %d price.tick messages over two ticks, want 2", len(sent))
	}
}

func TestSamplePrices_publishesAcceptedPricesSoASpikeWaitsForTheNextSample(t *testing.T) {
	t.Parallel()
	aapl := marketfake.AAPLx()
	q := &quotes{}
	r := newSampleRig(t, fixed(q), aapl)
	for _, micros := range []uint64{200_000_000, 250_000_000, 250_000_000} {
		q.quote(aapl, micros)
		if _, err := r.poller.Tick(t.Context()); err != nil {
			t.Fatal(err)
		}
		r.clock.Advance(domain.SampleBucket)
	}
	second, third := r.bucket.Add(domain.SampleBucket), r.bucket.Add(2*domain.SampleBucket)
	sent := r.ticks.published()
	if len(sent) != 3 {
		t.Fatalf("published %d price.tick messages over three ticks, want 3", len(sent))
	}
	wantTick(t, sent[1], second, tickPrice(aapl, 200_000_000, r.bucket))
	wantTick(t, sent[2], third, tickPrice(aapl, 250_000_000, third))
}

func TestSamplePrices_keepsWhatCameBackAndReturnsTheFailedBatchsCode(t *testing.T) {
	t.Parallel()
	aapl, tsla := marketfake.AAPLx(), marketfake.TSLAx()
	q := &quotes{err: errs.New(errs.CodeJupiterUnavailable, "test.batch")}
	q.quote(aapl, 254_371_234)
	r := newSampleRig(t, fixed(q), aapl, tsla)
	_, err := r.poller.Tick(t.Context())
	detail := errs.Detail(err)
	if errs.CodeOf(err) != errs.CodeJupiterUnavailable || attr(detail, "written") != "1" ||
		attr(detail, "missing") != "1" {
		t.Fatalf("Tick err = %v %v, want jupiter_unavailable with 1 written and 1 missing", err, detail)
	}
	wantPoints(t, r.pool, pricePoint{aapl.Mint.String(), r.bucket, 254_371_234, "jupiter"})
	if sent := r.ticks.published(); len(sent) != 1 {
		t.Fatalf("published %d price.tick messages, want the tick after the partial insert", len(sent))
	}
}

func generatedAsset(t *testing.T, g *testkit.IDs, i int) market.Asset {
	t.Helper()
	key := sha256.Sum256([]byte("asset-" + strconv.Itoa(i)))
	m, err := domain.ParseMint(string(chain.AddressOf(key[:])))
	if err != nil {
		t.Fatal(err)
	}
	symbol := "G" + strconv.Itoa(1000+i) + "x"
	return market.Asset{
		ID: domain.NewAssetID(g), Symbol: symbol, Mint: m, Decimals: 8, Issuer: domain.IssuerXStocks,
		Kind: domain.KindEquity, DisplayName: symbol, UIMultiplier: domain.Multiplier{Num: 1, Den: 1},
		IssuerTradable: true, Override: domain.OverrideAuto, CompanyKey: strings.ToLower(symbol),
	}
}

func TestSamplePrices_120MintsTakeThreeJupiterCallsAndOneTick(t *testing.T) {
	t.Parallel()
	g := testkit.NewIDs(120)
	assets := make([]market.Asset, 120)
	for i := range assets {
		assets[i] = generatedAsset(t, g, i)
	}
	j := fakeJupiter(t)
	r := newSampleRig(t, j.source(5*time.Second), assets...)
	report, err := r.poller.Tick(t.Context())
	if err != nil || report.Scanned != 120 || attr(report.Attrs, "missing") != "120" {
		t.Fatalf("Tick over 120 unpriced mints = %+v, %v", report, err)
	}
	if calls, sent := j.calls.Load(), r.ticks.published(); calls != 3 || len(sent) != 1 || len(sent[0].Prices) != 0 {
		t.Fatalf("one tick over 120 mints made %d Jupiter calls and %d price.tick messages, want 3 and 1",
			calls, len(sent))
	}
}

func TestSamplePrices_aPriceTooLargeToStoreCountsAsMissing(t *testing.T) {
	t.Parallel()
	aapl := marketfake.AAPLx()
	q := &quotes{prices: map[domain.Mint]money.Micros{aapl.Mint: money.MicrosFromUint64(math.MaxInt64 + 1)}}
	r := newSampleRig(t, fixed(q), aapl)
	report, err := r.poller.Tick(t.Context())
	if err != nil || report.Changed != 0 || attr(report.Attrs, "missing") != "1" {
		t.Fatalf("Tick with an unstorable price = %+v, %v, want it counted missing", report, err)
	}
	wantPoints(t, r.pool)
}

func TestSamplePrices_failsBeforeAskingWhenTheCatalogCannotBeRead(t *testing.T) {
	t.Parallel()
	q := &quotes{}
	r := newSampleRig(t, fixed(q), marketfake.AAPLx())
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := r.poller.Tick(ctx); err == nil || q.calls != 0 || len(r.ticks.published()) != 0 {
		t.Fatalf("Tick on a cancelled context = %v after %d price calls, want an error and no call", err, q.calls)
	}
}

func TestSamplePrices_aFailedInsertPublishesNothing(t *testing.T) {
	t.Parallel()
	aapl := marketfake.AAPLx()
	ctx, cancel := context.WithCancel(t.Context())
	q := &quotes{during: cancel}
	q.quote(aapl, 254_371_234)
	r := newSampleRig(t, fixed(q), aapl)
	if _, err := r.poller.Tick(ctx); err == nil || len(r.ticks.published()) != 0 {
		t.Fatalf("Tick whose insert fails = %v with %d ticks, want an error and no tick", err, len(r.ticks.published()))
	}
	wantPoints(t, r.pool)
}

func TestSamplePrices_aStoredPriceThatDoesNotDecodeFailsTheTick(t *testing.T) {
	t.Parallel()
	aapl, tsla := marketfake.AAPLx(), marketfake.TSLAx()
	q := &quotes{}
	q.quote(aapl, 254_371_234)
	r := newSampleRig(t, fixed(q), aapl, tsla)
	_, err := r.pool.Exec(t.Context(),
		`INSERT INTO price_points (mint, ts, price_micros, source) VALUES ($1, $2, -1, 'jupiter')`,
		tsla.Mint.String(), r.bucket.Add(-domain.SampleBucket))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.poller.Tick(
		t.Context(),
	); errs.CodeOf(err) != errs.CodeDecodeFailed ||
		len(r.ticks.published()) != 0 {
		t.Fatalf("Tick over a negative stored price = %v, want decode_failed and no tick", err)
	}
}

func TestSamplePrices_aFailedPublishFailsTheTickAfterTheInsert(t *testing.T) {
	t.Parallel()
	aapl := marketfake.AAPLx()
	q := &quotes{}
	q.quote(aapl, 254_371_234)
	r := newSampleRig(t, fixed(q), aapl)
	r.ticks.err = errs.New(errs.CodeUpstreamUnavailable, "test.publish")
	if _, err := r.poller.Tick(t.Context()); errs.CodeOf(err) != errs.CodeUpstreamUnavailable {
		t.Fatalf("Tick whose publish fails = %v, want upstream_unavailable", err)
	}
	wantPoints(t, r.pool, pricePoint{aapl.Mint.String(), r.bucket, 254_371_234, "jupiter"})
}
