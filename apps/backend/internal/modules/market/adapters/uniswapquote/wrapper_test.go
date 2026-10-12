package uniswapquote_test

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"io"
	"math/big"
	"net/http"
	"net/http/httptest"
	"os"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/market/adapters/uniswapquote"
	"github.com/monaco/monaco/apps/backend/internal/modules/market/app"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/evmsim"
	mc "github.com/monaco/monaco/apps/backend/internal/platform/chain/multichain"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpclient"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

const (
	liveOwner    = "0xf15262a7ee9d20fa690185d58e0eb2aad475bf32"
	liveRouter   = "0xd6145b2d3f379919e8cdeda7b97e37c4b2ca9c40"
	b20Token     = "0xb2000000000000000000002d0ba3164cc74f58b7"
	quotedUnits  = 1_427_581
	receivedLive = 1_314_373
	liveInput    = 4_896_325
)

type probedSlot struct {
	Slot string `json:"slot"`
}

type probe struct {
	Cash    string     `json:"cash"`
	Balance probedSlot `json:"balance"`
	Allow   probedSlot `json:"allowance_to_permit2"`
	Permit2 probedSlot `json:"permit2_allowance_to_router"`
}

type wrapperRig struct {
	mu        sync.Mutex
	quotes    []map[string]any
	swaps     int
	calls     []map[string]any
	rpcReply  string
	swap      string
	status    int
	rpcURL    string
	uniBase   string
	codeReply string
	quote     string
	hookless  *route
	clock     *testkit.Clock
	client    *uniswapquote.Client
	wrapper   *uniswapquote.Wrapper
}

type route struct {
	quote, swap, reply string
}

func (r *route) quoteID() string {
	var w struct {
		Quote struct {
			QuoteID string `json:"quoteId"`
		} `json:"quote"`
	}
	_ = json.Unmarshal([]byte(r.quote), &w)
	return w.Quote.QuoteID
}

func (r *route) calldata() string {
	var w struct {
		Swap struct {
			Data string `json:"data"`
		} `json:"swap"`
	}
	_ = json.Unmarshal([]byte(r.swap), &w)
	return strings.ToLower(strings.TrimPrefix(w.Swap.Data, "0x"))
}

func hooklessRequest(body map[string]any) bool {
	protocols, _ := body["protocols"].([]any)
	_, hooks := body["hooksOptions"]
	return !hooks && slices.Equal(protocols, []any{"V2", "V3"})
}

func (r *wrapperRig) serveUniswap(w http.ResponseWriter, path string, body map[string]any) {
	quote, _ := body["quote"].(map[string]any)
	hookless := r.hookless != nil &&
		(hooklessRequest(body) || quote != nil && quote["quoteId"] == r.hookless.quoteID())
	switch {
	case path == "/v1/swap" && hookless:
		r.swaps++
		_, _ = io.WriteString(w, r.hookless.swap)
	case path == "/v1/swap":
		r.swaps++
		w.WriteHeader(r.status)
		_, _ = io.WriteString(w, r.swap)
	case hookless:
		r.quotes = append(r.quotes, body)
		_, _ = io.WriteString(w, r.hookless.quote)
	default:
		r.quotes = append(r.quotes, body)
		_, _ = io.WriteString(w, r.quote)
	}
}

func (r *wrapperRig) serveCall(w http.ResponseWriter, body map[string]any) {
	params, _ := body["params"].([]any)
	call, _ := params[0].(map[string]any)
	data, _ := call["data"].(string)
	if r.hookless != nil && r.hookless.calldata() != "" && strings.Contains(data, r.hookless.calldata()) {
		_, _ = io.WriteString(w, r.hookless.reply)
		return
	}
	_, _ = io.WriteString(w, r.rpcReply)
}

func revertTooLittle(floor, received int64) string {
	w := func(v int64) string { return hex.EncodeToString(big.NewInt(v).FillBytes(make([]byte, 32))) }
	return `{"jsonrpc":"2.0","id":1,"error":{"code":3,"message":"execution reverted","data":"0x8b063d73` +
		w(floor) + w(received) + `"}}`
}

