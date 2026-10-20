package coingecko_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/market/adapters/coingecko"
	"github.com/monaco/monaco/apps/backend/internal/modules/market/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/market/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/concurrency"
	"github.com/monaco/monaco/apps/backend/internal/platform/config"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpclient"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
	"github.com/monaco/monaco/apps/backend/internal/testkit/fakes"
)

const (
	aaplx      = "XsbEhLAtcf6HdfpFZ5xEMdqW8nfAvcsP5bdudRLJzJp"
	tspacex    = "TSPXcLV76s6V2zDiZQ18kBfcbnjaE2ZzNT3ga2Pd99v"
	anduril    = "PresTj4Yc2bAR197Er7wz4UUKSfqt6FryBEdAriBoQB"
	unlisted   = "USxhdMQhkMoYC52vEK6giwuQeu2F5SWuXeqS3EeyidB"
	chartRoute = "/coingecko/coins/solana/contract/" + aaplx + "/market_chart"
)

type sent struct {
	at     time.Time
	path   string
	query  string
	apiKey string
}

type upstream struct {
	handler http.Handler

	mu   sync.Mutex
	sent []sent
}

func (u *upstream) RoundTrip(r *http.Request) (*http.Response, error) {
	u.mu.Lock()
	u.sent = append(u.sent, sent{
		at: clock.Real{}.Now(), path: r.URL.Path, query: r.URL.RawQuery, apiKey: r.Header.Get("x-cg-demo-api-key"),
	})
	u.mu.Unlock()
	rec := httptest.NewRecorder()
	u.handler.ServeHTTP(rec, r)
	if err := r.Context().Err(); err != nil {
		return nil, err
	}
	return rec.Result(), nil
}

func (u *upstream) requests() []sent {
	u.mu.Lock()
	defer u.mu.Unlock()
	return append([]sent(nil), u.sent...)
}

func testConfig(key string) config.Config {
	return config.Config{
		CoinGecko: config.CoinGecko{BaseURL: "http://coingecko.test/coingecko", APIKey: key},
		Timeouts:  config.Timeouts{CoinGecko: 15 * time.Second},
	}
}

func clientWith(rt http.RoundTripper, key string) *coingecko.Client {
	opts := append(coingecko.Options(testConfig(key)), httpclient.WithTransport(rt))
	return coingecko.New(httpclient.New("coingecko", opts...), key)
}

func clientOver(h http.Handler, key string) (*coingecko.Client, *upstream) {
	u := &upstream{handler: h}
	return clientWith(u, key), u
}

func overFakes(t *testing.T) (*coingecko.Client, *upstream, *fakes.Server) {
	t.Helper()
	srv := fakes.New()
	c, u := clientOver(srv, "demo-key")
	return c, u, srv
}

func replying(status int, body string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(status)
		_, _ = io.WriteString(w, body)
	})
}

func script(t *testing.T, srv *fakes.Server, step fakes.Step) {
	t.Helper()
	raw, err := json.Marshal(step)
	if err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/_script", bytes.NewReader(raw)))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("script %s = %d %q", raw, rec.Code, rec.Body.String())
	}
}

func mint(t *testing.T, raw string) domain.Mint {
	t.Helper()
	m, err := domain.ParseMint(raw)
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func micros(v ...uint64) []money.Micros {
	out := make([]money.Micros, len(v))
	for i, x := range v {
		out[i] = money.MicrosFromUint64(x)
	}
	return out
}

func wantSamples(t *testing.T, got []app.Sample, firstMs, stepMs int64, want []money.Micros) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("got %d samples %+v, want %d", len(got), got, len(want))
	}
	for i, s := range got {
		at := time.UnixMilli(firstMs + int64(i)*stepMs).UTC()
		if !s.At.Equal(at) || s.Price != want[i] {
			t.Fatalf("sample %d = %v %s, want %v %s", i, s.At, s.Price, at, want[i])
		}
	}
}

