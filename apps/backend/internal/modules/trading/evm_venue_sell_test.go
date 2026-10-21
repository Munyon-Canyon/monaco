package trading_test

import (
	"encoding/hex"
	"math/big"
	"net/http"
	"slices"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/trading/app"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/evmsim"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/multichain"
	"github.com/monaco/monaco/apps/backend/internal/testkit/evmcalls"
)

const (
	otherRouter = "0xfdf682f51fe81aa4898f0ae2163d8a55c127fbc7"
	simMet      = `{"jsonrpc":"2.0","id":1,"result":"0x"}`
	simReverted = `{"jsonrpc":"2.0","id":1,"error":{"code":3,"data":"0x08c379a0"}}`
	delegated   = `{"jsonrpc":"2.0","id":1,"result":"0xef0100d6cedde84be40893d153be9d467cd6ad37875b28"}`
	undelegated = `{"jsonrpc":"2.0","id":1,"result":"0x"}`
)

func defaultSwap() string {
	return evmcalls.Hex(evmcalls.Honest(venueOwner, venueToken, baseCash, 9_992_500, 995_000))
}

func hooklessSwap() string {
	return evmcalls.Hex(evmcalls.Honest(venueOwner, venueToken, baseCash, 9_992_500, 996_000))
}

func simShort(received int64) string {
	w := func(v int64) string { return hex.EncodeToString(big.NewInt(v).FillBytes(make([]byte, 32))) }
	return `{"jsonrpc":"2.0","id":1,"error":{"code":3,"message":"execution reverted","data":"0x8b063d73` +
		w(1) + w(received) + `"}}`
}

func (e *venueEnv) sellSpec(t *testing.T) app.EVMSwapSpec {
	t.Helper()
	return app.EVMSwapSpec{
		Network: chain.NetworkBase, Owner: evmAddress(t, venueOwner), TokenIn: evmAddress(t, venueToken),
		TokenOut: evmAddress(t, baseCash), AmountIn: 9_992_500, SlippageBps: 100,
	}
}

type sellRoutes struct {
	code                      string
	defaultOut, hooklessOut   string
	defaultRouter, altRouter  string
	finalOut, finalMin        string
	defaultSim, hooklessSim   string
	finalRouter, approvalData string
}

func defaultRoutes() sellRoutes {
	return sellRoutes{
		code:          delegated,
		defaultOut:    "1000000",
		hooklessOut:   "990000",
		defaultRouter: venueRouter,
		altRouter:     venueRouter,
		finalOut:      "995000",
		finalMin:      "985050",
		defaultSim:    simMet,
		hooklessSim:   simMet,
		finalRouter:   venueRouter,
		approvalData:  abiCall("095ea7b3", word(permit2), word("9992500")),
	}
}

func (e *venueEnv) scriptSell(r sellRoutes) {
	e.uniswap.replies["/v1/check_approval"] = []string{approvalReply(evmTxJSON(venueToken, r.approvalData, "0x00"))}
	e.uniswap.replies["/v1/quote"] = []string{
		quoteReply(r.defaultOut, r.defaultOut, "null"), quoteReply(r.hooklessOut, r.hooklessOut, "null"),
		quoteReply(r.finalOut, r.finalOut, "null"), quoteReply(r.finalOut, r.finalMin, "null"),
	}
	e.uniswap.replies["/v1/swap"] = []string{
		swapReply(
			evmTxJSON(r.defaultRouter, defaultSwap(), "0x0"),
		),
		swapReply(evmTxJSON(r.altRouter, hooklessSwap(), "0x0")),
		swapReply(evmTxJSON(r.finalRouter, defaultSwap(), "0x0")),
	}
	e.rpc.replies["/rpc"] = []string{r.code, r.defaultSim, r.hooklessSim}
}

func (e *venueEnv) sell(t *testing.T, r sellRoutes) (app.EVMSwapPlan, error) {
	t.Helper()
	e.scriptSell(r)
	spec := e.sellSpec(t)
	built, err := e.venue.Build(t.Context(), spec)
	if err != nil {
		t.Fatal(err)
	}
	if len(built.Candidates) != 2 || len(built.Approvals) != 1 || len(e.rpc.calls) != 0 {
		t.Fatalf("built %+v after %d simulations, want two candidates, the approval and no simulation yet",
			built, len(e.rpc.calls))
	}
	return e.venue.Choose(t.Context(), spec, built.Candidates)
}