func (r *wrapperRig) uniswap(w http.ResponseWriter, req *http.Request) {
	var body map[string]any
	_ = json.NewDecoder(req.Body).Decode(&body)
	r.mu.Lock()
	defer r.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	r.serveUniswap(w, req.URL.Path, body)
}

func (r *wrapperRig) rpc(w http.ResponseWriter, req *http.Request) {
	var body map[string]any
	_ = json.NewDecoder(req.Body).Decode(&body)
	r.mu.Lock()
	defer r.mu.Unlock()
	if body["method"] == "eth_getCode" {
		_, _ = io.WriteString(w, r.codeReply)
		return
	}
	r.calls = append(r.calls, body)
	r.serveCall(w, body)
}

func newWrapperRig(t *testing.T, network chain.Network) *wrapperRig {
	t.Helper()
	r := &wrapperRig{
		rpcReply: revertTooLittle(
			quotedUnits,
			receivedLive,
		),
		clock:     testkit.NewClock(quotedAt()),
		status:    http.StatusOK,
		codeReply: `{"jsonrpc":"2.0","id":1,"result":"0xef0100D6CEDDE84BE40893D153BE9D467CD6AD37875B28"}`,
		quote: `{"requestId":"q1","routing":"CLASSIC","permitData":null,"permitTransaction":null,` +
			`"quote":{"quoteId":"q-live","input":{"amount":"4896325"},"output":{"amount":"1427581"},` +
			`"gasFeeUSD":"0.01","route":[[{"fee":"10000"}]]}}`,
		swap: `{"requestId":"s1","swap":{"to":"0xD6145b2d3F379919E8CdEdA7B97E37c4b2Ca9C40",` +
			`"data":"0x3593564c00","value":"0x00","chainId":8453}}`,
	}
	uni := httptest.NewServer(http.HandlerFunc(r.uniswap))
	t.Cleanup(uni.Close)
	r.uniBase = uni.URL
	rpc := httptest.NewServer(http.HandlerFunc(r.rpc))
	t.Cleanup(rpc.Close)
	r.rpcURL = rpc.URL
	owner, _ := chain.ParseEVMAddress(liveOwner)
	delegate, _ := chain.ParseEVMAddress(uniswapquote.DefaultDelegate)
	hc := httpclient.New("uniswap-wrapper-test", uniswapquote.Options(uni.URL, 5*time.Second)...)
	rpcs := map[chain.Network]*httpclient.Client{
		network: httpclient.New("evm-rpc-test", httpclient.WithBaseURL(rpc.URL), httpclient.WithTimeout(time.Second)),
	}
	r.wrapper = uniswapquote.NewWrapper(rpcs, delegate)
	r.client = uniswapquote.New(hc, "test-key", uniswapquote.Stored(owner), r.clock).WithWrapper(r.wrapper)
	return r
}

func b20Listing(t *testing.T, network chain.Network) mc.Listing {
	t.Helper()
	l := listing(t, network, b20Token, 8)
	l.SharesPerToken = big.NewRat(1, 1)
	return l
}

func (r *wrapperRig) uniURL(t *testing.T) string {
	t.Helper()
	return r.uniBase
}

func (r *wrapperRig) buy(t *testing.T, network chain.Network) (mc.Quote, error) {
	t.Helper()
	return r.client.Quote(t.Context(), b20Listing(t, network), mc.SideBuy, money.MicrosFromUint64(liveInput))
}

func TestWrapper_pricesTheLiveWrapperShortfallIntoTheQuote(t *testing.T) {
	t.Parallel()
	r := newWrapperRig(t, chain.NetworkBase)

	q, err := r.buy(t, chain.NetworkBase)

	if err != nil || q.GrossOutput.Int64() != receivedLive {
		t.Fatalf("quote = %+v, %v, want the simulated %d", q, err, receivedLive)
	}
	impact := uint64(0)
	for _, c := range q.Costs {
		if c.Kind == mc.CostPriceImpact {
			impact += c.Micros.Uint64()
		}
	}
	if impact != 388_281 {
		t.Fatalf("price impact = %d, want floor(4896325 * 113208 / 1427581) = 388281 micros", impact)
	}
	if q := r.quotes[0]; q["slippageTolerance"] != 0.0 || q["generatePermitAsTransaction"] != true {
		t.Fatalf("quote request = %+v, want zero slippage and the permit as a transaction", q)
	}
	r.wantSimulation(t)
}

