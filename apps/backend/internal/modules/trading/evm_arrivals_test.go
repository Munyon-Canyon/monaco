package trading_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/trading/adapters"
	"github.com/monaco/monaco/apps/backend/internal/modules/trading/app"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/config"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpclient"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

const (
	rhTreasury = "0x3318a72025dafedf50c63fe0d087c0e2653310e5"
	rhUSDG     = "0x5fc5360d0400a0fd4f2af552add042d716f1d168"
	rhBridge   = "0xf70da97812cb96acdf810712aa562db8dfa3dbef"
	transfer   = "0xddf252ad1be2c89b69c2b068fc378daa952ba7f163c4a11628f55a4df523b3ef"
)

const chainHead = 10_000

const alchemyFreeCap = "Under the Free tier plan, you can make eth_getLogs requests with up to a 10 block range. " +
	"Based on your parameters, this block range should work: [0x3219ddd, 0x3219de6]. Upgrade to PAYG for expanded block range."

func chainStart() time.Time { return time.Date(2026, 10, 12, 4, 0, 0, 0, time.UTC) }

type chainLog struct {
	block  uint64
	token  string
	from   string
	to     string
	amount uint64
	tx     string
}

type fakeChain struct {
	mu       sync.Mutex
	head     uint64
	logs     []chainLog
	maxSpan  uint64
	failLogs bool
	calls    map[string]int
	froms    []uint64
	budget   int
	clock    steppingClock
}

func newFakeChain() *fakeChain {
	return &fakeChain{
		head: chainHead, calls: map[string]int{}, budget: -1, clock: steppingClock{testkit.NewClock(chainStart())},
	}
}

func (c *fakeChain) allowLogs(n int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.budget = n
}

func (c *fakeChain) blockAt(n int64) time.Time {
	return chainStart().Add(time.Duration(n) * time.Second)
}

func topic(addr string) string {
	return "0x" + strings.Repeat("0", 24) + strings.TrimPrefix(addr, "0x")
}

