package jupiter_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"math/big"
	"net/http"
	"os"
	"slices"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/jupiter"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/testkit/fakes"
	"github.com/monaco/monaco/apps/backend/internal/testkit/jupiterfake"
)

const priceFixtures = "../../../testkit/fakes/testdata/fakes/jupiter/price/"

func fixtureMint(i int) jupiter.Mint {
	return jupiter.Mint{Address: fmt.Sprintf("Xs%02dPriceFixtureMint1111111111111111111111", i), Decimals: 8}
}

func fixtureBody(t *testing.T, file string) string {
	t.Helper()
	raw, err := fs.ReadFile(os.DirFS(priceFixtures), file)
	if err != nil {
		t.Fatal(err)
	}
	var f struct {
		Body json.RawMessage `json:"body"`
	}
	if err := json.Unmarshal(raw, &f); err != nil {
		t.Fatal(err)
	}
	return string(f.Body)
}

func TestPrices_BatchesOf50(t *testing.T) {
	t.Parallel()
	first, second := fixtureBody(t, "v3.json"), fixtureBody(t, "v3/batch-2.json")
	u := &upstream{handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body := first
		if !strings.Contains(r.URL.Query().Get("ids"), fixtureMint(1).Address) {
			body = second
		}
		_, _ = w.Write([]byte(body))
	})}
	mints := make([]jupiter.Mint, 55)
	for i := range mints {
		mints[i] = fixtureMint(i + 1)
	}
	start := now()

	got, err := client(u).Prices(t.Context(), mints)
	if err != nil {
		t.Fatal(err)
	}
	wantBatches(t, u.requests(), 50, 5)
	if len(got) != 55 {
		t.Fatalf("got %d prices, want 55", len(got))
	}
	for i := 1; i <= 55; i++ {
		m := fixtureMint(i)
		want := money.MicrosFromUint64(uint64((10+7*i)*1_000_000 + (i*123457)%1_000_000))
		if p := got[m]; p.Mint != m || p.USDMicros != want || p.ObservedAt.Before(start) {
			t.Fatalf("price %d = %+v, want %v micros", i, p, want)
		}
	}
}

func wantBatches(t *testing.T, sent []sent, sizes ...int) {
	t.Helper()
	got := make([]int, 0, len(sent))
	for _, s := range sent {
		got = append(got, len(strings.Split(s.query.Get("ids"), ",")))
		if s.path != "/jupiter/price/v3" || s.apiKey != "test-key" {
			t.Fatalf("request = %+v, want GET /jupiter/price/v3 with the key", s)
		}
	}
	slices.Sort(got)
	slices.Sort(sizes)
	if !slices.Equal(got, sizes) {
		t.Fatalf("price calls carried %v mints, want %v", got, sizes)
	}
}

func TestPrices_missingOrNullMintIsAbsentNotZero(t *testing.T) {
	t.Parallel()
	a, b, c := fixtureMint(1), fixtureMint(2), fixtureMint(3)
	u := replying(http.StatusOK, `{"`+a.Address+`":{"usdPrice":"1.5"},"`+b.Address+`":null,"Other":{"usdPrice":9}}`)
	got, err := client(u).Prices(t.Context(), []jupiter.Mint{a, b, c})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[a].USDMicros != money.MicrosFromUint64(1_500_000) {
		t.Fatalf("Prices = %+v, want only %s at 1500000 micros", got, a.Address)
	}
	if _, ok := got[b]; ok {
		t.Fatalf("null mint %s is present", b.Address)
	}
}

func TestPrices_stockDataPriceWhenUSDPriceIsAbsent(t *testing.T) {
	t.Parallel()
	stock, plain := fixtureMint(1), fixtureMint(2)
	u := replying(http.StatusOK, `{"`+stock.Address+`":{"createdAt":"2026-09-03T18:11:57Z","decimals":8,`+
		`"stockData":{"id":"xstocks","price":8.125,"mcap":516152903.35,"updatedAt":"2026-10-05T17:09:33.658Z"}},`+
		`"`+plain.Address+`":{"usdPrice":108.83,"stockData":{"price":1}}}`)
	got, err := client(u).Prices(t.Context(), []jupiter.Mint{stock, plain})
	if err != nil {
		t.Fatal(err)
	}
	if got[stock].USDMicros.Uint64() != 8_125_000 || got[plain].USDMicros.Uint64() != 108_830_000 {
		t.Fatalf("Prices = %+v, want 8125000 from stockData and 108830000 from usdPrice", got)
	}
}

