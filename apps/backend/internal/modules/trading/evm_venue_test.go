package trading_test

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"io"
	"math/big"
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
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/multichain"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpclient"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

func venueNow() time.Time { return time.Date(2026, 10, 11, 12, 0, 0, 0, time.UTC) }

func expiresIn(d time.Duration) string { return strconv.FormatInt(venueNow().Add(d).Unix(), 10) }

const (
	venueOwner  = "0x00000000000000000000000000000000000000aa"
	venueToken  = "0x00000000000000000000000000000000000000bb"
	venueRouter = "0x6ff5693b99212da76ad316178a184ab56d299b43"
	permit2     = "0x000000000022d473030f116ddee9f6b43ac78ba3"
	venueHash   = "0x1111111111111111111111111111111111111111111111111111111111111111"
	transferSig = "0xddf252ad1be2c89b69c2b068fc378daa952ba7f163c4a11628f55a4df523b3ef"
)

type venueCall struct {
	path, key string
	body      map[string]any
}

type uniswapStub struct {
	mu      sync.Mutex
	calls   []venueCall
	replies map[string][]string
	status  map[string]int
}

func (u *uniswapStub) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	raw, _ := io.ReadAll(r.Body)
	var body map[string]any
	_ = json.Unmarshal(raw, &body)
	u.mu.Lock()
	u.calls = append(u.calls, venueCall{path: r.URL.Path, key: r.Header.Get("x-api-key"), body: body})
	replies := u.replies[r.URL.Path]
	reply := `{}`
	if len(replies) > 0 {
		reply, u.replies[r.URL.Path] = replies[0], replies[1:]
	}
	status := u.status[r.URL.Path]
	u.mu.Unlock()
	if status != 0 {
		w.WriteHeader(status)
	}
	_, _ = w.Write([]byte(reply))
}

func (u *uniswapStub) paths() []string {
	u.mu.Lock()
	defer u.mu.Unlock()
	out := make([]string, 0, len(u.calls))
	for _, c := range u.calls {
		out = append(out, c.path)
	}
	return out
}

func approvalReply(tx string) string { return `{"requestId":"r1","approval":` + tx + `,"cancel":null}` }

func quoteReply(out, minAmount, permit string) string {
	return `{"requestId":"r2","routing":"CLASSIC","quote":{"quoteId":"q1","chainId":8453,"swapper":"` + venueOwner +
		`","input":{"token":"` + baseCash + `","amount":"9992500"},"output":{"token":"` + venueToken +
		`","amount":"` + out + `","recipient":"` + venueOwner + `"},"aggregatedOutputs":[{"token":"` + venueToken +
		`","amount":"` + out + `","recipient":"0x00000000000000000000000000000000000000cc","minAmount":"1"},{"token":"` +
		venueToken + `","amount":"` + out + `","recipient":"` + strings.ToUpper(venueOwner[:2]) + venueOwner[2:] +
		`","minAmount":"` + minAmount + `"}]},"permitTransaction":` + permit + `}`
}

func swapReply(tx string) string { return `{"requestId":"r3","swap":` + tx + `}` }

func evmTxJSON(to, data, value string) string {
	return `{"to":"` + to + `","from":"` + venueOwner + `","data":"` + data + `","value":"` + value +
		`","chainId":8453,"gasLimit":"200000"}`
}

type venueEnv struct {
	uniswap *uniswapStub
	rpc     *uniswapStub
	venue   *adapters.EVMVenue
}

func newVenueEnv(t *testing.T) *venueEnv {
	t.Helper()
	e := &venueEnv{
		uniswap: &uniswapStub{replies: map[string][]string{}, status: map[string]int{}},
		rpc:     &uniswapStub{replies: map[string][]string{}, status: map[string]int{}},
	}
	uni, rpc := httptest.NewServer(e.uniswap), httptest.NewServer(e.rpc)
	t.Cleanup(uni.Close)
	t.Cleanup(rpc.Close)
	client := func(name, url string) *httpclient.Client {
		return httpclient.New(name, httpclient.WithBaseURL(url), httpclient.WithTimeout(time.Second))
	}
	e.venue = adapters.NewEVMVenue(client("uniswap-test", uni.URL), "uniswap-key",
		map[chain.Network]*httpclient.Client{chain.NetworkBase: client("evm-rpc-base-test", rpc.URL+"/rpc")},
		testkit.NewClock(venueNow()))
	return e
}