func (r *wrapperRig) simulation(t *testing.T) (map[string]any, map[string]any) {
	t.Helper()
	params, _ := r.calls[0]["params"].([]any)
	call, _ := params[0].(map[string]any)
	overrides, _ := params[2].(map[string]any)
	return call, overrides
}

func probedOverrides(overrides map[string]any, live probe) bool {
	_, account := overrides[liveOwner]
	cash, _ := overrides[live.Cash].(map[string]any)
	permit2, _ := overrides["0x000000000022d473030f116ddee9f6b43ac78ba3"].(map[string]any)
	cashDiff, _ := cash["stateDiff"].(map[string]any)
	permitDiff, _ := permit2["stateDiff"].(map[string]any)
	return !account && len(overrides) == 2 && len(cashDiff) == 2 && len(permitDiff) == 1 &&
		cashDiff[live.Balance.Slot] != nil && cashDiff[live.Allow.Slot] != nil && permitDiff[live.Permit2.Slot] != nil
}

func readProbe(t *testing.T, network chain.Network) probe {
	t.Helper()
	raw, err := os.ReadFile("testdata/slot_probe_" + string(network) + ".json")
	if err != nil {
		t.Fatal(err)
	}
	var live probe
	if err := json.Unmarshal(raw, &live); err != nil {
		t.Fatal(err)
	}
	return live
}

func (r *wrapperRig) wantSimulation(t *testing.T) {
	t.Helper()
	live := readProbe(t, chain.NetworkBase)
	call, overrides := r.simulation(t)
	data, _ := call["data"].(string)
	if call["from"] != "0x0000000071727de22e5e9d8baf0edac6f37da032" || call["to"] != liveOwner ||
		!strings.HasPrefix(data, "0xe9ae5c53") || !strings.Contains(data, strings.TrimPrefix(liveRouter, "0x")+
		strings.Repeat("0", 64)+"3593564c00") {
		t.Fatalf("eth_call = %+v, want EntryPoint calling execute(router, 0, swap calldata) on the account", call)
	}
	if call["gas"] != "0x2dc6c0" || !probedOverrides(overrides, live) {
		t.Fatalf("eth_call %+v overrides = %+v, want 3M gas, the account's own code and only the probed slots",
			call, overrides)
	}
}

func TestWrapper_overridesEachNetworksProbedCashSlots(t *testing.T) {
	t.Parallel()
	for _, network := range []chain.Network{
		chain.NetworkBase, chain.NetworkArbitrum, chain.NetworkEthereum, chain.NetworkRobinhood,
	} {
		t.Run(string(network), func(t *testing.T) {
			t.Parallel()
			live := readProbe(t, network)
			r := newWrapperRig(t, network)

			q, err := r.buy(t, network)

			if err != nil || q.GrossOutput.Int64() != receivedLive || live.Cash != network.Cash().Address {
				t.Fatalf(
					"quote = %v, %v for cash %s, want the simulated %d",
					q.GrossOutput,
					err,
					live.Cash,
					receivedLive,
				)
			}
			if _, overrides := r.simulation(t); !probedOverrides(overrides, live) {
				t.Fatalf("overrides = %+v, want the slots probed live on %s", overrides, network)
			}
		})
	}
}

func TestWrapper_keepsTheQuoteWhenTheSimulatedSwapMeetsIt(t *testing.T) {
	t.Parallel()
	r := newWrapperRig(t, chain.NetworkBase)
	r.rpcReply = `{"jsonrpc":"2.0","id":1,"result":"0x"}`

	q, err := r.buy(t, chain.NetworkBase)

	if err != nil || q.GrossOutput.Int64() != quotedUnits || len(q.Costs) != 2 {
		t.Fatalf("quote = %+v, %v, want the quoted output and no wrapper cost", q, err)
	}
}