func TestPrices_unpriceableEntryIsSkippedAndTheBatchKept(t *testing.T) {
	t.Parallel()
	good, negative, empty, nothing := fixtureMint(1), fixtureMint(2), fixtureMint(3), fixtureMint(4)
	u := replying(http.StatusOK, `{"`+good.Address+`":{"stockData":{"price":0.000123}},`+
		`"`+negative.Address+`":{"usdPrice":-1},`+
		`"`+empty.Address+`":{"stockData":{"price":"abc"}},`+
		`"`+nothing.Address+`":{"decimals":8}}`)
	got, err := client(u).Prices(t.Context(), []jupiter.Mint{good, negative, empty, nothing})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[good].USDMicros.Uint64() != 123 {
		t.Fatalf("Prices = %+v, want only %s at 123 micros", got, good.Address)
	}
}

func TestPrices_noMintsMakesNoCall(t *testing.T) {
	t.Parallel()
	u := replying(http.StatusOK, `{}`)
	got, err := client(u).Prices(t.Context(), nil)
	if err != nil || len(got) != 0 || len(u.requests()) != 0 {
		t.Fatalf("Prices(nil) = %v, %v after %d calls", got, err, len(u.requests()))
	}
}

func TestPrices_failures(t *testing.T) {
	t.Parallel()
	m := fixtureMint(1)
	for name, tc := range map[string]struct {
		u    *upstream
		want errs.Code
	}{
		"unauthorized": {replying(http.StatusUnauthorized, `Unauthorized`), errs.CodeJupiterRejected},
		"server error": {replying(http.StatusInternalServerError, ``), errs.CodeJupiterUnavailable},
		"not json":     {replying(http.StatusOK, `{"`), errs.CodeDecodeFailed},
	} {
		_, err := client(tc.u).Prices(t.Context(), []jupiter.Mint{m})
		if errs.CodeOf(err) != tc.want {
			t.Fatalf("%s: err = %v, want %s", name, err, tc.want)
		}
	}
}

func TestPrices_oneFailedBatchFailsTheCall(t *testing.T) {
	t.Parallel()
	var calls atomic.Int32
	u := &upstream{handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if calls.Add(1) == 2 {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		_, _ = w.Write([]byte(`{}`))
	})}
	mints := make([]jupiter.Mint, 51)
	for i := range mints {
		mints[i] = fixtureMint(i + 1)
	}
	if _, err := client(u).Prices(t.Context(), mints); errs.CodeOf(err) != errs.CodeJupiterRejected {
		t.Fatalf("err = %v, want jupiter_rejected", err)
	}
}

func TestPrices_keepsTheBatchesThatAnsweredAndReturnsTheFailedBatchsCode(t *testing.T) {
	t.Parallel()
	first := fixtureBody(t, "v3.json")
	u := &upstream{handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Query().Get("ids"), fixtureMint(51).Address) {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		_, _ = w.Write([]byte(first))
	})}
	mints := make([]jupiter.Mint, 51)
	for i := range mints {
		mints[i] = fixtureMint(i + 1)
	}
	got, err := client(u).Prices(t.Context(), mints)
	if errs.CodeOf(err) != errs.CodeJupiterUnavailable || attr(err, "failed_batches") != "1" {
		t.Fatalf("err = %v, want jupiter_unavailable from one failed batch", err)
	}
	if _, ok := got[fixtureMint(51)]; ok || len(got) != 50 || got[fixtureMint(1)].USDMicros.Uint64() != 17_123_457 {
		t.Fatalf("Prices kept %d prices, want the 50 of the batch that answered", len(got))
	}
}

func TestPrices_cancelledCallAsksNothing(t *testing.T) {
	t.Parallel()
	u := replying(http.StatusOK, `{}`)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	got, err := client(u).Prices(ctx, []jupiter.Mint{fixtureMint(1)})
	if !errors.Is(err, context.Canceled) || got != nil || len(u.requests()) != 0 {
		t.Fatalf("Prices on a cancelled context = %v, %v after %d calls", got, err, len(u.requests()))
	}
}