func (e *venueEnv) spec(t *testing.T) app.EVMSwapSpec {
	t.Helper()
	return app.EVMSwapSpec{
		Network: chain.NetworkBase, Owner: evmAddress(t, venueOwner), TokenIn: evmAddress(t, baseCash),
		TokenOut: evmAddress(t, venueToken), AmountIn: 9_992_500, MinOut: 990_000,
	}
}

func (e *venueEnv) script(approval, probe, quote, swap string) {
	e.uniswap.replies["/v1/check_approval"] = []string{approval}
	e.uniswap.replies["/v1/quote"] = []string{probe, quote}
	e.uniswap.replies["/v1/swap"] = []string{swap}
}

func TestEVMVenue_buildChecksTheApprovalQuotesAtTheFloorAndReturnsTheSwap(t *testing.T) {
	t.Parallel()
	e := newVenueEnv(t)
	approveData := abiCall("095ea7b3", word(permit2), word("9992500"))
	e.script(approvalReply(evmTxJSON(baseCash, approveData, "0x00")), quoteReply("1000000", "1000000", "null"),
		quoteReply("1000100", "990200", "null"), swapReply(evmTxJSON(venueRouter, "0x3593564c", "0x0")))

	plan, err := e.venue.Build(t.Context(), e.spec(t))
	if err != nil {
		t.Fatal(err)
	}
	approval, _ := hex.DecodeString(strings.TrimPrefix(approveData, "0x"))
	want := app.EVMSwapPlan{
		Approvals: []multichain.EVMTransaction{{
			Network: chain.NetworkBase, To: evmAddress(t, baseCash), Data: approval, Value: new(big.Int),
		}},
		Swap: multichain.EVMTransaction{
			Network: chain.NetworkBase, To: evmAddress(t, venueRouter), Data: []byte{0x35, 0x93, 0x56, 0x4c},
			Value: new(big.Int),
		},
		OutAmount: 1_000_100, MinOut: 990_200,
	}
	if !samePlan(plan, want) {
		t.Fatalf("plan = %+v, want %+v", plan, want)
	}
	e.wantBuildCalls(t)
}

func (e *venueEnv) wantBuildCalls(t *testing.T) {
	t.Helper()
	calls := e.uniswap.calls
	if !slices.Equal(e.uniswap.paths(), []string{"/v1/check_approval", "/v1/quote", "/v1/quote", "/v1/swap"}) {
		t.Fatalf("calls = %v, want approval, probe quote, quote at the floor, swap", e.uniswap.paths())
	}
	for _, c := range calls {
		if c.key != "uniswap-key" {
			t.Fatalf("%s sent key %q", c.path, c.key)
		}
	}
	if calls[0].body["amount"] != "9992500" || calls[0].body["walletAddress"] != venueOwner ||
		calls[1].body["slippageTolerance"] != 0.0 || calls[2].body["slippageTolerance"] != 0.99 ||
		calls[2].body["type"] != "EXACT_INPUT" || calls[2].body["tokenOut"] != venueToken {
		t.Fatalf("bodies = %+v, want the approval check and a quote whose tolerance keeps the floor", calls)
	}
	if q, ok := calls[3].body["quote"].(map[string]any); !ok || q["quoteId"] != "q1" {
		t.Fatalf("swap body = %+v, want the quote passed back", calls[3].body)
	}
}

