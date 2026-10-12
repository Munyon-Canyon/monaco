package uniswapquote_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"golang.org/x/sync/errgroup"
	"golang.org/x/time/rate"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/market/adapters/uniswapquote"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	mc "github.com/monaco/monaco/apps/backend/internal/platform/chain/multichain"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpclient"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

type hookless struct{}

func (hookless) RoundTrip(req *http.Request) (*http.Response, error) {
	body, _ := io.ReadAll(req.Body)
	if bytes.Contains(body, []byte(`"protocols":["V2","V3"]`)) {
		<-req.Context().Done()
		return nil, req.Context().Err()
	}
	req.Body = io.NopCloser(bytes.NewReader(body))
	return http.DefaultTransport.RoundTrip(req)
}

type spendOnSimulation struct{ spend context.CancelFunc }

type spentBody struct {
	io.ReadCloser
	spend context.CancelFunc
}

func (b spentBody) Close() error {
	defer b.spend()
	return b.ReadCloser.Close()
}

func (s spendOnSimulation) RoundTrip(req *http.Request) (*http.Response, error) {
	body, _ := io.ReadAll(req.Body)
	req.Body = io.NopCloser(bytes.NewReader(body))
	resp, err := http.DefaultTransport.RoundTrip(req)
	if err == nil && bytes.Contains(body, []byte(`"eth_call"`)) {
		resp.Body = spentBody{ReadCloser: resp.Body, spend: s.spend}
	}
	return resp, err
}

func TestWrapper_keepsTheFinishedRouteWhenTheOtherOutlivesTheBudget(t *testing.T) {
	t.Parallel()
	r := newWrapperRig(t, chain.NetworkBase)
	budget, spend := context.WithCancel(t.Context())
	defer spend()
	owner, _ := chain.ParseEVMAddress(liveOwner)
	delegate, _ := chain.ParseEVMAddress(uniswapquote.DefaultDelegate)
	rpc := httpclient.New("evm-rpc-test", httpclient.WithBaseURL(r.rpcURL), httpclient.WithTimeout(time.Second),
		httpclient.WithTransport(spendOnSimulation{spend: spend}))
	uni := httpclient.New("uniswap-wrapper-test",
		append(uniswapquote.Options(r.uniBase, 5*time.Second), httpclient.WithTransport(hookless{}))...)
	client := uniswapquote.New(uni, "test-key", uniswapquote.Stored(owner), r.clock).
		WithWrapper(uniswapquote.NewWrapper(map[chain.Network]*httpclient.Client{chain.NetworkBase: rpc}, delegate))

	q, err := client.Quote(budget, b20Listing(t, chain.NetworkBase), mc.SideBuy, money.MicrosFromUint64(liveInput))

	if err != nil || q.RouteID != "q-live" || q.Options.HooksOptions != "" || q.GrossOutput.Int64() != receivedLive {
		t.Fatalf("quote = %v %s %+v, %v, want the default route penalized to %d after the hookless one ran out",
			q.GrossOutput, q.RouteID, q.Options, err, receivedLive)
	}
}

type meeting struct {
	mu   sync.Mutex
	want int
	met  int
	all  chan struct{}
}

func (m *meeting) attend(ctx, test context.Context) bool {
	m.mu.Lock()
	m.met++
	if m.met == m.want {
		close(m.all)
	}
	m.mu.Unlock()
	select {
	case <-m.all:
		return true
	case <-ctx.Done():
		return false
	case <-test.Done():
		return false
	}
}