func (c *fakeChain) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Method string            `json:"method"`
		Params []json.RawMessage `json:"params"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)
	c.mu.Lock()
	defer c.mu.Unlock()
	c.calls[req.Method]++
	if req.Method == "eth_getLogs" && c.budget == 0 {
		w.WriteHeader(http.StatusTooManyRequests)
		return
	}
	if req.Method == "eth_getLogs" && c.budget > 0 {
		c.budget--
	}
	result, rpcErr := c.answer(req.Method, req.Params)
	reply := map[string]any{"jsonrpc": "2.0", "id": 1, "result": result}
	if rpcErr == alchemyFreeCap {
		w.WriteHeader(http.StatusBadRequest)
	}
	if rpcErr != "" {
		reply = map[string]any{"jsonrpc": "2.0", "id": 1, "error": map[string]any{"code": -32600, "message": rpcErr}}
	}
	_ = json.NewEncoder(w).Encode(reply)
}

func quantity(raw json.RawMessage) uint64 {
	var s string
	_ = json.Unmarshal(raw, &s)
	n, _ := strconv.ParseUint(strings.TrimPrefix(s, "0x"), 16, 64)
	return n
}

func (c *fakeChain) answer(method string, params []json.RawMessage) (any, string) {
	switch method {
	case "eth_blockNumber":
		return fmt.Sprintf("0x%x", c.head), ""
	case "eth_getBlockByNumber":
		n := quantity(params[0])
		if n > c.head {
			return nil, ""
		}
		return map[string]string{"timestamp": fmt.Sprintf("0x%x", chainStart().Unix()+int64(n%(1<<40)))}, ""
	case "eth_getLogs":
		return c.getLogs(params[0])
	}
	return nil, "method not found"
}

func (c *fakeChain) getLogs(param json.RawMessage) (any, string) {
	var f struct {
		FromBlock json.RawMessage `json:"fromBlock"`
		ToBlock   json.RawMessage `json:"toBlock"`
		Address   string          `json:"address"`
		Topics    []*string       `json:"topics"`
	}
	_ = json.Unmarshal(param, &f)
	from, to := quantity(f.FromBlock), quantity(f.ToBlock)
	c.froms = append(c.froms, from)
	if c.failLogs {
		return nil, "internal error"
	}
	if c.maxSpan > 0 && to-from+1 > c.maxSpan {
		return nil, alchemyFreeCap
	}
	out := []map[string]any{}
	for _, l := range c.logs {
		if l.block < from || l.block > to || l.token != f.Address || *f.Topics[0] != transfer ||
			*f.Topics[2] != topic(l.to) {
			continue
		}
		out = append(out, map[string]any{
			"address": l.token, "topics": []string{transfer, topic(l.from), topic(l.to)},
			"data": fmt.Sprintf("0x%064x", l.amount), "transactionHash": l.tx, "removed": false,
		})
	}
	return out, ""
}

func (c *fakeChain) takeCalls() map[string]int {
	c.mu.Lock()
	defer c.mu.Unlock()
	calls := c.calls
	c.calls, c.froms = map[string]int{}, nil
	return calls
}

func (c *fakeChain) firstFrom() uint64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.froms[0]
}

func (c *fakeChain) arrivals(t *testing.T, chunk int64) *adapters.EVMArrivals {
	t.Helper()
	srv := httptest.NewServer(c)
	t.Cleanup(srv.Close)
	rpc := httpclient.New(
		"evm-rpc-robinhood-test",
		httpclient.WithBaseURL(srv.URL),
		httpclient.WithTimeout(time.Second),
	)
	return adapters.NewEVMArrivals(map[chain.Network]*httpclient.Client{chain.NetworkRobinhood: rpc}, chunk, c.clock)
}

func (c *fakeChain) inbound(
	t *testing.T, a *adapters.EVMArrivals, since time.Time, exclude ...string,
) (app.EVMArrival, bool, error) {
	t.Helper()
	return c.inboundCtx(t.Context(), t, a, since, exclude...)
}

func (c *fakeChain) inboundCtx(
	ctx context.Context, t *testing.T, a *adapters.EVMArrivals, since time.Time, exclude ...string,
) (app.EVMArrival, bool, error) {
	t.Helper()
	return a.Inbound(ctx, chain.NetworkRobinhood, evmAddress(t, rhUSDG), evmAddress(t, rhTreasury), since,
		money.MicrosFromUint64(arrivalFloor), money.MicrosFromUint64(1_000_000), exclude)
}

const arrivalFloor = 900_000

func txHash(n int) string { return fmt.Sprintf("0x%064x", n) }

func TestEVMArrivals_findsTheEarliestTransferInSinceTheStep(t *testing.T) {
	t.Parallel()
	c := newFakeChain()
	c.logs = []chainLog{
		{block: 500, token: rhUSDG, from: rhBridge, to: rhTreasury, amount: 400_000, tx: txHash(1)},
		{
			block: 4_200, token: rhUSDG, from: rhBridge, to: "0x00000000000000000000000000000000000000aa",
			amount: 972_507, tx: txHash(2),
		},
		{
			block: 4_300, token: "0x00000000000000000000000000000000000000bb", from: rhBridge, to: rhTreasury,
			amount: 972_507, tx: txHash(3),
		},
		{block: 4_400, token: rhUSDG, from: rhBridge, to: rhTreasury, amount: 1_000_001, tx: txHash(4)},
		{block: 4_500, token: rhUSDG, from: rhBridge, to: rhTreasury, amount: 972_507, tx: txHash(5)},
		{block: 4_600, token: rhUSDG, from: rhBridge, to: rhTreasury, amount: 990_000, tx: txHash(6)},
	}

	got, found, err := c.inbound(t, c.arrivals(t, 1_000), c.blockAt(4_000))

	want := app.EVMArrival{Amount: money.MicrosFromUint64(972_507), TxHash: txHash(5)}
	if err != nil || !found || got != want {
		t.Fatalf("Inbound = %+v, %v, %v, want %+v", got, found, err, want)
	}
}

func TestEVMArrivals_ignoresATransferBelowTheBridgesFeeFloor(t *testing.T) {
	t.Parallel()
	const stranger = "0x00000000000000000000000000000000000000cc"
	c := newFakeChain()
	c.logs = []chainLog{
		{block: 4_100, token: rhUSDG, from: stranger, to: rhTreasury, amount: 1, tx: txHash(1)},
		{block: 4_200, token: rhUSDG, from: rhBridge, to: rhTreasury, amount: arrivalFloor - 1, tx: txHash(2)},
		{block: 4_300, token: rhUSDG, from: rhBridge, to: rhTreasury, amount: 972_507, tx: txHash(3)},
	}

	got, found, err := c.inbound(t, c.arrivals(t, 50_000), c.blockAt(4_000))

	want := app.EVMArrival{Amount: money.MicrosFromUint64(972_507), TxHash: txHash(3)}
	if err != nil || !found || got != want {
		t.Fatalf("Inbound = %+v, %v, %v, want only the transfer at or above the floor %+v", got, found, err, want)
	}
}

func TestEVMArrivals_doesNotMatchSaleProceedsAsTheBridgeArrival(t *testing.T) {
	t.Parallel()
	const router = "0x00000000000000000000000000000000000000dd"
	c := newFakeChain()
	c.logs = []chainLog{{block: 4_100, token: rhUSDG, from: router, to: rhTreasury, amount: 310_000, tx: txHash(1)}}

	got, found, err := c.inbound(t, c.arrivals(t, 50_000), c.blockAt(4_000))

	if err != nil || found {
		t.Fatalf("Inbound = %+v, %v, %v, want sale proceeds under the floor left unmatched", got, found, err)
	}
}

func TestEVMArrivals_acceptsATransferExactlyAtTheFloor(t *testing.T) {
	t.Parallel()
	c := newFakeChain()
	c.logs = []chainLog{
		{block: 4_100, token: rhUSDG, from: rhBridge, to: rhTreasury, amount: arrivalFloor, tx: txHash(1)},
	}

	got, found, err := c.inbound(t, c.arrivals(t, 50_000), c.blockAt(4_000))

	if err != nil || !found || got.Amount != money.MicrosFromUint64(arrivalFloor) {
		t.Fatalf("Inbound = %+v, %v, %v, want the transfer at the floor", got, found, err)
	}
}

func TestEVMArrivals_looksTwoMinutesBeforeTheStep(t *testing.T) {
	t.Parallel()
	c := newFakeChain()
	c.logs = []chainLog{
		{block: 3_879, token: rhUSDG, from: rhBridge, to: rhTreasury, amount: 950_000, tx: txHash(1)},
		{block: 3_880, token: rhUSDG, from: rhBridge, to: rhTreasury, amount: 960_000, tx: txHash(2)},
	}

	got, found, err := c.inbound(t, c.arrivals(t, 50_000), c.blockAt(4_000))

	if err != nil || !found || got.TxHash != txHash(2) {
		t.Fatalf("Inbound = %+v, %v, %v, want the transfer at since - 2m", got, found, err)
	}
}

func TestEVMArrivals_skipsAnExcludedTransfer(t *testing.T) {
	t.Parallel()
	c := newFakeChain()
	c.logs = []chainLog{
		{block: 4_100, token: rhUSDG, from: rhBridge, to: rhTreasury, amount: 972_507, tx: txHash(1)},
		{block: 4_200, token: rhUSDG, from: rhBridge, to: rhTreasury, amount: 980_000, tx: txHash(2)},
	}

	got, found, err := c.inbound(t, c.arrivals(t, 50_000), c.blockAt(4_000), txHash(1))

	if err != nil || !found || got.TxHash != txHash(2) {
		t.Fatalf("Inbound = %+v, %v, %v, want the second transfer", got, found, err)
	}
}

func TestEVMArrivals_reportsNothingFoundYet(t *testing.T) {
	t.Parallel()
	c := newFakeChain()
	c.logs = []chainLog{{block: 100, token: rhUSDG, from: rhBridge, to: rhTreasury, amount: 972_507, tx: txHash(1)}}

	for name, since := range map[string]time.Time{
		"after the last transfer": c.blockAt(4_000),
		"after the head":          c.blockAt(20_000),
	} {
		got, found, err := c.inbound(t, c.arrivals(t, 50_000), since)
		if err != nil || found {
			t.Fatalf("%s: Inbound = %+v, %v, %v, want not found", name, got, found, err)
		}
	}
}

func TestEVMArrivals_findsALaterArrivalUnderATenBlockCapWithoutRescanning(t *testing.T) {
	t.Parallel()
	c := newFakeChain()
	c.maxSpan = 10
	c.logs = []chainLog{{block: 4_300, token: rhUSDG, from: rhBridge, to: rhTreasury, amount: 972_507, tx: txHash(1)}}
	a := c.arrivals(t, 10)

	_, found, err := c.inbound(t, a, c.blockAt(4_000))
	first := c.takeCalls()
	if err != nil || found || first["eth_getLogs"] != 30 {
		t.Fatalf("tick 1 = %v, %v with %v calls, want nothing yet after 30 chunks", found, err, first)
	}
	start := c.clock.Now()
	got, found, err := c.inbound(t, a, c.blockAt(4_000))
	second := c.takeCalls()

	if err != nil || !found || got.TxHash != txHash(1) {
		t.Fatalf("tick 2 = %+v, %v, %v, want the transfer 300 blocks after the step", got, found, err)
	}
	if paced := c.clock.Now().Sub(start); paced != 13*250*time.Millisecond {
		t.Fatalf("tick 2 waited %s between its 14 calls, want 250ms before each after the first", paced)
	}
	if second["eth_getLogs"] != 13 || second["eth_getBlockByNumber"] != 0 || second["eth_blockNumber"] != 1 {
		t.Fatalf("tick 2 calls = %v, want 13 chunks from the cursor and no block search", second)
	}
}

func TestEVMArrivals_halvesARefusedChunkAndRetriesItOnce(t *testing.T) {
	t.Parallel()
	c := newFakeChain()
	c.maxSpan = 10
	c.logs = []chainLog{{block: 3_885, token: rhUSDG, from: rhBridge, to: rhTreasury, amount: 972_507, tx: txHash(1)}}
	a := c.arrivals(t, 40)
	logs := &testkit.Logs{}
	ctx := observability.WithLogger(t.Context(), observability.NewLogger(config.Config{Env: config.EnvTest}, logs))

	_, _, err := c.inboundCtx(ctx, t, a, c.blockAt(4_000))
	first := c.takeCalls()
	if errs.CodeOf(err) != errs.CodeRPCUnavailable || !strings.HasSuffix(err.Error(), alchemyFreeCap) ||
		first["eth_getLogs"] != 2 {
		t.Fatalf("tick 1 = %v with %v calls, want the provider's refusal after one halved retry", err, first)
	}
	got, found, err := c.inboundCtx(ctx, t, a, c.blockAt(4_000))
	second := c.takeCalls()

	if err != nil || !found || got.TxHash != txHash(1) || second["eth_getLogs"] != 2 {
		t.Fatalf("tick 2 = %+v, %v, %v with %v calls, want the transfer after halving to 10", got, found, err, second)
	}
	want := []string{"3880 3919 20 " + alchemyFreeCap, "3880 3899 10 " + alchemyFreeCap}
	if lines := logs.Lines(observability.TradingEVMArrivalsChunk.Name, "from", "to", "chunk",
		"rpc_message"); !slices.Equal(lines, want) {
		t.Fatalf("chunk logs = %q, want %q", lines, want)
	}
}

func TestEVMArrivals_anRPCOutageIsRetryable(t *testing.T) {
	t.Parallel()
	c := newFakeChain()
	c.failLogs = true

	_, _, err := c.inbound(t, c.arrivals(t, 50_000), c.blockAt(4_000))

	if errs.CodeOf(err) != errs.CodeRPCUnavailable || !errs.Retryable(errs.CodeOf(err)) {
		t.Fatalf("Inbound err = %v, want a retryable rpc_unavailable", err)
	}
}

func TestEVMArrivals_anUnknownNetworkIsInvalidInput(t *testing.T) {
	t.Parallel()
	a := adapters.NewEVMArrivals(nil, 10, steppingClock{testkit.NewClock(chainStart())})

	_, _, err := a.Inbound(t.Context(), chain.NetworkBase, evmAddress(t, rhUSDG), evmAddress(t, rhTreasury),
		chainStart(), money.Micros{}, money.MicrosFromUint64(1), nil)

	if errs.CodeOf(err) != errs.CodeInvalidInput {
		t.Fatalf("Inbound err = %v, want invalid_input", err)
	}
}

func rawRPC(t *testing.T, reply func(method string) (int, string)) *adapters.EVMArrivals {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Method string `json:"method"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		status, body := reply(req.Method)
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	rpc := httpclient.New(
		"evm-rpc-robinhood-test",
		httpclient.WithBaseURL(srv.URL),
		httpclient.WithTimeout(time.Second),
	)
	return adapters.NewEVMArrivals(map[chain.Network]*httpclient.Client{chain.NetworkRobinhood: rpc}, 10,
		steppingClock{testkit.NewClock(chainStart())})
}