func samePlan(a, b app.EVMSwapPlan) bool {
	sameTx := func(x, y multichain.EVMTransaction) bool {
		return x.Network == y.Network && x.To == y.To && slices.Equal(x.Data, y.Data) && x.Value.Cmp(y.Value) == 0
	}
	return a.MinOut == b.MinOut && a.OutAmount == b.OutAmount && sameTx(a.Swap, b.Swap) &&
		slices.EqualFunc(a.Approvals, b.Approvals, sameTx)
}

func TestEVMVenue_buildAddsThePermitTransactionAndSkipsAnApprovedToken(t *testing.T) {
	t.Parallel()
	e := newVenueEnv(t)
	permit := evmTxJSON(permit2, abiCall("87517c45", word(baseCash), word(venueRouter), word("9992500"),
		word(expiresIn(10*time.Minute))), "")
	e.script(approvalReply("null"), quoteReply("990000", "990000", "null"), quoteReply("990000", "990000", permit),
		swapReply(evmTxJSON(venueRouter, "0x3593564c", "0x0")))

	plan, err := e.venue.Build(t.Context(), e.spec(t))
	if err != nil || len(plan.Approvals) != 1 ||
		plan.Approvals[0].To != evmAddress(t, permit2) ||
		plan.Swap.Value.Sign() != 0 ||
		plan.MinOut != 990_000 {
		t.Fatalf("plan = %+v, %v, want the permit as the one approval", plan, err)
	}
	if e.uniswap.calls[2].body["slippageTolerance"] != 0.0 {
		t.Fatalf("tolerance = %v, want 0 when the quote is the floor", e.uniswap.calls[2].body["slippageTolerance"])
	}
}

func TestEVMVenue_buildRefusesAQuoteUnderTheFloorBeforeTheSwap(t *testing.T) {
	t.Parallel()
	e := newVenueEnv(t)
	e.script(approvalReply("null"), quoteReply("989999", "989999", "null"), "", "")

	if _, err := e.venue.Build(t.Context(), e.spec(t)); errs.CodeOf(err) != errs.CodePriceMoved {
		t.Fatalf("Build err = %v, want price_moved", err)
	}
	if !slices.Equal(e.uniswap.paths(), []string{"/v1/check_approval", "/v1/quote"}) {
		t.Fatalf("calls = %v, want no swap", e.uniswap.paths())
	}
}