func wantVolume(t *testing.T, got *app.Volume, ms int64, want uint64) {
	t.Helper()
	if got == nil || !got.At.Equal(time.UnixMilli(ms).UTC()) || got.USD != money.MicrosFromUint64(want) {
		t.Fatalf("volume = %+v, want %d micros at %d", got, want, ms)
	}
}

func wantCode(t *testing.T, err error, code errs.Code) {
	t.Helper()
	if errs.CodeOf(err) != code {
		t.Fatalf("err = %v, want %s", err, code)
	}
}

func TestCoinGecko_readsEveryRecordedMintAtEveryDays(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		c, u, _ := overFakes(t)
		for _, tc := range []struct {
			raw   string
			days  int
			first int64
			step  int64
			want  []money.Micros
		}{
			{aaplx, 365, 1790553641000, 86400000, micros(270123456, 271500000, 268250000, 272999999)},
			{aaplx, 90, 1790553623000, 3600000, micros(272824691, 274215000, 270932500, 275729999)},
			{aaplx, 1, 1790553607000, 300000, micros(275525925, 276930000, 273615000, 278459999)},
			{tspacex, 365, 1790553641000, 86400000, micros(412500000, 415000001, 409750000, 418100000)},
			{anduril, 1, 1790553607000, 300000, micros(147551109, 153204000, 142698000, 164374274)},
		} {
			got, err := c.MarketChart(t.Context(), mint(t, tc.raw), tc.days)
			if err != nil {
				t.Fatalf("%s days=%d: %v", tc.raw, tc.days, err)
			}
			wantSamples(t, got.Prices, tc.first, tc.step, tc.want)
			if tc.days == 1 {
				wantVolume(t, got.Volume24h, 1790553607000, 1_500_000_500_000)
			}
		}
		last := u.requests()[len(u.requests())-1]
		if last.path != "/coingecko/coins/solana/contract/"+anduril+"/market_chart" ||
			last.query != "days=1&vs_currency=usd" || last.apiKey != "demo-key" {
			t.Fatalf("last request = %+v, want the market_chart path, days=1&vs_currency=usd and the demo key", last)
		}
	})
}

func TestCoinGecko_unlistedMintIsNotFound(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		c, _, srv := overFakes(t)
		script(t, srv, fakes.Step{Route: chartRoute, Action: fakes.ActionFail, Status: http.StatusNotFound})
		got, err := c.MarketChart(t.Context(), mint(t, aaplx), 1)
		wantCode(t, err, errs.CodeNotFound)
		if got.Prices != nil || got.Volume24h != nil {
			t.Fatalf("scripted 404 answered %v, want nothing", got)
		}
		_, err = c.MarketChart(t.Context(), mint(t, unlisted), 365)
		wantCode(t, err, errs.CodeNotFound)
	})
}

func TestCoinGecko_noKeyNeverCallsTheKeylessAPI(t *testing.T) {
	t.Parallel()
	c, u := clientOver(fakes.New(), "")
	if c.Configured() {
		t.Fatal("Configured with no key = true")
	}
	_, err := c.MarketChart(t.Context(), mint(t, aaplx), 1)
	wantCode(t, err, errs.CodeInvalidInput)
	if n := len(u.requests()); n != 0 {
		t.Fatalf("keyless client sent %d requests, want none", n)
	}
}

func TestCoinGecko_refusesAnEmptyWindow(t *testing.T) {
	t.Parallel()
	c, u, _ := overFakes(t)
	if !c.Configured() {
		t.Fatal("Configured with a key = false")
	}
	_, err := c.MarketChart(t.Context(), mint(t, aaplx), 0)
	wantCode(t, err, errs.CodeInvalidInput)
	if n := len(u.requests()); n != 0 {
		t.Fatalf("days=0 sent %d requests, want none", n)
	}
}

