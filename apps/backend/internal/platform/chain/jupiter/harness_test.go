package jupiter_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/jupiter"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/config"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpclient"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/testkit/fakes"
)

func usdc() jupiter.Mint {
	return jupiter.Mint{Address: "EPjFWdd5AufqSSqeM2qN1xzybapC8G4wEGGkZwyTDt1v", Decimals: 6}
}

func aaplx() jupiter.Mint {
	return jupiter.Mint{Address: "XsbEhLAtcf6HdfpFZ5xEMdqW8nfAvcsP5bdudRLJzJp", Decimals: 8}
}

const treasury = jupiter.SolanaAddress("9xQeWvG816bUx9EPjHmaT23yvVM2ZWbrrpZb9PusVFin")

type sent struct {
	at     time.Time
	method string
	path   string
	query  url.Values
	apiKey string
	body   string
}

type upstream struct {
	handler   http.Handler
	transport error
	badBody   bool

	mu   sync.Mutex
	sent []sent
}

func (u *upstream) RoundTrip(r *http.Request) (*http.Response, error) {
	var body []byte
	if r.Body != nil {
		body, _ = io.ReadAll(r.Body)
		r.Body = io.NopCloser(bytes.NewReader(body))
	}
	u.mu.Lock()
	u.sent = append(u.sent, sent{
		at: now(), method: r.Method, path: r.URL.Path, query: r.URL.Query(),
		apiKey: r.Header.Get("x-api-key"), body: string(body),
	})
	u.mu.Unlock()
	if u.transport != nil {
		return nil, u.transport
	}
	rec := httptest.NewRecorder()
	u.handler.ServeHTTP(rec, r)
	if err := r.Context().Err(); err != nil {
		return nil, err
	}
	resp := rec.Result()
	if u.badBody {
		resp.Body = io.NopCloser(errReader{})
	}
	return resp, nil
}

func (u *upstream) requests() []sent {
	u.mu.Lock()
	defer u.mu.Unlock()
	return append([]sent(nil), u.sent...)
}

type errReader struct{}

func (errReader) Read([]byte) (int, error) {
	return 0, errs.New(errs.CodeUpstreamUnavailable, "test.read")
}

func testConfig() config.Config {
	return config.Config{
		Jupiter: config.Jupiter{
			SwapBaseURL:  "http://jupiter.test/jupiter/swap/v2",
			PriceBaseURL: "http://jupiter.test/jupiter/price/v3",
			APIKey:       "test-key",
		},
		Timeouts: config.Timeouts{JupiterQuote: 5 * time.Second, JupiterExecute: 2 * time.Minute},
	}
}

func client(u *upstream) *jupiter.Client { return clientWith(testConfig(), u) }

func clientWith(cfg config.Config, u *upstream) *jupiter.Client {
	return jupiter.New(cfg, clock.Real{}, httpclient.WithTransport(u))
}

type recordingClock struct {
	clock.Real
	mu     sync.Mutex
	afters []time.Duration
}

func (c *recordingClock) After(d time.Duration) <-chan time.Time {
	c.mu.Lock()
	c.afters = append(c.afters, d)
	c.mu.Unlock()
	return c.Real.After(d)
}

func overFakesWithClock(t *testing.T, clk clock.Clock) (*jupiter.Client, *upstream, *fakes.Server) {
	t.Helper()
	srv := fakes.New()
	u := &upstream{handler: srv}
	return jupiter.New(testConfig(), clk, httpclient.WithTransport(u)), u, srv
}

func overFakes(t *testing.T) (*jupiter.Client, *upstream, *fakes.Server) {
	t.Helper()
	return overFakesWithClock(t, clock.Real{})
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

func replying(status int, body string) *upstream {
	return &upstream{handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(status)
		_, _ = io.WriteString(w, body)
	})}
}

func units(v uint64, m jupiter.Mint) money.BaseUnits { return money.NewBaseUnits(v, m.Decimals) }

func wantCode(t *testing.T, err error, code errs.Code) {
	t.Helper()
	if errs.CodeOf(err) != code {
		t.Fatalf("err = %v, want %s", err, code)
	}
}

func attr(err error, key string) string {
	for _, a := range errs.Detail(err) {
		if a.Key == key {
			return a.Value.String()
		}
	}
	return ""
}

func now() time.Time { return clock.Real{}.Now() }