func TestEVMVenue_buildMapsUpstreamErrors(t *testing.T) {
	t.Parallel()
	ok := approvalReply("null")
	probe := quoteReply("1000000", "1000000", "null")
	quote := quoteReply("1000000", "990000", "null")
	swap := swapReply(evmTxJSON(venueRouter, "0x01", "0x0"))
	for name, tc := range map[string]struct {
		approval, probe, quote, swap string
		status                       map[string]int
		spec                         func(*app.EVMSwapSpec)
		want                         errs.Code
	}{
		"no route": {
			ok, `{"errorCode":"NoRouteFoundError"}`, "", "",
			map[string]int{"/v1/quote": 404},
			nil,
			errs.CodeNoRoute,
		},
		"refused":   {`{"errorCode":"VALIDATION_ERROR"}`, "", "", "", map[string]int{"/v1/check_approval": 400}, nil, errs.CodeInvalidInput},
		"outage":    {"", "", "", "", map[string]int{"/v1/check_approval": 503}, nil, errs.CodeUpstreamUnavailable},
		"not json":  {"<html>", "", "", "", nil, nil, errs.CodeDecodeFailed},
		"dutch":     {ok, `{"routing":"DUTCH_V2","quote":{}}`, "", "", nil, nil, errs.CodeDecodeFailed},
		"no quote":  {ok, `{"routing":"CLASSIC"}`, "", "", nil, nil, errs.CodeDecodeFailed},
		"bad quote": {ok, `{"routing":"CLASSIC","quote":[]}`, "", "", nil, nil, errs.CodeDecodeFailed},
		"bad out":   {ok, quoteReply("1e6", "1", "null"), "", "", nil, nil, errs.CodeDecodeFailed},
		"requote":   {ok, probe, `{"errorCode":"NoRouteFoundError"}`, "", nil, nil, errs.CodeNoRoute},
		"no swap":   {ok, probe, quote, `{"swap":null}`, nil, nil, errs.CodeDecodeFailed},
		"swap 500":  {ok, probe, quote, "", map[string]int{"/v1/swap": 500}, nil, errs.CodeUpstreamUnavailable},
		"no min": {
			ok, probe, strings.Replace(quote, `"minAmount":"990000"`, `"minAmount":"x"`, 1), swap, nil, nil,
			errs.CodeDecodeFailed,
		},
		"no owner": {
			ok, probe, strings.ReplaceAll(quote, strings.ToUpper(venueOwner[:2])+venueOwner[2:], "0xbad"), swap,
			nil, nil, errs.CodeDecodeFailed,
		},
		"bad to":      {ok, probe, quote, swapReply(evmTxJSON("0x12", "0x01", "0x0")), nil, nil, errs.CodeDecodeFailed},
		"bad data":    {ok, probe, quote, swapReply(evmTxJSON(venueRouter, "0x1", "0x0")), nil, nil, errs.CodeDecodeFailed},
		"bad value":   {ok, probe, quote, swapReply(evmTxJSON(venueRouter, "0x01", "0xzz")), nil, nil, errs.CodeDecodeFailed},
		"bad approve": {approvalReply(evmTxJSON("0x12", "0x01", "0x0")), probe, quote, swap, nil, nil, errs.CodeDecodeFailed},
		"no amount":   {"", "", "", "", nil, func(s *app.EVMSwapSpec) { s.AmountIn = 0 }, errs.CodeInvalidInput},
		"solana":      {"", "", "", "", nil, func(s *app.EVMSwapSpec) { s.Network = chain.NetworkSolana }, errs.CodeInvalidInput},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			e := newVenueEnv(t)
			e.script(tc.approval, tc.probe, tc.quote, tc.swap)
			for path, status := range tc.status {
				e.uniswap.status[path] = status
			}
			spec := e.spec(t)
			if tc.spec != nil {
				tc.spec(&spec)
			}
			if _, err := e.venue.Build(t.Context(), spec); errs.CodeOf(err) != tc.want {
				t.Fatalf("Build err = %v, want %s", err, tc.want)
			}
		})
	}
}

func TestEVMVenue_buildStopsWhenTheCallerIsGone(t *testing.T) {
	t.Parallel()
	e := newVenueEnv(t)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	if _, err := e.venue.Build(ctx, e.spec(t)); errs.CodeOf(err) != errs.CodeUpstreamUnavailable {
		t.Fatalf("Build err = %v, want upstream_unavailable", err)
	}
	if n := len(e.uniswap.paths()); n != 0 {
		t.Fatalf("%d calls, want none", n)
	}
}

func receiptReply(status string, logs ...string) string {
	return `{"jsonrpc":"2.0","id":1,"result":{"transactionHash":"` + venueHash + `","status":"` + status +
		`","logs":[` + strings.Join(logs, ",") + `]}}`
}

func transferLog(token, to, amountHex string) string {
	pad := func(a string) string { return "0x000000000000000000000000" + strings.TrimPrefix(a, "0x") }
	return `{"address":"` + token + `","topics":["` + transferSig + `","` + pad(venueRouter) + `","` + pad(to) +
		`"],"data":"0x` + strings.Repeat("0", 64-len(amountHex)) + amountHex + `"}`
}

func (e *venueEnv) fill(t *testing.T, reply string, status int) (app.EVMFill, error) {
	t.Helper()
	e.rpc.replies["/rpc"] = []string{reply}
	e.rpc.status["/rpc"] = status
	return e.venue.Fill(t.Context(), chain.NetworkBase, venueHash, evmAddress(t, venueToken), evmAddress(t, venueOwner))
}