func TestCoinGecko_pricesRoundDownAndNeverUseFloats(t *testing.T) {
	t.Parallel()
	body := `{"prices":[[1790553600000,0.9999999999],[1790553900000,1.5e2],[1790554200000,1234567.1234569],` +
		`[1790554500000,0.0000004],[1790554800000,9007199254.740993]]}`
	c, _ := clientOver(replying(http.StatusOK, body), "k")
	got, err := c.MarketChart(t.Context(), mint(t, aaplx), 1)
	if err != nil {
		t.Fatal(err)
	}
	wantSamples(t, got.Prices[:3], 1790553600000, 300000, micros(999999, 150000000, 1234567123456))
	if len(got.Prices) != 4 || got.Prices[3].Price != money.MicrosFromUint64(9007199254740993) {
		t.Fatalf("samples = %+v, want the sub-micro price dropped and the 2^53+1 price exact", got)
	}
}

func TestCoinGecko_countsTheSubMicroPricesItDropsInTheLog(t *testing.T) {
	t.Parallel()
	var logs testkit.Logs
	ctx := observability.WithLogger(t.Context(), observability.NewLogger(config.Config{Env: config.EnvTest}, &logs))
	body := `{"prices":[[1790553600000,0.0000004],[1790553900000,2],[1790554200000,0.0000009]]}`
	c, _ := clientOver(replying(http.StatusOK, body), "k")
	if got, err := c.MarketChart(ctx, mint(t, aaplx), 90); err != nil || len(got.Prices) != 1 {
		t.Fatalf("MarketChart = %v, %v, want the one whole-micro price", got, err)
	}
	want := `"msg":"market.coingecko.sub_micro_dropped"`
	if line := string(logs.Bytes()); !strings.Contains(line, want) || !strings.Contains(line, `"dropped":2`) ||
		!strings.Contains(line, `"days":90`) || !strings.Contains(line, `"mint":"`+aaplx+`"`) {
		t.Fatalf("log = %s, want %s for 2 prices of the 90 day AAPLx chart", line, want)
	}
	logs = testkit.Logs{}
	clean, _ := clientOver(replying(http.StatusOK, `{"prices":[[1790553600000,2]]}`), "k")
	_, err := clean.MarketChart(ctx, mint(t, aaplx), 90)
	if err != nil || strings.Contains(string(logs.Bytes()), "dropped") {
		t.Fatalf("a clean answer logged %s with err %v, want nothing", logs.Bytes(), err)
	}
}

func TestCoinGecko_malformedAnswersAreDecodeFailures(t *testing.T) {
	t.Parallel()
	for name, body := range map[string]string{
		"not json":            `<html>`,
		"prices not an array": `{"prices": 7}`,
		"fractional ts":       `{"prices":[[1.5,2]]}`,
		"short pair":          `{"prices":[[1790553600000]]}`,
		"negative price":      `{"prices":[[1790553600000,-1]]}`,
		"price as text":       `{"prices":[[1790553600000,"abc"]]}`,
		"price over int64":    `{"prices":[[1790553600000,9223372036855]]}`,
	} {
		c, _ := clientOver(replying(http.StatusOK, body), "k")
		_, err := c.MarketChart(t.Context(), mint(t, aaplx), 1)
		if errs.CodeOf(err) != errs.CodeDecodeFailed {
			t.Errorf("%s: err = %v, want decode_failed", name, err)
		}
	}
}

func TestCoinGecko_otherStatusesAreUnavailable(t *testing.T) {
	t.Parallel()
	for _, status := range []int{http.StatusUnauthorized, http.StatusForbidden, http.StatusInternalServerError} {
		c, _ := clientOver(replying(status, `{"error":"no"}`), "k")
		_, err := c.MarketChart(t.Context(), mint(t, aaplx), 1)
		wantCode(t, err, errs.CodeUpstreamUnavailable)
	}
}

func TestCoinGecko_429BacksOff(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		c, u, srv := overFakes(t)
		script(t, srv, fakes.Step{
			Route: chartRoute, Query: map[string]string{"days": "1"}, Action: fakes.ActionSucceed,
			Fixture: "/coingecko/rate-limited", Times: 1,
		})
		got, err := c.MarketChart(t.Context(), mint(t, aaplx), 1)
		if err != nil {
			t.Fatal(err)
		}
		wantSamples(t, got.Prices, 1790553607000, 300000, micros(275525925, 276930000, 273615000, 278459999))
		reqs := u.requests()
		if len(reqs) != 2 || reqs[1].at.Sub(reqs[0].at) != 2*time.Second {
			t.Fatalf("requests = %+v, want two calls 2s apart, the Retry-After the 429 named", reqs)
		}
	})
}