func (e *venueEnv) finalQuote(t *testing.T) map[string]any {
	t.Helper()
	var quotes []map[string]any
	for _, c := range e.uniswap.calls {
		if c.path == "/v1/quote" {
			quotes = append(quotes, c.body)
		}
	}
	return quotes[len(quotes)-1]
}

func TestEVMVenue_aSellTakesTheHooklessRouteWhenTheDefaultSimulatesShort(t *testing.T) {
	t.Parallel()
	e := newVenueEnv(t)
	r := defaultRoutes()
	r.defaultSim = simShort(900_000)

	plan, err := e.sell(t, r)

	if err != nil || plan.OutAmount != 995_000 || plan.MinOut != 985_050 || len(plan.Approvals) != 0 {
		t.Fatalf("plan = %+v, %v, want the final build at the real slippage and no second approval", plan, err)
	}
	final := e.finalQuote(t)
	if final["hooksOptions"] != multichain.NoHooks || final["slippageTolerance"] != 0.99 {
		t.Fatalf("final quote = %+v, want the hookless route at the real slippage", final)
	}
	e.wantSimulationsOnRealState(t)
}

func (e *venueEnv) wantSimulationsOnRealState(t *testing.T) {
	t.Helper()
	e.wantSimulations(
		t,
		func(call map[string]any) bool { return call["from"] == evmsim.EntryPoint && call["to"] == venueOwner },
	)
}

func (e *venueEnv) wantSimulations(t *testing.T, sentAs func(map[string]any) bool) {
	t.Helper()
	if len(e.rpc.calls) != 3 || e.rpc.calls[0].body["method"] != "eth_getCode" {
		t.Fatalf("rpc calls = %+v, want the treasury's code read then one simulation per candidate", e.rpc.calls)
	}
	for _, c := range e.rpc.calls[1:] {
		params, _ := c.body["params"].([]any)
		call, _ := params[0].(map[string]any)
		if c.body["method"] != "eth_call" || call["gas"] != "0x2dc6c0" || len(params) != 2 || !sentAs(call) {
			t.Fatalf("simulation = %+v, want an eth_call at 3,000,000 gas with no overrides", c.body)
		}
	}
}

func TestEVMVenue_anUndelegatedTreasurySimulatesItsSellAsADirectCall(t *testing.T) {
	t.Parallel()
	e := newVenueEnv(t)
	r := defaultRoutes()
	r.code, r.defaultSim = undelegated, simShort(900_000)

	plan, err := e.sell(t, r)

	if final := e.finalQuote(t); err != nil || final["hooksOptions"] != multichain.NoHooks {
		t.Fatalf("plan = %+v, %v, final quote %+v, want the hookless route the direct call proved", plan, err, final)
	}
	e.wantSimulations(t, func(call map[string]any) bool {
		return call["from"] == venueOwner && call["to"] == venueRouter &&
			(call["data"] == defaultSwap() || call["data"] == hooklessSwap())
	})
}

func TestEVMVenue_aSellKeepsTheDefaultRouteWhenItSimulatesHigher(t *testing.T) {
	t.Parallel()
	for name, hookless := range map[string]string{
		"hookless short":    simShort(980_000),
		"both meet":         simMet,
		"hookless reverted": simReverted,
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			e := newVenueEnv(t)
			r := defaultRoutes()
			r.hooklessSim = hookless

			if plan, err := e.sell(t, r); err != nil || plan.OutAmount != 995_000 {
				t.Fatalf("plan = %+v, %v, want the default route", plan, err)
			}
			if _, set := e.finalQuote(t)["hooksOptions"]; set {
				t.Fatalf("final quote = %+v, want the default route", e.finalQuote(t))
			}
		})
	}
}

func TestEVMVenue_aSellWhoseDefaultRouteRevertsTakesTheHooklessOne(t *testing.T) {
	t.Parallel()
	e := newVenueEnv(t)
	r := defaultRoutes()
	r.defaultSim = simReverted
	r.hooklessSim = simShort(1)

	if _, err := e.sell(t, r); err != nil || e.finalQuote(t)["hooksOptions"] != multichain.NoHooks {
		t.Fatalf("Choose err = %v, final quote = %+v, want the hookless route", err, e.finalQuote(t))
	}
}