func TestWrapper_dropsTheVenueWhenTheSimulationCannotPriceIt(t *testing.T) {
	t.Parallel()
	for name, c := range map[string]struct {
		reply   string
		network chain.Network
		rpcOn   chain.Network
		want    errs.Code
	}{
		"another revert": {
			`{"jsonrpc":"2.0","id":1,"error":{"code":3,"data":"0x08c379a0"}}`,
			chain.NetworkBase, chain.NetworkBase, errs.CodeNoRoute,
		},
		"overrides unsupported": {
			`{"jsonrpc":"2.0","id":1,"error":{"code":-32602,"message":"invalid params"}}`,
			chain.NetworkBase, chain.NetworkBase, errs.CodeNoRoute,
		},
		"no rpc for the network": {"", chain.NetworkArbitrum, chain.NetworkBase, errs.CodeNoRoute},
		"unreadable reply":       {`{`, chain.NetworkBase, chain.NetworkBase, errs.CodeDecodeFailed},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			r := newWrapperRig(t, c.rpcOn)
			r.rpcReply = c.reply
			if _, err := r.buy(t, c.network); errs.CodeOf(err) != c.want {
				t.Fatalf("Quote err = %v, want %s", err, c.want)
			}
		})
	}
}

func TestWrapper_reusesASimulationForFifteenSeconds(t *testing.T) {
	t.Parallel()
	r := newWrapperRig(t, chain.NetworkBase)
	for range 2 {
		if _, err := r.buy(t, chain.NetworkBase); err != nil {
			t.Fatal(err)
		}
	}
	if len(r.quotes) != 2 || r.swaps != 2 || len(r.calls) != 2 {
		t.Fatalf("%d quotes, %d swaps, %d simulations, want one per route inside the window",
			len(r.quotes), r.swaps, len(r.calls))
	}
	r.clock.Advance(16 * time.Second)
	if _, err := r.buy(t, chain.NetworkBase); err != nil || len(r.calls) != 4 {
		t.Fatalf("err %v after %d simulations, want a fresh one past the window", err, len(r.calls))
	}
}

func TestWrapper_evictsExpiredSimulations(t *testing.T) {
	t.Parallel()
	r := newWrapperRig(t, chain.NetworkBase)
	for _, amount := range []uint64{liveInput, liveInput + 1} {
		if _, err := r.client.Quote(t.Context(), b20Listing(t, chain.NetworkBase), mc.SideBuy,
			money.MicrosFromUint64(amount)); err != nil {
			t.Fatal(err)
		}
	}
	r.clock.Advance(15 * time.Second)
	if _, err := r.client.Quote(t.Context(), b20Listing(t, chain.NetworkBase), mc.SideBuy,
		money.MicrosFromUint64(liveInput+2)); err != nil {
		t.Fatal(err)
	}
	if n := r.wrapper.Cached(); n != 1 {
		t.Fatalf("%d cached simulations, want only the live one after the others expired", n)
	}
}

func TestWrapper_sellsAreQuotedWithoutASimulation(t *testing.T) {
	t.Parallel()
	r := newWrapperRig(t, chain.NetworkBase)
	if _, err := r.client.Quote(t.Context(), b20Listing(t, chain.NetworkBase), mc.SideSell,
		money.MicrosFromUint64(liveInput)); err != nil || len(r.calls) != 0 {
		t.Fatalf("sell err %v after %d simulations, want none", err, len(r.calls))
	}
}

type oneStock struct{ listings []mc.Listing }

func (o oneStock) EnabledListings(context.Context, mc.StockID) ([]mc.Listing, error) {
	return o.listings, nil
}

type flatPrice struct{ micros uint64 }

func (f flatPrice) SharePrice(context.Context, mc.StockID) (money.Micros, error) {
	return money.MicrosFromUint64(f.micros), nil
}