func TestCoinGecko_aStubborn429IsRateLimitedAndRetryable(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		c, u, srv := overFakes(t)
		script(t, srv, fakes.Step{
			Route: chartRoute, Action: fakes.ActionSucceed, Fixture: "/coingecko/rate-limited", Times: 3,
		})
		_, err := c.MarketChart(t.Context(), mint(t, aaplx), 1)
		wantCode(t, err, errs.CodeCoinGeckoRateLimited)
		if !errs.Retryable(errs.CodeOf(err)) || len(u.requests()) != 3 {
			t.Fatalf("retryable = %v after %d calls, want retryable after 3",
				errs.Retryable(errs.CodeOf(err)), len(u.requests()))
		}
	})
}

func TestCoinGecko_aDownUpstreamIsUnavailableNotRateLimited(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		c, _ := clientOver(replying(http.StatusServiceUnavailable, ""), "k")
		_, err := c.MarketChart(t.Context(), mint(t, aaplx), 1)
		wantCode(t, err, errs.CodeUpstreamUnavailable)
		c = clientWith(failingTransport{}, "k")
		_, err = c.MarketChart(t.Context(), mint(t, aaplx), 1)
		wantCode(t, err, errs.CodeUpstreamUnavailable)
	})
}

type failingTransport struct{}

func (failingTransport) RoundTrip(*http.Request) (*http.Response, error) {
	return nil, io.ErrUnexpectedEOF
}

func TestCoinGecko_aRetryAfterBeyondTheDeadlineIsStillRateLimited(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		h := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Retry-After", "3600")
			w.WriteHeader(http.StatusTooManyRequests)
		})
		c, _ := clientOver(h, "k")
		_, err := c.MarketChart(t.Context(), mint(t, aaplx), 1)
		wantCode(t, err, errs.CodeCoinGeckoRateLimited)
	})
}

func TestCoinGecko_LimiterHolds(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		c, u, _ := overFakes(t)
		callers := make([]int, 45)
		call := func(ctx context.Context, i int) (struct{}, error) {
			raw := []string{aaplx, tspacex, anduril}[i%3]
			_, err := c.MarketChart(ctx, mint(t, raw), []int{1, 90, 365}[i%3])
			return struct{}{}, err
		}
		if _, err := concurrency.FanOut(t.Context(), len(callers), callers, call); err != nil {
			t.Fatal(err)
		}
		reqs := u.requests()
		if len(reqs) != 45 {
			t.Fatalf("sent %d requests, want 45", len(reqs))
		}
		if most := mostInAMinute(reqs); most > 25 {
			t.Fatalf("%d calls inside one minute, want at most 25", most)
		}
	})
}

func TestCoinGecko_aCallerThatCannotWaitForTheLimiterGivesUp(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		c, u, _ := overFakes(t)
		if _, err := c.MarketChart(t.Context(), mint(t, aaplx), 1); err != nil {
			t.Fatal(err)
		}
		ctx, cancel := context.WithTimeout(t.Context(), time.Second)
		defer cancel()
		_, err := c.MarketChart(ctx, mint(t, aaplx), 1)
		wantCode(t, err, errs.CodeUpstreamUnavailable)
		if n := len(u.requests()); n != 1 {
			t.Fatalf("requests = %d, want the second call never sent", n)
		}
	})
}

func mostInAMinute(reqs []sent) int {
	most := 0
	for i, r := range reqs {
		n := 0
		for _, o := range reqs[i:] {
			if o.at.Sub(r.at) < time.Minute-time.Millisecond {
				n++
			}
		}
		most = max(most, n)
	}
	return most
}