func TestParsePrice_roundsDownToMicros(t *testing.T) {
	t.Parallel()
	for raw, want := range map[string]uint64{
		"0":                0,
		"1":                1_000_000,
		"212.3456789":      212_345_678,
		"108.83":           108_830_000,
		"0.000123":         123,
		"0.0000009":        0,
		"1.2e-5":           12,
		"18446744073709.5": 18_446_744_073_709_500_000,
	} {
		got, ok := jupiter.ParsePrice(raw)
		if !ok || got.Uint64() != want {
			t.Fatalf("ParsePrice(%q) = %d, %v, want %d", raw, got.Uint64(), ok, want)
		}
	}
	for _, raw := range []string{"-1", "abc", "1e1000", "18446744073709.552", ""} {
		if _, ok := jupiter.ParsePrice(raw); ok {
			t.Fatalf("ParsePrice(%q) accepted", raw)
		}
	}
}

func FuzzParsePrice(f *testing.F) {
	for _, seed := range []string{"212.3456789", "0", "-0.5", "1.2e-7", "9e18", "1e-999", "NaN", "0x10", "1_0"} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, raw string) {
		got, ok := jupiter.ParsePrice(raw)
		if !ok {
			return
		}
		r, parsed := new(big.Rat).SetString(raw)
		if !parsed || r.Sign() < 0 {
			t.Fatalf("ParsePrice(%q) accepted a non-price", raw)
		}
		micros := new(big.Rat).SetUint64(got.Uint64())
		scaled := new(big.Rat).Mul(r, big.NewRat(1_000_000, 1))
		if micros.Cmp(scaled) > 0 || new(big.Rat).Add(micros, big.NewRat(1, 1)).Cmp(scaled) <= 0 {
			t.Fatalf("ParsePrice(%q) = %d micros, want %s rounded down", raw, got.Uint64(), scaled.FloatString(3))
		}
	})
}

func catalog(n int) []jupiter.Mint {
	mints := make([]jupiter.Mint, n)
	for i := range mints {
		mints[i] = jupiter.Mint{Address: fmt.Sprintf("Xs%04dCatalogMint11111111111111111111111", i), Decimals: 8}
	}
	return mints
}

func mainnetLimit() *jupiterfake.PriceAPI {
	return &jupiterfake.PriceAPI{Clock: clock.Real{}, Limit: 10, Window: 10 * time.Second}
}

func TestPrices_wholeCatalogStaysUnderTheKeysRateLimit(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		api := mainnetLimit()
		u := &upstream{handler: api}
		start := now()

		got, err := client(u).Prices(t.Context(), catalog(1282))
		if err != nil || len(got) != 1282 {
			t.Fatalf("Prices = %d prices, %v, want all 1282 and no error", len(got), err)
		}
		if api.Limited() != 0 || len(u.requests()) != 26 {
			t.Fatalf("%d of %d requests were rate limited, want 0 of 26", api.Limited(), len(u.requests()))
		}
		if took := now().Sub(start); took >= 2*time.Minute {
			t.Fatalf("pricing the catalog took %v, want under the 2m poll interval", took)
		}
	})
}

func TestOrder_takesTheNextTokenAheadOfQueuedPriceBatches(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		prices, swaps := mainnetLimit(), fakes.New()
		u := &upstream{handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == orderRoute {
				swaps.ServeHTTP(w, r)
				return
			}
			prices.ServeHTTP(w, r)
		})}
		c := client(u)
		done := make(chan error, 1)
		go func() {
			_, err := c.Prices(t.Context(), catalog(1000))
			done <- err
		}()
		synctest.Wait()
		asked := now()

		_, err := c.Order(t.Context(), jupiter.OrderSpec{
			In: usdc(), Out: aaplx(), Amount: units(25_000_000, usdc()), Taker: treasury, SlippageBps: 50,
		})
		if err != nil {
			t.Fatal(err)
		}
		if err := <-done; err != nil {
			t.Fatal(err)
		}
		reqs := u.requests()
		i := slices.IndexFunc(reqs, func(s sent) bool { return s.path == orderRoute })
		if i < 1 || i+1 >= len(reqs) || !reqs[i].at.Equal(asked) || !reqs[i+1].at.After(asked) {
			t.Fatalf("order went out as request %d of %d, %v after it was asked, want at once",
				i, len(reqs), reqs[max(i, 0)].at.Sub(asked))
		}
		if prices.Limited() != 0 {
			t.Fatalf("%d price requests were rate limited, want 0", prices.Limited())
		}
	})
}