func TestEVMVenue_fillReadsTheReceipt(t *testing.T) {
	t.Parallel()
	upper := strings.ToUpper(venueToken[:2]) + venueToken[2:]
	for name, tc := range map[string]struct {
		reply string
		want  app.EVMFill
	}{
		"pending":  {`{"jsonrpc":"2.0","id":1,"result":null}`, app.EVMFill{State: app.FillPending}},
		"reverted": {receiptReply("0x0"), app.EVMFill{State: app.FillReverted}},
		"filled": {receiptReply("0x1",
			transferLog(venueToken, venueOwner, "f4240"),
			transferLog(upper, venueOwner, "64"),
			transferLog(venueToken, "0x00000000000000000000000000000000000000cc", "ffff"),
			transferLog(baseCash, venueOwner, "ffff"),
			`{"address":"`+venueToken+`","topics":["0x01"],"data":"0x"}`,
		), app.EVMFill{State: app.FillSucceeded, Out: 1_000_100}},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			e := newVenueEnv(t)
			got, err := e.fill(t, tc.reply, 0)
			if err != nil || got != tc.want {
				t.Fatalf("Fill = %+v, %v, want %+v", got, err, tc.want)
			}
			if body := e.rpc.calls[0].body; body["method"] != "eth_getTransactionReceipt" ||
				body["params"].([]any)[0] != venueHash {
				t.Fatalf("rpc body = %+v", body)
			}
		})
	}
}

func TestEVMVenue_fillRefusesWhatItCannotRead(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct {
		reply  string
		status int
		want   errs.Code
	}{
		"rpc error":  {`{"jsonrpc":"2.0","id":1,"error":{"code":-32000,"message":"header not found"}}`, 0, errs.CodeRPCUnavailable},
		"outage":     {`{}`, 503, errs.CodeRPCUnavailable},
		"refused":    {`{}`, 404, errs.CodeRPCUnavailable},
		"not json":   {`<html>`, 0, errs.CodeDecodeFailed},
		"status":     {receiptReply("0x2"), 0, errs.CodeDecodeFailed},
		"short data": {receiptReply("0x1", strings.Replace(transferLog(venueToken, venueOwner, "1"), `"data":"0x`, `"data":"0x0`, 1)), 0, errs.CodeDecodeFailed},
		"overflow": {receiptReply("0x1", transferLog(venueToken, venueOwner, strings.Repeat("f", 16)),
			transferLog(venueToken, venueOwner, "1")), 0, errs.CodeDecodeFailed},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			e := newVenueEnv(t)
			if _, err := e.fill(t, tc.reply, tc.status); errs.CodeOf(err) != tc.want {
				t.Fatalf("Fill err = %v, want %s", err, tc.want)
			}
		})
	}
	e := newVenueEnv(t)
	for _, call := range []func() error{
		func() error {
			_, err := e.venue.Fill(t.Context(), chain.NetworkArbitrum, venueHash, chain.EVMAddress{}, chain.EVMAddress{})
			return err
		},
		func() error {
			_, err := e.venue.Fill(t.Context(), chain.NetworkBase, "0x12", chain.EVMAddress{}, chain.EVMAddress{})
			return err
		},
	} {
		if err := call(); errs.CodeOf(err) != errs.CodeInvalidInput {
			t.Fatalf("Fill err = %v, want invalid_input", err)
		}
	}
}

func word(v string) string {
	if strings.HasPrefix(v, "0x") {
		return strings.Repeat("0", 24) + strings.TrimPrefix(v, "0x")
	}
	n, _ := new(big.Int).SetString(v, 10)
	h := n.Text(16)
	return strings.Repeat("0", 64-len(h)) + h
}

func abiCall(selector string, words ...string) string {
	return "0x" + selector + strings.Join(words, "")
}