func TestCoinGecko_maxAsksForTheWholeHistory(t *testing.T) {
	t.Parallel()
	c, u := clientOver(replying(http.StatusOK, `{"prices":[[1790553641000,270.5]]}`), "demo-key")
	got, err := c.MarketChart(t.Context(), mint(t, aaplx), app.HistoryMaxDays)
	if err != nil || len(got.Prices) != 1 {
		t.Fatalf("max = %v err %v", got, err)
	}
	if reqs := u.requests(); len(reqs) != 1 || reqs[0].query != "days=max&vs_currency=usd" {
		t.Fatalf("requests = %+v", reqs)
	}
}

func TestCoinGecko_volumeIsTheNewestTotalVolumesPointFlooredToMicros(t *testing.T) {
	t.Parallel()
	body := `{"prices":[[1790553600000,2]],"total_volumes":[[1790553900000,5],` +
		`[1790554500000,1234567.1234569],[1790554200000,9]]}`
	c, _ := clientOver(replying(http.StatusOK, body), "k")
	got, err := c.MarketChart(t.Context(), mint(t, aaplx), 1)
	if err != nil {
		t.Fatal(err)
	}
	wantVolume(t, got.Volume24h, 1790554500000, 1234567123456)
}

func TestCoinGecko_noVolumeSeriesGivesNilVolumeAndKeepsThePrices(t *testing.T) {
	t.Parallel()
	for name, body := range map[string]string{
		"absent": `{"prices":[[1790553600000,2]]}`,
		"empty":  `{"prices":[[1790553600000,2]],"total_volumes":[]}`,
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			c, _ := clientOver(replying(http.StatusOK, body), "k")
			got, err := c.MarketChart(t.Context(), mint(t, aaplx), 1)
			if err != nil || got.Volume24h != nil || len(got.Prices) != 1 {
				t.Fatalf("MarketChart = %+v, %v, want one price and no volume", got, err)
			}
		})
	}
}

func TestCoinGecko_aBadNewestVolumeGivesNilVolumeAndNeverFailsThePrices(t *testing.T) {
	t.Parallel()
	for name, volume := range map[string]string{
		"zero":      `[1790554500000,0]`,
		"sub micro": `[1790554500000,0.0000004]`,
		"negative":  `[1790554500000,-5]`,
		"too large": `[1790554500000,9223372036855]`,
		"null":      `[1790554500000,null]`,
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			body := `{"prices":[[1790553600000,2]],"total_volumes":[[1790553900000,5],` + volume + `]}`
			c, _ := clientOver(replying(http.StatusOK, body), "k")
			got, err := c.MarketChart(t.Context(), mint(t, aaplx), 1)
			if err != nil || got.Volume24h != nil || len(got.Prices) != 1 {
				t.Fatalf("MarketChart = %+v, %v, want one price and no volume", got, err)
			}
		})
	}
}

func TestCoinGecko_aVolumePointWithoutAnIntegerTimestampIsSkipped(t *testing.T) {
	t.Parallel()
	body := `{"prices":[[1790553600000,2]],"total_volumes":[[1790553900000,5],[1.5,9],[1790554200000,7]]}`
	c, _ := clientOver(replying(http.StatusOK, body), "k")
	got, err := c.MarketChart(t.Context(), mint(t, aaplx), 1)
	if err != nil {
		t.Fatal(err)
	}
	wantVolume(t, got.Volume24h, 1790554200000, 7_000_000)
}

const volumeBody = `{
  "` + aaplx + `": {"usd": 275.5, "usd_24h_vol": 181232010.10689816},
  "` + tspacex + `": {"usd": 10, "usd_24h_vol": null},
  "` + anduril + `": {"usd": 3, "usd_24h_vol": 0}
}`