func TestEVMArrivals_refusesWhatItCannotRead(t *testing.T) {
	t.Parallel()
	ok := func(result string) (int, string) {
		return http.StatusOK, `{"jsonrpc":"2.0","id":1,"result":` + result + `}`
	}
	block := func(method string) (int, string) {
		if method == "eth_blockNumber" {
			return ok(`"0x1000"`)
		}
		return ok(`{"timestamp":"0x7fffffff"}`)
	}
	for name, tc := range map[string]struct {
		reply func(string) (int, string)
		want  errs.Code
	}{
		"outage":       {func(string) (int, string) { return http.StatusServiceUnavailable, `{}` }, errs.CodeRPCUnavailable},
		"refused":      {func(string) (int, string) { return http.StatusNotFound, `{}` }, errs.CodeRPCUnavailable},
		"not json":     {func(string) (int, string) { return http.StatusOK, `<html>` }, errs.CodeDecodeFailed},
		"not a number": {func(string) (int, string) { return ok(`7`) }, errs.CodeDecodeFailed},
		"not hex":      {func(string) (int, string) { return ok(`"0xzz"`) }, errs.CodeDecodeFailed},
		"block refused": {func(method string) (int, string) {
			if method == "eth_blockNumber" {
				return ok(`"0x10"`)
			}
			return http.StatusOK, `{"jsonrpc":"2.0","id":1,"error":{"code":-32000,"message":"no"}}`
		}, errs.CodeRPCUnavailable},
		"short transfer data": {func(method string) (int, string) {
			if method != "eth_getLogs" {
				return block(method)
			}
			return ok(`[{"address":"` + rhUSDG + `","topics":[],"data":"0x01","transactionHash":"` + txHash(1) + `"}]`)
		}, errs.CodeDecodeFailed},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			_, _, err := rawRPC(t, tc.reply).Inbound(t.Context(), chain.NetworkRobinhood, evmAddress(t, rhUSDG),
				evmAddress(t, rhTreasury), chainStart(), money.Micros{}, money.MicrosFromUint64(1_000_000), nil)
			if errs.CodeOf(err) != tc.want {
				t.Fatalf("Inbound err = %v, want %s", err, tc.want)
			}
		})
	}
}