func TestEVMVenue_buildRefusesAnApprovalItDidNotExpect(t *testing.T) {
	t.Parallel()
	maxUint256 := new(big.Int).Sub(new(big.Int).Lsh(big.NewInt(1), 256), big.NewInt(1)).String()
	otherRouter := "0x3fc91a3afd70395cd496c647d5a6cc9d4b2b7fad"
	permitCall := func(token, spender, amount string) string {
		return evmTxJSON(permit2, abiCall("87517c45", word(token), word(spender), word(amount),
			word(expiresIn(time.Minute))), "0x0")
	}
	for name, tc := range map[string]struct{ approval, permit string }{
		"unlimited approval": {evmTxJSON(baseCash, abiCall("095ea7b3", word(permit2), word(maxUint256)), "0x0"), "null"},
		"approval above the amount": {
			evmTxJSON(baseCash, abiCall("095ea7b3", word(permit2), word("9992501")), "0x0"), "null",
		},
		"approval to another spender": {
			evmTxJSON(baseCash, abiCall("095ea7b3", word(otherRouter), word("1")), "0x0"), "null",
		},
		"approval on another token": {
			evmTxJSON(venueToken, abiCall("095ea7b3", word(permit2), word("1")), "0x0"), "null",
		},
		"unknown selector": {evmTxJSON(baseCash, abiCall("39509351", word(permit2), word("1")), "0x0"), "null"},
		"short call":       {evmTxJSON(baseCash, "0x095ea7", "0x0"), "null"},
		"dirty address word": {
			evmTxJSON(baseCash, abiCall("095ea7b3", "ff"+word(permit2)[2:], word("1")), "0x0"), "null",
		},
		"truncated approval":       {evmTxJSON(baseCash, abiCall("095ea7b3", word(permit2)), "0x0"), "null"},
		"permit to another router": {"null", permitCall(baseCash, otherRouter, "1")},
		"permit of another token":  {"null", permitCall(venueToken, venueRouter, "1")},
		"unlimited permit":         {"null", permitCall(baseCash, venueRouter, maxUint256)},
		"permit not on Permit2": {"null", evmTxJSON(otherRouter,
			abiCall("87517c45", word(baseCash), word(venueRouter), word("1"), word(expiresIn(time.Minute))), "0x0")},
		"approval with value": {evmTxJSON(baseCash, abiCall("095ea7b3", word(permit2), word("1")), "0x1"), "null"},
		"permit with value": {"null", evmTxJSON(permit2, abiCall("87517c45", word(baseCash), word(venueRouter),
			word("1"), word(expiresIn(time.Minute))), "0x1")},
		"permit without expiry": {"null", evmTxJSON(permit2, abiCall("87517c45", word(baseCash), word(venueRouter),
			word("1"), word("0")), "0x0")},
		"permit already expired": {"null", evmTxJSON(permit2, abiCall("87517c45", word(baseCash), word(venueRouter),
			word("1"), word(expiresIn(0))), "0x0")},
		"permit past the window": {"null", evmTxJSON(permit2, abiCall("87517c45", word(baseCash), word(venueRouter),
			word("1"), word(expiresIn(30*time.Minute+time.Second))), "0x0")},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			e := newVenueEnv(t)
			e.script(approvalReply(tc.approval), quoteReply("1000000", "1000000", "null"),
				quoteReply("1000000", "990000", tc.permit), swapReply(evmTxJSON(venueRouter, "0x01", "0x0")))

			if _, err := e.venue.Build(t.Context(), e.spec(t)); errs.CodeOf(err) != errs.CodeDecodeFailed {
				t.Fatalf("Build err = %v, want decode_failed", err)
			}
		})
	}
}

func TestEVMVenue_buildRefusesASwapToAnotherRouterOrOnAChainWithoutOne(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct {
		router  string
		network chain.Network
	}{
		"another router": {"0x3fc91a3afd70395cd496c647d5a6cc9d4b2b7fad", chain.NetworkBase},
		"robinhood":      {venueRouter, chain.NetworkRobinhood},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			e := newVenueEnv(t)
			e.script(approvalReply("null"), quoteReply("1000000", "1000000", "null"),
				quoteReply("1000000", "990000", "null"), swapReply(evmTxJSON(tc.router, "0x01", "0x0")))
			spec := e.spec(t)
			spec.Network = tc.network

			if _, err := e.venue.Build(t.Context(), spec); errs.CodeOf(err) != errs.CodeDecodeFailed {
				t.Fatalf("Build err = %v, want decode_failed", err)
			}
		})
	}
}