type freeBridge struct{}

func (freeBridge) BridgeOut(_ context.Context, _ chain.Network, amount money.Micros) (money.Micros, error) {
	return amount, nil
}

type solanaQuote struct{ units int64 }

func (s solanaQuote) Quote(_ context.Context, l mc.Listing, side mc.Side, amount money.Micros) (mc.Quote, error) {
	at := quotedAt()
	return mc.Quote{
		ListingID: l.ID, Side: side, InputMicros: amount, GrossOutput: big.NewInt(s.units),
		QuotedAt: at, ExpiresAt: at.Add(time.Minute),
	}, nil
}

func TestWrapper_aPenalizedBaseRanksBelowSolanaWhenThePenaltyExceedsItsLead(t *testing.T) {
	t.Parallel()
	met := `{"jsonrpc":"2.0","id":1,"result":"0x"}`
	for name, c := range map[string]struct {
		reply    string
		price    uint64
		solUnits int64
		want     chain.Network
		baseNet  mc.ShareNanos
	}{
		"8% wrapper loss ranks below Solana, not dropped": {
			revertTooLittle(quotedUnits, receivedLive), 343_000_000, 1_400_000, chain.NetworkSolana, 13_116_885,
		},
		"met":                        {met, 343_000_000, 1_400_000, chain.NetworkBase, 14_246_653},
		"thin pool is still dropped": {met, 300_000_000, 1_632_000, chain.NetworkSolana, 0},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			r := newWrapperRig(t, chain.NetworkBase)
			r.rpcReply = c.reply
			base := b20Listing(t, chain.NetworkBase)
			sol := listing(t, chain.NetworkSolana, solMint, 8)
			sol.SharesPerToken, sol.Venue = big.NewRat(1, 1), mc.VenueJupiter
			_ = sol.ID.UnmarshalText([]byte("0199d2f4-5a10-7000-8000-000000000002"))
			router := app.NewRouter(
				oneStock{[]mc.Listing{base, sol}},
				app.Venues{mc.VenueUniswap: r.client, mc.VenueJupiter: solanaQuote{units: c.solUnits}},
				flatPrice{micros: c.price},
				freeBridge{},
				r.clock,
				app.RouterRules{
					MinEdgeBps:   10,
					MaxImpactBps: 300,
					QuoteTimeout: time.Second,
					MaxQuoteAge:  time.Minute,
				},
			)

			got, err := router.Best(t.Context(), mc.StockID{}, mc.SideBuy, money.MicrosFromUint64(liveInput))

			if err != nil || got.Ranked[0].Network != c.want {
				t.Fatalf("comparison = %+v, %v, want %s to win", got, err, c.want)
			}
			var baseNet mc.ShareNanos
			for _, q := range got.Ranked {
				if q.Network == chain.NetworkBase {
					baseNet = q.NetShares
				}
			}
			if baseNet != c.baseNet {
				t.Fatalf("Base net shares = %d, want %d from the received units at 8 decimals", baseNet, c.baseNet)
			}
		})
	}
}

func TestWrapper_dropsTheVenueWhenTheSwapCalldataIsUnusable(t *testing.T) {
	t.Parallel()
	for name, c := range map[string]struct {
		swap   string
		status int
		want   errs.Code
	}{
		"refused":       {`{"errorCode":"QuoteExpired"}`, http.StatusBadRequest, errs.CodeNoRoute},
		"no swap":       {`{"requestId":"s1"}`, http.StatusOK, errs.CodeDecodeFailed},
		"native value":  {`{"swap":{"to":"` + liveRouter + `","data":"0x01","value":"0x01"}}`, http.StatusOK, errs.CodeDecodeFailed},
		"bad router":    {`{"swap":{"to":"0x12","data":"0x01","value":"0x0"}}`, http.StatusOK, errs.CodeDecodeFailed},
		"bad calldata":  {`{"swap":{"to":"` + liveRouter + `","data":"0x1","value":"0x0"}}`, http.StatusOK, errs.CodeDecodeFailed},
		"upstream down": {`{}`, http.StatusBadGateway, errs.CodeUpstreamUnavailable},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			r := newWrapperRig(t, chain.NetworkBase)
			r.swap, r.status = c.swap, c.status
			if _, err := r.buy(t, chain.NetworkBase); errs.CodeOf(err) != c.want {
				t.Fatalf("Quote err = %v, want %s", err, c.want)
			}
		})
	}
}