func TestEVMVenue_aSellWithNoRouteThatSimulatesIsNoRoute(t *testing.T) {
	t.Parallel()
	e := newVenueEnv(t)
	r := defaultRoutes()
	r.defaultSim = simReverted
	r.hooklessSim = `{"jsonrpc":"2.0","id":1,"error":{"code":3,"data":"0x39d35496"}}`

	_, err := e.sell(t, r)

	if errs.CodeOf(err) != errs.CodeNoRoute || errs.Retryable(errs.CodeOf(err)) {
		t.Fatalf("Choose err = %v, want no_route, which does not retry", err)
	}
	swaps := slices.DeleteFunc(e.uniswap.paths(), func(p string) bool { return p != "/v1/swap" })
	if len(swaps) != 2 {
		t.Fatalf("calls %v, want the two candidates built and no final build", e.uniswap.paths())
	}
}

func TestEVMVenue_aSellWhoseRoutesUseDifferentRoutersBuildsTheDefault(t *testing.T) {
	t.Parallel()
	for name, mutate := range map[string]func(*sellRoutes){
		"different routers": func(r *sellRoutes) { r.altRouter = otherRouter },
		"no hookless swap":  func(r *sellRoutes) { r.altRouter = "0x12" },
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			e := newVenueEnv(t)
			r := defaultRoutes()
			mutate(&r)
			e.scriptSell(r)

			plan, err := e.venue.Build(t.Context(), e.sellSpec(t))

			if err != nil || len(plan.Candidates) != 0 || plan.OutAmount != 995_000 || plan.MinOut != 985_050 ||
				len(plan.Approvals) != 1 || plan.Swap.To != evmAddress(t, venueRouter) {
				t.Fatalf("plan = %+v, %v, want the default route built at the real slippage", plan, err)
			}
			if _, set := e.finalQuote(t)["hooksOptions"]; set || len(e.rpc.calls) != 0 {
				t.Fatalf("final quote = %+v after %d simulations, want the default and none",
					e.finalQuote(t), len(e.rpc.calls))
			}
		})
	}
}

func TestEVMVenue_aSellWhoseDefaultCandidateCannotBuildFails(t *testing.T) {
	t.Parallel()
	e := newVenueEnv(t)
	e.uniswap.replies["/v1/check_approval"] = []string{approvalReply("null")}
	e.uniswap.replies["/v1/quote"] = []string{`{"errorCode":"NoRouteFoundError"}`}
	e.uniswap.status["/v1/quote"] = http.StatusNotFound

	if _, err := e.venue.Build(t.Context(), e.sellSpec(t)); errs.CodeOf(err) != errs.CodeNoRoute {
		t.Fatalf("Build err = %v, want no_route", err)
	}
}

func TestEVMVenue_chooseRefusesWhatItCannotSimulateOrBuild(t *testing.T) {
	t.Parallel()
	candidates := func(t *testing.T) []app.EVMSwapCandidate {
		t.Helper()
		swap := multichain.EVMTransaction{Network: chain.NetworkBase, To: evmAddress(t, venueRouter), Data: []byte{1}}
		return []app.EVMSwapCandidate{{Swap: swap, OutAmount: 1_000_000}}
	}
	for name, c := range map[string]struct {
		network chain.Network
		code    string
		sim     string
		final   []string
		router  string
		want    errs.Code
	}{
		"no rpc":          {network: chain.NetworkArbitrum, want: errs.CodeNoRoute},
		"code unreadable": {network: chain.NetworkBase, code: `<html>`, want: errs.CodeDecodeFailed},
		"code refused": {
			network: chain.NetworkBase, code: `{"jsonrpc":"2.0","id":1,"error":{"code":-32000,"message":"busy"}}`,
			want: errs.CodeRPCUnavailable,
		},
		"rpc unreadable": {network: chain.NetworkBase, code: delegated, sim: `<html>`, want: errs.CodeDecodeFailed},
		"final failed": {
			network: chain.NetworkBase, code: delegated, sim: simMet,
			final: []string{`{"errorCode":"NoRouteFoundError"}`}, want: errs.CodeNoRoute,
		},
		"final elsewhere": {
			network: chain.NetworkBase, code: delegated, sim: simMet, router: otherRouter, want: errs.CodeDecodeFailed,
			final: []string{quoteReply("995000", "995000", "null"), quoteReply("995000", "985050", "null")},
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			e := newVenueEnv(t)
			e.rpc.replies["/rpc"] = []string{c.code, c.sim}
			e.uniswap.replies["/v1/quote"] = c.final
			e.uniswap.replies["/v1/swap"] = []string{swapReply(evmTxJSON(c.router, defaultSwap(), "0x0"))}
			spec := e.sellSpec(t)
			spec.Network = c.network

			if _, err := e.venue.Choose(t.Context(), spec, candidates(t)); errs.CodeOf(err) != c.want {
				t.Fatalf("Choose err = %v, want %s", err, c.want)
			}
		})
	}
}