func TestEVMVenue_fillNeverConfirmsASwapThatCreditedNothing(t *testing.T) {
	t.Parallel()
	e := newVenueEnv(t)
	got, err := e.fill(
		t,
		receiptReply("0x1", transferLog(venueToken, "0x00000000000000000000000000000000000000cc", "64")),
		0,
	)

	if errs.CodeOf(err) != errs.CodeDecodeFailed || got.State == app.FillSucceeded {
		t.Fatalf("Fill = %+v, %v, want decode_failed and no success", got, err)
	}
}

func TestEVMVenue_buildAcceptsEachNetworksUniversalRouter(t *testing.T) {
	t.Parallel()
	for network, router := range map[chain.Network]string{
		chain.NetworkEthereum: "0x66a9893cc07d91d95644aedd05d03f95e1dba8af",
		chain.NetworkArbitrum: "0xa51afafe0263b40edaef0df8781ea9aa03e381a3",
	} {
		t.Run(string(network), func(t *testing.T) {
			t.Parallel()
			e := newVenueEnv(t)
			e.script(approvalReply("null"), quoteReply("1000000", "1000000", "null"),
				quoteReply("1000000", "990000", "null"), swapReply(evmTxJSON(router, "0x01", "0x0")))
			spec := e.spec(t)
			spec.Network = network

			if plan, err := e.venue.Build(t.Context(), spec); err != nil || plan.Swap.To != evmAddress(t, router) {
				t.Fatalf("Build = %+v, %v, want the swap through %s", plan, err, router)
			}
		})
	}
}

func TestEVMVenue_buildAcceptsANewerRouterAndHoldsThePermitToIt(t *testing.T) {
	t.Parallel()
	const router212 = "0xd6145b2d3f379919e8cdeda7b97e37c4b2ca9c40"
	permitTo := func(spender string) string {
		return evmTxJSON(permit2, abiCall("87517c45", word(baseCash), word(spender), word("1"),
			word(expiresIn(30*time.Minute))), "0x0")
	}
	for name, tc := range map[string]struct {
		spender string
		ok      bool
	}{
		"same router":    {router212, true},
		"another in set": {venueRouter, false},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			e := newVenueEnv(t)
			e.script(approvalReply("null"), quoteReply("1000000", "1000000", "null"),
				quoteReply("1000000", "990000", permitTo(tc.spender)), swapReply(evmTxJSON(router212, "0x01", "0x0")))

			plan, err := e.venue.Build(t.Context(), e.spec(t))
			if tc.ok && (err != nil || plan.Swap.To != evmAddress(t, router212) || len(plan.Approvals) != 1) {
				t.Fatalf("Build = %+v, %v, want the 2.1.2 router accepted with its permit", plan, err)
			}
			if !tc.ok && errs.CodeOf(err) != errs.CodeDecodeFailed {
				t.Fatalf("Build err = %v, want decode_failed for a permit to another router", err)
			}
		})
	}
}

func TestEVMVenue_buildRefusesASwapThatSendsValue(t *testing.T) {
	t.Parallel()
	e := newVenueEnv(t)
	e.script(approvalReply("null"), quoteReply("1000000", "1000000", "null"),
		quoteReply("1000000", "990000", "null"), swapReply(evmTxJSON(venueRouter, "0x01", "0x1")))

	if _, err := e.venue.Build(t.Context(), e.spec(t)); errs.CodeOf(err) != errs.CodeDecodeFailed {
		t.Fatalf("Build err = %v, want decode_failed for a swap sending native value", err)
	}
}