func TestWrapper_readsTheAccountCodeWhileBothRoutesQuote(t *testing.T) {
	t.Parallel()
	r := newWrapperRig(t, chain.NetworkBase)
	steps := &meeting{want: 3, all: make(chan struct{})}
	uni := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.URL.Path == "/v1/quote" && !steps.attend(req.Context(), t.Context()) {
			return
		}
		if req.URL.Path == "/v1/swap" {
			_, _ = io.WriteString(w, r.swap)
			return
		}
		_, _ = io.WriteString(w, r.quote)
	}))
	t.Cleanup(uni.Close)
	rpc := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		body, _ := io.ReadAll(req.Body)
		if !bytes.Contains(body, []byte(`"eth_getCode"`)) {
			_, _ = io.WriteString(w, `{"jsonrpc":"2.0","id":1,"result":"0x"}`)
			return
		}
		if steps.attend(req.Context(), t.Context()) {
			_, _ = io.WriteString(w, r.codeReply)
		}
	}))
	t.Cleanup(rpc.Close)
	owner, _ := chain.ParseEVMAddress(liveOwner)
	delegate, _ := chain.ParseEVMAddress(uniswapquote.DefaultDelegate)
	client := uniswapquote.New(httpclient.New("uniswap-meeting-test", uniswapquote.Options(uni.URL, time.Second)...),
		"test-key", uniswapquote.Stored(owner), r.clock).
		WithWrapper(uniswapquote.NewWrapper(map[chain.Network]*httpclient.Client{chain.NetworkBase: httpclient.New(
			"evm-rpc-meeting-test", httpclient.WithBaseURL(rpc.URL), httpclient.WithTimeout(time.Second))}, delegate))

	q, err := client.Quote(t.Context(), b20Listing(t, chain.NetworkBase), mc.SideBuy, money.MicrosFromUint64(liveInput))

	if err != nil || q.GrossOutput.Int64() != quotedUnits {
		t.Fatalf(
			"quote = %v, %v, want both routes and eth_getCode in flight at once, then the quote met",
			q.GrossOutput,
			err,
		)
	}
}

type slowHookless struct {
	clock *testkit.Clock
	took  time.Duration
}

func (s slowHookless) RoundTrip(req *http.Request) (*http.Response, error) {
	body, _ := io.ReadAll(req.Body)
	req.Body = io.NopCloser(bytes.NewReader(body))
	resp, err := http.DefaultTransport.RoundTrip(req)
	if bytes.Contains(body, []byte(`"protocols":["V2","V3"]`)) {
		s.clock.Advance(s.took)
	}
	return resp, err
}

func TestWrapper_logsHowLongEachStepTook(t *testing.T) {
	t.Parallel()
	r := newWrapperRig(t, chain.NetworkBase)
	r.rpcReply = `{"jsonrpc":"2.0","id":1,"error":{"code":3,"data":"0x08c379a0"}}`
	r.hookless = &route{
		quote: strings.Replace(r.quote, `"q-live"`, `"q-hookless"`, 1),
		swap:  strings.Replace(r.swap, "0x3593564c00", "0x3593564c01", 1),
		reply: `{"jsonrpc":"2.0","id":1,"result":"0x"}`,
	}
	owner, _ := chain.ParseEVMAddress(liveOwner)
	uni := httpclient.New("uniswap-steps-test", append(uniswapquote.Options(r.uniBase, 5*time.Second),
		httpclient.WithTransport(slowHookless{clock: r.clock, took: 2 * time.Second}))...)
	client := uniswapquote.New(uni, "test-key", uniswapquote.Stored(owner), r.clock).WithWrapper(r.wrapper)
	var logs bytes.Buffer
	ctx := observability.WithLogger(t.Context(),
		slog.New(slog.NewJSONHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug})))

	if _, err := client.Quote(ctx, b20Listing(t, chain.NetworkBase), mc.SideBuy,
		money.MicrosFromUint64(liveInput)); err != nil {
		t.Fatal(err)
	}

	var line map[string]any
	_ = json.Unmarshal(logs.Bytes(), &line)
	for _, key := range observability.MarketRouterUniswapSteps.Required {
		if _, ok := line[key]; !ok {
			t.Fatalf("log line %s has no %q, want every step's duration", logs.String(), key)
		}
	}
	if line["msg"] != "market.router.uniswap_steps" || line["network"] != "base" || line["route"] != "hookless" ||
		line["hookless_quote_ms"] != 2000.0 || line["total_ms"] != 2000.0 {
		t.Fatalf("log line = %s, want Base's hookless route quoted in 2000 ms of a 2000 ms quote", logs.String())
	}
}