func TestEVMArrivals_aClosedRPCIsUnavailable(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.NotFoundHandler())
	srv.Close()
	rpc := httpclient.New(
		"evm-rpc-robinhood-test",
		httpclient.WithBaseURL(srv.URL),
		httpclient.WithTimeout(time.Second),
	)
	a := adapters.NewEVMArrivals(map[chain.Network]*httpclient.Client{chain.NetworkRobinhood: rpc}, 10,
		steppingClock{testkit.NewClock(chainStart())})

	_, _, err := a.Inbound(t.Context(), chain.NetworkRobinhood, evmAddress(t, rhUSDG), evmAddress(t, rhTreasury),
		chainStart(), money.Micros{}, money.MicrosFromUint64(1), nil)

	if !errs.Retryable(errs.CodeOf(err)) {
		t.Fatalf("Inbound err = %v, want a retryable outage", err)
	}
}

func TestEVMArrivals_resumesFromItsCursorAfterARateLimitAndBacksOff(t *testing.T) {
	t.Parallel()
	c := newFakeChain()
	c.maxSpan = 10
	c.logs = []chainLog{{block: 4_300, token: rhUSDG, from: rhBridge, to: rhTreasury, amount: 972_507, tx: txHash(1)}}
	a := c.arrivals(t, 10)
	backoffs := []time.Duration{15 * time.Second, 30 * time.Second, time.Minute, time.Minute}
	firsts := make([]uint64, 0, len(backoffs)+1)

	for tick, backoff := range backoffs {
		c.allowLogs(10)
		_, found, err := c.inbound(t, a, c.blockAt(4_000))
		firsts = append(firsts, c.firstFrom())
		if calls := c.takeCalls(); found || !errs.Retryable(errs.CodeOf(err)) || calls["eth_getLogs"] != 11 {
			t.Fatalf("tick %d = %v, %v with %v calls, want a retryable 429 after 10 chunks", tick+1, found, err, calls)
		}
		c.clock.Advance(backoff - time.Second)
		_, _, err = c.inbound(t, a, c.blockAt(4_000))
		if calls := c.takeCalls(); errs.CodeOf(err) != errs.CodeRPCUnavailable || len(calls) != 0 {
			t.Fatalf("tick %d retry inside %s = %v with %v calls, want no RPC call", tick+1, backoff, err, calls)
		}
		c.clock.Advance(time.Second)
	}
	c.allowLogs(10)
	got, found, err := c.inbound(t, a, c.blockAt(4_000))
	firsts = append(firsts, c.firstFrom())
	last := c.takeCalls()

	if err != nil || !found || got.TxHash != txHash(1) || last["eth_getLogs"] != 3 {
		t.Fatalf("tick 5 = %+v, %v, %v with %v calls, want the transfer in the third chunk", got, found, err, last)
	}
	if want := []uint64{3_880, 3_980, 4_080, 4_180, 4_280}; !slices.Equal(firsts, want) {
		t.Fatalf("first block per tick = %v, want %v", firsts, want)
	}
}