func TestCoinGecko_volumesAskForEveryMintInOneCallAndFloorToMicros(t *testing.T) {
	t.Parallel()
	c, u := clientOver(replying(http.StatusOK, volumeBody), "demo-key")
	asked := []domain.Mint{mint(t, aaplx), mint(t, tspacex), mint(t, anduril), mint(t, unlisted)}
	got, err := c.Volumes(t.Context(), asked)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[mint(t, aaplx)] != money.MicrosFromUint64(181232010106898) {
		t.Fatalf("volumes = %v, want only the aapl reading: null, zero and missing are left out", got)
	}
	reqs := u.requests()
	if len(reqs) != 1 || reqs[0].path != "/coingecko/simple/token_price/solana" || reqs[0].apiKey != "demo-key" {
		t.Fatalf("requests = %+v, want one keyed call to simple/token_price/solana", reqs)
	}
	q, err := url.ParseQuery(reqs[0].query)
	if err != nil {
		t.Fatal(err)
	}
	if q.Get("contract_addresses") != aaplx+","+tspacex+","+anduril+","+unlisted ||
		q.Get("include_24hr_vol") != "true" || q.Get("vs_currencies") != "usd" {
		t.Fatalf("query = %v, want all four addresses with volume on, in usd", q)
	}
}

func TestCoinGecko_volumesRefuseNoKeyAnEmptyListAndATooBigBatch(t *testing.T) {
	t.Parallel()
	keyed, u := clientOver(replying(http.StatusOK, "{}"), "demo-key")
	if _, err := keyed.Volumes(t.Context(), nil); errs.CodeOf(err) != errs.CodeInvalidInput {
		t.Fatalf("empty list err = %v, want invalid_input", err)
	}
	big := make([]domain.Mint, app.VolumeBatch+1)
	for i := range big {
		big[i] = mint(t, aaplx)
	}
	if _, err := keyed.Volumes(t.Context(), big); errs.CodeOf(err) != errs.CodeInvalidInput {
		t.Fatalf("oversized batch err = %v, want invalid_input", err)
	}
	keyless, ku := clientOver(replying(http.StatusOK, "{}"), "")
	_, err := keyless.Volumes(t.Context(), []domain.Mint{mint(t, aaplx)})
	if errs.CodeOf(err) != errs.CodeInvalidInput {
		t.Fatalf("keyless err = %v, want invalid_input", err)
	}
	if len(u.requests())+len(ku.requests()) != 0 {
		t.Fatal("a refused call reached the upstream")
	}
}

func TestCoinGecko_aVolumesAnswerThatIsNotJSONIsADecodeFailure(t *testing.T) {
	t.Parallel()
	c, _ := clientOver(replying(http.StatusOK, "<html>"), "demo-key")
	_, err := c.Volumes(t.Context(), []domain.Mint{mint(t, aaplx)})
	wantCode(t, err, errs.CodeDecodeFailed)
}

func TestCoinGecko_aVolumesCallThatIsRateLimitedIsRetryable(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		c, _ := clientOver(replying(http.StatusTooManyRequests, "{}"), "demo-key")
		_, err := c.Volumes(t.Context(), []domain.Mint{mint(t, aaplx)})
		wantCode(t, err, errs.CodeCoinGeckoRateLimited)
		if !errs.Retryable(errs.CodeOf(err)) {
			t.Fatal("rate limit is not retryable")
		}
	})
}

func TestCoinGecko_aDownVolumesCallIsUnavailable(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		c, _ := clientOver(replying(http.StatusBadGateway, "{}"), "demo-key")
		_, err := c.Volumes(t.Context(), []domain.Mint{mint(t, aaplx)})
		wantCode(t, err, errs.CodeUpstreamUnavailable)
	})
}

func TestCoinGecko_theProPlanSendsItsOwnKeyHeader(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		var header string
		rt := roundTripFunc(func(r *http.Request) (*http.Response, error) {
			header = r.Header.Get(config.CoinGeckoPro.KeyHeader())
			rec := httptest.NewRecorder()
			replying(http.StatusOK, `{"prices":[[1700000000000,1.5]],"total_volumes":[]}`).ServeHTTP(rec, r)
			return rec.Result(), nil
		})
		opts := append(coingecko.Options(testConfig("pro-key")), httpclient.WithTransport(rt))
		c := coingecko.New(httpclient.New("coingecko", opts...), "pro-key").WithPlan(config.CoinGeckoPro)
		if _, err := c.MarketChart(t.Context(), mint(t, aaplx), 1); err != nil || header != "pro-key" {
			t.Fatalf("MarketChart = %v with header %q, want pro-key in x-cg-pro-api-key", err, header)
		}
	})
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