func TestPrices_rateLimitedBatchRetriesWhenTheWindowResets(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		for resetIn, want := range map[time.Duration]time.Duration{
			7 * time.Second:  7 * time.Second,
			60 * time.Second: 15 * time.Second,
		} {
			reset := now().Add(resetIn).Unix()
			var calls atomic.Int32
			u := &upstream{handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				if calls.Add(1) == 1 {
					w.Header().Set("x-ratelimit-reset", strconv.FormatInt(reset, 10))
					w.WriteHeader(http.StatusTooManyRequests)
					return
				}
				_, _ = w.Write([]byte(`{"` + fixtureMint(1).Address + `":{"usdPrice":2}}`))
			})}

			got, err := client(u).Prices(t.Context(), []jupiter.Mint{fixtureMint(1)})
			if err != nil || got[fixtureMint(1)].USDMicros.Uint64() != 2_000_000 {
				t.Fatalf("reset in %v: Prices = %+v, %v, want the retried batch's price", resetIn, got, err)
			}
			sent := u.requests()
			if len(sent) != 2 || sent[1].at.Sub(sent[0].at) != want {
				t.Fatalf("reset in %v: %d requests, retry after %v, want 2 with the retry after %v",
					resetIn, len(sent), sent[len(sent)-1].at.Sub(sent[0].at), want)
			}
		}
	})
}

func TestPrices_cancelledWhileWaitingForATokenGivesItsPlaceBack(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		u := &upstream{handler: mainnetLimit()}
		c := client(u)
		ctx, cancel := context.WithCancel(t.Context())
		done := make(chan error, 1)
		go func() {
			_, err := c.Prices(ctx, catalog(1000))
			done <- err
		}()
		synctest.Wait()
		cancel()
		if err := <-done; !errors.Is(err, context.Canceled) {
			t.Fatalf("cancelled Prices = %v, want context.Canceled", err)
		}
		first := len(u.requests())
		<-time.After(10 * time.Second)
		asked := now()

		if _, err := c.Prices(t.Context(), catalog(300)); err != nil {
			t.Fatal(err)
		}
		for _, s := range u.requests()[first:] {
			if !s.at.Equal(asked) {
				t.Fatalf("a batch went out %v after the call, want all 6 at once", s.at.Sub(asked))
			}
		}
	})
}

func TestPrices_cancelledDuringTheRateLimitWaitStops(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		reset := now().Add(7 * time.Second).Unix()
		u := &upstream{handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("x-ratelimit-reset", strconv.FormatInt(reset, 10))
			w.WriteHeader(http.StatusTooManyRequests)
		})}
		ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
		defer cancel()
		start := now()

		_, err := client(u).Prices(ctx, []jupiter.Mint{fixtureMint(1)})
		if !errors.Is(err, context.DeadlineExceeded) || len(u.requests()) != 1 || now().Sub(start) != 3*time.Second {
			t.Fatalf("Prices = %v after %d requests and %v, want the 3s deadline after 1 request",
				err, len(u.requests()), now().Sub(start))
		}
	})
}

func TestPrices_aContextThatRunsOutIsAnUpstreamTimeout(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	_, err := client(replying(http.StatusOK, `{}`)).Prices(ctx, []jupiter.Mint{fixtureMint(1)})
	if errs.CodeOf(err) != errs.CodeUpstreamTimeout {
		t.Fatalf("Prices on a spent context = %v, want upstream_timeout", err)
	}
}

func TestPrices_configuredRateLimitSendsTheCatalogWithoutWaiting(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		u := &upstream{handler: &jupiterfake.PriceAPI{Clock: clock.Real{}, Limit: 1000, Window: 10 * time.Second}}
		cfg := testConfig()
		cfg.Jupiter.RateLimit = 1000
		start := now()

		got, err := clientWith(cfg, u).Prices(t.Context(), catalog(1282))
		if err != nil || len(got) != 1282 {
			t.Fatalf("Prices = %d prices, %v, want all 1282 and no error", len(got), err)
		}
		if took := now().Sub(start); took != 0 {
			t.Fatalf("26 requests under a limit of 1000 took %v, want no wait", took)
		}
	})
}