func TestWrapper_buildsEachSwapUnderTheRateLimit(t *testing.T) {
	t.Parallel()
	r := newWrapperRig(t, chain.NetworkBase)
	quotes := &meeting{want: 2, all: make(chan struct{})}
	both := func(w http.ResponseWriter, req *http.Request) {
		if req.URL.Path == "/v1/quote" && !quotes.attend(req.Context(), t.Context()) {
			return
		}
		r.uniswap(w, req)
	}
	owner, _ := chain.ParseEVMAddress(liveOwner)
	uni := httpclient.New("uniswap-limit-test", append(uniswapquote.Options("http://uniswap.test", 5*time.Second),
		httpclient.WithTransport(inMemory{serve: both}))...)
	client := uniswapquote.New(uni, "test-key", uniswapquote.Stored(owner), r.clock).WithWrapper(r.wrapper).
		WithLimiter(rate.NewLimiter(rate.Every(time.Hour), 2))
	budget, cancel := context.WithTimeout(t.Context(), time.Minute)
	defer cancel()

	_, err := client.Quote(budget, b20Listing(t, chain.NetworkBase), mc.SideBuy, money.MicrosFromUint64(liveInput))

	if errs.CodeOf(err) != errs.CodeUpstreamUnavailable || len(r.quotes) != 2 || r.swaps != 0 {
		t.Fatalf("quote err = %v after %d quotes and %d swap builds, want both quotes and no swap past the limit",
			err, len(r.quotes), r.swaps)
	}
}

type inMemory struct {
	serve   http.HandlerFunc
	release <-chan struct{}
}

func (m inMemory) RoundTrip(req *http.Request) (*http.Response, error) {
	body, _ := io.ReadAll(req.Body)
	req.Body = io.NopCloser(bytes.NewReader(body))
	if m.release != nil && bytes.Contains(body, []byte(`"protocols":["V2","V3"]`)) {
		select {
		case <-m.release:
		case <-req.Context().Done():
			return nil, req.Context().Err()
		}
	}
	w := httptest.NewRecorder()
	m.serve(w, req)
	return w.Result(), nil
}

func TestWrapper_skipsTheSwapOfARouteThatCannotBeatTheSimulatedOne(t *testing.T) {
	t.Parallel()
	r := newWrapperRig(t, chain.NetworkBase)
	r.rpcReply = `{"jsonrpc":"2.0","id":1,"result":"0x"}`
	r.hookless = &route{
		quote: strings.Replace(r.quote, `"q-live"`, `"q-hookless"`, 1),
		swap:  strings.Replace(r.swap, "0x3593564c00", "0x3593564c01", 1),
		reply: `{"jsonrpc":"2.0","id":1,"result":"0x"}`,
	}
	owner, _ := chain.ParseEVMAddress(liveOwner)
	delegate, _ := chain.ParseEVMAddress(uniswapquote.DefaultDelegate)
	synctest.Test(t, func(t *testing.T) {
		release := make(chan struct{})
		uni := httpclient.New(
			"uniswap-outranked-test",
			append(uniswapquote.Options("http://uniswap.test", 5*time.Second),
				httpclient.WithTransport(inMemory{serve: r.uniswap, release: release}))...)
		rpc := httpclient.New("evm-rpc-outranked-test", httpclient.WithBaseURL("http://rpc.test"),
			httpclient.WithTimeout(time.Second), httpclient.WithTransport(inMemory{serve: r.rpc}))
		client := uniswapquote.New(uni, "test-key", uniswapquote.Stored(owner), r.clock).
			WithWrapper(uniswapquote.NewWrapper(map[chain.Network]*httpclient.Client{chain.NetworkBase: rpc}, delegate))
		var q mc.Quote
		var err error
		var g errgroup.Group
		g.Go(func() error {
			q, err = client.Quote(t.Context(), b20Listing(t, chain.NetworkBase), mc.SideBuy,
				money.MicrosFromUint64(liveInput))
			return nil
		})

		synctest.Wait()
		close(release)
		_ = g.Wait()

		if err != nil || q.RouteID != "q-live" || q.GrossOutput.Int64() != quotedUnits || r.swaps != 1 {
			t.Fatalf("quote = %v %s, %v after %d swap builds, want the simulated default route and no hookless swap",
				q.GrossOutput, q.RouteID, err, r.swaps)
		}
	})
}