func TestWrapper_anUnreachableRPCDropsTheVenue(t *testing.T) {
	t.Parallel()
	r := newWrapperRig(t, chain.NetworkBase)
	owner, _ := chain.ParseEVMAddress(liveOwner)
	delegate, _ := chain.ParseEVMAddress(uniswapquote.DefaultDelegate)
	dead := httptest.NewServer(http.NotFoundHandler())
	dead.Close()
	rpcs := map[chain.Network]*httpclient.Client{chain.NetworkBase: httpclient.New("evm-rpc-dead",
		httpclient.WithBaseURL(dead.URL), httpclient.WithTimeout(time.Second))}
	hc := httpclient.New("uniswap-wrapper-dead", uniswapquote.Options(r.uniURL(t), 5*time.Second)...)
	client := uniswapquote.New(hc, "test-key", uniswapquote.Stored(owner), r.clock).
		WithWrapper(uniswapquote.NewWrapper(rpcs, delegate))
	_, err := client.Quote(t.Context(), b20Listing(t, chain.NetworkBase), mc.SideBuy, money.MicrosFromUint64(liveInput))
	if errs.CodeOf(err) != errs.CodeRPCUnavailable {
		t.Fatalf("Quote err = %v, want rpc_unavailable", err)
	}
}

func TestWrapper_dropsTheVenueWhenTheAccountIsNotPrivyDelegated(t *testing.T) {
	t.Parallel()
	for name, reply := range map[string]string{
		"no code":         `{"jsonrpc":"2.0","id":1,"result":"0x"}`,
		"other delegate":  `{"jsonrpc":"2.0","id":1,"result":"0xef01000000000000000000000000000000000000000001"}`,
		"code unreadable": `{"jsonrpc":"2.0","id":1,"error":{"code":-32000,"message":"boom"}}`,
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			r := newWrapperRig(t, chain.NetworkBase)
			r.codeReply = reply
			if _, err := r.buy(t, chain.NetworkBase); errs.CodeOf(err) != errs.CodeNoRoute || len(r.calls) != 0 {
				t.Fatalf("Quote err = %v after %d simulations, want no_route and none", err, len(r.calls))
			}
		})
	}
}

type liveReplay struct {
	Block         string          `json:"block"`
	QuoteResponse json.RawMessage `json:"quote_response"`
	SwapResponse  json.RawMessage `json:"swap_response"`
	AccountCode   string          `json:"account_code"`
	Call          map[string]any  `json:"call"`
	Overrides     map[string]any  `json:"overrides"`
	CallResponse  json.RawMessage `json:"call_response"`
}

func readLive(t *testing.T) liveReplay {
	t.Helper()
	raw, err := os.ReadFile("testdata/googlx_base_live.json")
	if err != nil {
		t.Fatal(err)
	}
	var live liveReplay
	if err := json.Unmarshal(raw, &live); err != nil {
		t.Fatal(err)
	}
	return live
}

func TestWrapper_replaysTheLiveGOOGLxCaseAsPenalized(t *testing.T) {
	t.Parallel()
	live := readLive(t)
	r := newWrapperRig(t, chain.NetworkBase)
	r.quote, r.swap = string(live.QuoteResponse), string(live.SwapResponse)
	r.codeReply = `{"jsonrpc":"2.0","id":1,"result":"` + live.AccountCode + `"}`
	r.rpcReply = string(live.CallResponse)

	q, err := r.buy(t, chain.NetworkBase)

	if err != nil || q.GrossOutput.Int64() != 1_319_601 {
		t.Fatalf("quote = %v, %v, want the 1450095 quote penalized to the 1319601 the account receives",
			q.GrossOutput, err)
	}
	params, _ := r.calls[0]["params"].([]any)
	call, _ := params[0].(map[string]any)
	if call["data"] != live.Call["data"] || call["gas"] != live.Call["gas"] {
		t.Fatalf("eth_call = %+v, want the recorded execute call at 3M gas", call)
	}
}