func TestEVMArrivals_waitsForTheChainsSafeDepth(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct {
		network chain.Network
		depth   uint64
	}{
		"robinhood": {chain.NetworkRobinhood, 240},
		"base":      {chain.NetworkBase, 30},
		"unlisted":  {chain.NetworkSolana, 32},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			c := newFakeChain()
			token := rhUSDG
			if tc.network == chain.NetworkBase {
				token = chain.NetworkBase.Cash().Address
			}
			c.logs = []chainLog{
				{
					block:  c.head - tc.depth + 1,
					token:  token,
					from:   rhBridge,
					to:     rhTreasury,
					amount: 972_507,
					tx:     txHash(1),
				},
			}
			srv := httptest.NewServer(c)
			t.Cleanup(srv.Close)
			rpc := httpclient.New("evm-rpc-test", httpclient.WithBaseURL(srv.URL), httpclient.WithTimeout(time.Second))
			a := adapters.NewEVMArrivals(map[chain.Network]*httpclient.Client{tc.network: rpc}, 50_000, c.clock)
			arrive := func() (app.EVMArrival, bool, error) {
				return a.Inbound(t.Context(), tc.network, evmAddress(t, token), evmAddress(t, rhTreasury),
					c.blockAt(4_000), money.Micros{}, money.MicrosFromUint64(1_000_000), nil)
			}

			if _, found, err := arrive(); err != nil || found {
				t.Fatalf("Inbound inside the safe depth = %v, %v, want not found", found, err)
			}
			c.mu.Lock()
			c.head++
			c.mu.Unlock()
			if got, found, err := arrive(); err != nil || !found || got.TxHash != txHash(1) {
				t.Fatalf("Inbound at the safe depth = %+v, %v, %v, want the transfer", got, found, err)
			}
		})
	}
}