func TestWrapper_liveProbeRevertsAtThePinnedBlock(t *testing.T) {
	t.Parallel()
	url, set := os.LookupEnv("EVM_RPC_URL_BASE")
	if probe, _ := os.LookupEnv("MONACO_LIVE_EVM_PROBE"); probe != "1" || !set {
		return
	}
	live := readLive(t)
	body, _ := json.Marshal(map[string]any{
		"jsonrpc": "2.0", "id": 1, "method": "eth_call", "params": []any{live.Call, live.Block, live.Overrides},
	})
	client := httpclient.New("evm-live-probe", httpclient.WithBaseURL(url), httpclient.WithTimeout(20*time.Second))
	defer client.CloseIdleConnections()
	req, _ := http.NewRequestWithContext(t.Context(), http.MethodPost, "", strings.NewReader(string(body)))
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(t.Context(), req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	var got struct {
		Error struct {
			Data string `json:"data"`
		} `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	received := new(big.Int)
	if len(got.Error.Data) == 138 {
		received.SetString(got.Error.Data[74:], 16)
	}
	if !strings.HasPrefix(got.Error.Data, "0x8b063d73") || received.Int64() != 1_319_601 {
		t.Fatalf("live eth_call data = %q, want V4TooLittleReceived with 1319601 received", got.Error.Data)
	}
}

type recorded struct {
	Exchanges []struct {
		Path     string          `json:"path"`
		Request  map[string]any  `json:"request"`
		Response json.RawMessage `json:"response"`
	} `json:"exchanges"`
}

func liveRoutes(t *testing.T) (hooked, hookless route) {
	t.Helper()
	raw, err := os.ReadFile("testdata/googlx_base_routes.json")
	if err != nil {
		t.Fatal(err)
	}
	var rec recorded
	if err := json.Unmarshal(raw, &rec); err != nil {
		t.Fatal(err)
	}
	current := &hooked
	for _, e := range rec.Exchanges {
		switch {
		case e.Path == "/v1/quote" && e.Request["hooksOptions"] == mc.NoHooks:
			current = &hookless
			current.quote = string(e.Response)
		case e.Path == "/v1/quote":
			current = &hooked
			current.quote = string(e.Response)
		case e.Path == "/v1/swap":
			current.swap = string(e.Response)
		case e.Request["method"] == "eth_call":
			current.reply = string(e.Response)
		}
	}
	return hooked, hookless
}

func TestWrapper_replaysTheLiveRoutesAndBuysThroughTheHooklessOne(t *testing.T) {
	t.Parallel()
	hooked, hookless := liveRoutes(t)
	r := newWrapperRig(t, chain.NetworkBase)
	r.quote, r.swap, r.rpcReply = hooked.quote, hooked.swap, hooked.reply
	r.hookless = &hookless

	q, err := r.buy(t, chain.NetworkBase)

	if err != nil || q.GrossOutput.Int64() != 1_388_248 || q.RouteID != hookless.quoteID() ||
		q.Options.HooksOptions != mc.NoHooks || !r.askedBothRoutes() {
		t.Fatalf("quote = %v %s %+v, %v, want the hookless v3 route's 1388248 over the hooked route's 1318734",
			q.GrossOutput, q.RouteID, q.Options, err)
	}
	if len(r.calls) != 2 {
		t.Fatalf("%d simulations, want one per route", len(r.calls))
	}
}

func (r *wrapperRig) askedBothRoutes() bool {
	var hooked, hookless int
	for _, q := range r.quotes {
		protocols, _ := q["protocols"].([]any)
		_, hooks := q["hooksOptions"]
		switch {
		case hooks:
		case slices.Equal(protocols, []any{"V2", "V3", "V4"}):
			hooked++
		case slices.Equal(protocols, []any{"V2", "V3"}):
			hookless++
		}
	}
	return len(r.quotes) == 2 && hooked == 1 && hookless == 1
}

func TestWrapper_keepsTheHookedRouteWhenItSimulatesHigher(t *testing.T) {
	t.Parallel()
	r := newWrapperRig(t, chain.NetworkBase)
	r.rpcReply = `{"jsonrpc":"2.0","id":1,"result":"0x"}`
	r.hookless = &route{
		quote: strings.Replace(r.quote, `"q-live"`, `"q-hookless"`, 1),
		swap:  strings.Replace(r.swap, "0x3593564c00", "0x3593564c01", 1),
		reply: revertTooLittle(quotedUnits, receivedLive),
	}

	q, err := r.buy(t, chain.NetworkBase)

	if err != nil || q.GrossOutput.Int64() != quotedUnits || q.RouteID != "q-live" || q.Options.HooksOptions != "" {
		t.Fatalf("quote = %v %s %+v, %v, want the default route's full %d", q.GrossOutput, q.RouteID, q.Options, err,
			quotedUnits)
	}
}

func TestWrapper_oneRouteFailingLeavesTheOther(t *testing.T) {
	t.Parallel()
	for name, c := range map[string]struct {
		hookless route
		want     string
	}{
		"no hookless route": {route{quote: `{"errorCode":"NoRouteFoundError"}`}, "q-live"},
		"hookless swap reverts otherwise": {route{
			quote: `{"requestId":"q2","routing":"CLASSIC","permitData":null,"permitTransaction":null,` +
				`"quote":{"quoteId":"q-hookless","input":{"amount":"4896325"},"output":{"amount":"1400000"},` +
				`"gasFeeUSD":"0.01","route":[[{"fee":"3000"}]]}}`,
			swap:  `{"swap":{"to":"0xD6145b2d3F379919E8CdEdA7B97E37c4b2Ca9C40","data":"0x3593564c01","value":"0x0"}}`,
			reply: `{"jsonrpc":"2.0","id":1,"error":{"code":3,"data":"0x08c379a0"}}`,
		}, "q-live"},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			r := newWrapperRig(t, chain.NetworkBase)
			r.hookless = &c.hookless

			q, err := r.buy(t, chain.NetworkBase)

			if err != nil || q.RouteID != c.want || q.GrossOutput.Int64() != receivedLive {
				t.Fatalf("quote = %v %s, %v, want the default route penalized to %d", q.GrossOutput, q.RouteID, err,
					receivedLive)
			}
		})
	}
	r := newWrapperRig(t, chain.NetworkBase)
	r.rpcReply = `{"jsonrpc":"2.0","id":1,"error":{"code":3,"data":"0x08c379a0"}}`
	r.hookless = &route{
		quote: strings.Replace(r.quote, `"q-live"`, `"q-hookless"`, 1),
		swap:  strings.Replace(r.swap, "0x3593564c00", "0x3593564c01", 1),
		reply: `{"jsonrpc":"2.0","id":1,"result":"0x"}`,
	}
	if q, err := r.buy(t, chain.NetworkBase); err != nil || q.RouteID != "q-hookless" {
		t.Fatalf("quote = %s, %v, want the hookless route when the default cannot simulate", q.RouteID, err)
	}
}

func TestWrapper_simulatesUnderBasesPerTransactionGasCap(t *testing.T) {
	t.Parallel()
	if evmsim.SimulationGas > 1<<24 {
		t.Fatalf("simulation gas %d exceeds the 16777216 a Base transaction may use (EIP-7825)",
			evmsim.SimulationGas)
	}
}

func TestWrapper_aCallerThatIsGoneGetsNoQuote(t *testing.T) {
	t.Parallel()
	r := newWrapperRig(t, chain.NetworkBase)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := r.client.Quote(ctx, b20Listing(t, chain.NetworkBase), mc.SideBuy,
		money.MicrosFromUint64(liveInput)); err == nil {
		t.Fatal("quote for a cancelled caller, want an error")
	}
}
