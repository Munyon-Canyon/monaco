package trading_test

import (
	"bytes"
	"context"
	"log/slog"
	"math/big"
	"slices"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/trading/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/trading/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/multichain"
	"github.com/monaco/monaco/apps/backend/internal/platform/config"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
	mcfakes "github.com/monaco/monaco/apps/backend/internal/testkit/fakes/multichain"
)

const (
	bridgeFeeMicros = 7_500
	landedMicros    = buyMicros - bridgeFeeMicros
	requotedOut     = 306_000_000
	filledOut       = 305_000_000
)

type listingsFake struct {
	listings map[multichain.ListingID]multichain.Listing
}

func (f listingsFake) Listing(_ context.Context, id multichain.ListingID) (multichain.Listing, error) {
	l, ok := f.listings[id]
	if !ok {
		return multichain.Listing{}, errs.New(errs.CodeAssetNotFound, "listings.Listing")
	}
	return l, nil
}

func (f listingsFake) ListingByAsset(_ context.Context, asset string) (multichain.Listing, error) {
	for _, l := range f.listings {
		if string(l.Network)+":"+l.TokenAddress == asset {
			return l, nil
		}
	}
	return multichain.Listing{}, errs.New(errs.CodeAssetNotFound, "listings.ListingByAsset")
}

type treasuryEVMFake struct {
	wallet multichain.EVMWallet
	err    error
}

func (f *treasuryEVMFake) EnsureTreasuryEVMWallet(context.Context, ids.CabalID) (multichain.EVMWallet, error) {
	return f.wallet, f.err
}

type routeEnv struct {
	*engineEnv
	privy    *privyFake
	evm      *evmFake
	quotes   *mcfakes.Quoter
	treasury *treasuryEVMFake
	listings listingsFake
	listing  multichain.Listing
	route    *multichain.Comparison
	journal  app.Journal
	topUps   *topUpFake
	arrivals *arrivalsFake
	landings *evmArrivalsFake
	cash     *cashFake
}

func newRouteEnv(t *testing.T) *routeEnv {
	t.Helper()
	e := &routeEnv{engineEnv: newEngineEnv(t), privy: newPrivyFake(), quotes: mcfakes.New().Quoter}
	e.topUps = &topUpFake{privy: e.privy}
	e.arrivals = &arrivalsFake{privy: e.privy, short: bridgeFeeMicros}
	e.landings = &evmArrivalsFake{short: bridgeFeeMicros}
	e.cash = &cashFake{balance: money.MicrosFromUint64(1 << 40)}
	stepping := steppingClock{e.clk}
	e.journal = app.Journal{UoW: db.New(e.pool, e.ids, stepping), Reads: e.pool, Clock: stepping, IDs: e.ids}
	id := e.engineEnv.listing(t)
	var token string
	if err := e.pool.QueryRow(t.Context(), `SELECT token_address FROM asset_listings WHERE id = $1`, id.UUID()).
		Scan(&token); err != nil {
		t.Fatal(err)
	}
	e.listing = multichain.Listing{
		ID: id, Symbol: "AAPLc", Network: chain.NetworkBase, TokenAddress: token, Decimals: 8,
		Venue: multichain.VenueUniswap,
	}
	e.listings = listingsFake{listings: map[multichain.ListingID]multichain.Listing{id: e.listing}}
	e.route = e.comparison(t, id)
	e.route.Ranked[0].Costs = []multichain.Cost{
		{Kind: multichain.CostGas, Micros: money.MicrosFromUint64(1)},
		{Kind: multichain.CostBridge, Micros: money.MicrosFromUint64(bridgeFeeMicros)},
	}
	loser := e.route.Ranked[0]
	loser.ListingID, loser.Costs = multichain.ListingIDFrom(e.ids.NewV7()), []multichain.Cost{
		{Kind: multichain.CostBridge, Micros: money.MicrosFromUint64(99)},
	}
	e.route.Ranked = append(e.route.Ranked, loser)
	e.treasury = &treasuryEVMFake{wallet: multichain.EVMWallet{ID: "evm-wallet", Address: evmAddress(t, evmTreasury)}}
	e.evm = newEVMFake(app.EVMSwapPlan{
		Approvals: []multichain.EVMTransaction{{Network: chain.NetworkBase, To: evmAddress(t, baseCash)}},
		Swap:      multichain.EVMTransaction{Network: chain.NetworkBase, To: evmAddress(t, baseRouter)},
		MinOut:    302_940_000,
	})
	e.requote()
	return e
}

func (e *routeEnv) requote() {
	e.quotes.Quotes.Push(mcfakes.Reply[multichain.Quote]{Value: multichain.Quote{GrossOutput: big.NewInt(requotedOut)}})
}

func (e *routeEnv) exec() *app.RouteExec {
	return app.NewRouteExec(app.RouteExecDeps{
		Layer:       e.layer(),
		Journal:     e.journal,
		Wallets:     app.CabalWallets{Cabals: liveCabals{e: e.engineEnv}},
		TreasuryEVM: e.treasury,
		Listings:    e.listings,
		Quoter:      e.quotes,
		Actions:     e.privy,
		Sender:      e.evm,
		Venue:       e.evm,
		TopUp:       e.topUps,
		Arrivals:    e.arrivals,
		EVMArrivals: e.landings,
		Cash:        e.cash,
		Poll:        app.DefaultPoll(),
	})
}

func (e *routeEnv) request() app.SwapRequest {
	return app.SwapRequest{
		Source:  domain.Source{Kind: domain.SourceProposal, ID: e.ids.NewV7()},
		CabalID: e.cabal, Action: domain.ActionBuy, Symbol: "AAPLc", InAmount: buyMicros, QuoteOutAmount: quotedOut,
		SlippageBps: 100, SourceBatchSize: 1, Route: e.route,
	}
}

func (e *routeEnv) buy(ctx context.Context, req app.SwapRequest) (app.SwapView, error) {
	return e.exec().Trade(actorContext(ctx), req, e.listing, nil)
}

func (e *routeEnv) mustBuy(t *testing.T, req app.SwapRequest, want domain.Status) app.SwapView {
	t.Helper()
	view, err := e.buy(t.Context(), req)
	if err != nil || view.Status != want {
		t.Fatalf("Buy = %+v, %v, want %s", view, err, want)
	}
	return view
}

func (e *routeEnv) resume(t *testing.T, id ids.SwapID, want domain.Status) {
	t.Helper()
	if err := e.exec().Resume(actorContext(t.Context()), id); err != nil {
		t.Fatal(err)
	}
	if status, _, _, _ := e.row(t, id.UUID()); status != string(want) {
		t.Fatalf("after the resume the swap is %s, want %s", status, want)
	}
}

func (e *routeEnv) steps(t *testing.T, swap uuid.UUID) []string {
	t.Helper()
	rows, err := e.pool.Query(t.Context(), `SELECT payload->>'step', payload->>'status', payload->>'fee_micros'
		FROM events WHERE type = 'trade.step_changed' AND aggregate_id = $1 ORDER BY id`, swap)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var step, status, fee string
		if err := rows.Scan(&step, &status, &fee); err != nil {
			t.Fatal(err)
		}
		out = append(out, step+":"+status+":"+fee)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}

func (e *routeEnv) failure(t *testing.T, swap uuid.UUID) string {
	t.Helper()
	var code string
	if err := e.pool.QueryRow(t.Context(), `SELECT coalesce(failure_code, '') FROM swaps WHERE id = $1`, swap).
		Scan(&code); err != nil {
		t.Fatal(err)
	}
	return code
}

func wantSteps(t *testing.T, got []string, want ...string) {
	t.Helper()
	if !slices.Equal(got, want) {
		t.Fatalf("step events = %v, want %v", got, want)
	}
}

func bridged() []string {
	return []string{"bridge_out:pending:0", "bridge_out:submitted:7500", "bridge_out:succeeded:7500"}
}

func (e *routeEnv) wantEVMRow(t *testing.T, swap uuid.UUID, hash string) {
	t.Helper()
	cash, token := "base:"+baseCash, "base:"+strings.ToLower(e.listing.TokenAddress)
	var inMint, outMint, treasury string
	var inAmount, outAmount, fee int64
	if err := e.pool.QueryRow(t.Context(), `SELECT in_mint, out_mint, treasury_address, in_amount, out_amount,
		fee_micros FROM swaps WHERE id = $1`, swap).
		Scan(&inMint, &outMint, &treasury, &inAmount, &outAmount, &fee); err != nil {
		t.Fatal(err)
	}
	if inMint != cash || outMint != token || treasury != evmTreasury || inAmount != buyMicros ||
		outAmount != filledOut || fee != bridgeFeeMicros {
		t.Fatalf("swap row %s %s %s %d %d %d, want the EVM legs, the sent amount, the fill and the bridge fee",
			inMint, outMint, treasury, inAmount, outAmount, fee)
	}
	confirmed := e.payload(t, swap, "trade.confirmed")
	want := map[string]any{
		"in_mint": cash, "in_amount": "24992500", "out_mint": token, "out_amount": "305000000",
		"usdc_micros": "25000000", "fee_micros": "7500", "tx_signature": hash,
	}
	for k, v := range want {
		if confirmed[k] != v {
			t.Fatalf("trade.confirmed %s = %v, want %v (payload %v)", k, confirmed[k], v, confirmed)
		}
	}
}

func (e *routeEnv) wantOneBridgeAndOneBuild(t *testing.T, swap ids.SwapID) {
	t.Helper()
	calls := e.privy.calls()
	wantSpec := multichain.TransferSpec{
		From: chain.NetworkSolana, To: chain.NetworkBase, Destination: evmTreasury,
		AmountMicros: money.MicrosFromUint64(buyMicros),
	}
	if len(calls) != 1 || calls[0].spec != wantSpec || calls[0].wallet != treasuryWallet ||
		calls[0].key != "trade:"+swap.String()+":bridge_out" {
		t.Fatalf("transfers = %+v, want one bridge of the trade amount from the Solana treasury", calls)
	}
	want := app.EVMSwapSpec{
		Network: chain.NetworkBase, Owner: evmAddress(t, evmTreasury), TokenIn: evmAddress(t, baseCash),
		TokenOut: evmAddress(t, e.listing.TokenAddress), AmountIn: landedMicros, MinOut: 302_940_000, SlippageBps: 100,
	}
	if !slices.Equal(e.evm.specs, []app.EVMSwapSpec{want}) {
		t.Fatalf("built %+v, want the landed amount with a floor from the requote", e.evm.specs)
	}
}

func TestRouteExec_aBuyBridgesRequotesSwapsAndConfirmsInEVMAssets(t *testing.T) {
	t.Parallel()
	e := newRouteEnv(t)
	e.privy.script(multichain.ActionSucceeded)
	e.evm.fills = []app.EVMFill{{State: app.FillSucceeded, Out: filledOut}}

	view := e.mustBuy(t, e.request(), domain.StatusConfirmed)

	e.wantEVMRow(t, view.ID.UUID(), swapHash)
	wantSteps(t, e.steps(t, view.ID.UUID()), append(bridged(),
		"swap:pending:0", "swap:submitted:0", "swap:succeeded:0")...)
	e.wantOneBridgeAndOneBuild(t, view.ID)
}

func TestRouteExec_theSwapIsBuiltOnTheRouteTheRequoteChose(t *testing.T) {
	t.Parallel()
	e := newRouteEnv(t)
	e.quotes = mcfakes.New().Quoter
	e.quotes.Quotes.Push(mcfakes.Reply[multichain.Quote]{Value: multichain.Quote{
		GrossOutput: big.NewInt(requotedOut), Options: multichain.RouteOptions{HooksOptions: "V4_NO_HOOKS"},
	}})
	e.privy.script(multichain.ActionSucceeded)
	e.evm.fills = []app.EVMFill{{State: app.FillSucceeded, Out: filledOut}}

	e.mustBuy(t, e.request(), domain.StatusConfirmed)

	if len(e.evm.specs) != 1 || e.evm.specs[0].Route.HooksOptions != "V4_NO_HOOKS" ||
		e.evm.specs[0].MinOut != 302_940_000 {
		t.Fatalf("built %+v, want the hookless route the requote chose, floored from its output", e.evm.specs)
	}
}

func TestRouteExec_aPendingBridgeAndAPendingFillResumeWithoutASecondBridgeOrSwap(t *testing.T) {
	t.Parallel()
	e := newRouteEnv(t)

	view := e.mustBuy(t, e.request(), domain.StatusCreated)
	e.privy.script(multichain.ActionSucceeded)
	e.resume(t, view.ID, domain.StatusSubmitted)
	e.evm.fills = []app.EVMFill{{State: app.FillSucceeded, Out: filledOut}}
	e.resume(t, view.ID, domain.StatusConfirmed)
	e.resume(t, view.ID, domain.StatusConfirmed)

	if len(e.privy.calls()) != 1 || e.evm.hashes() != 2 || len(e.evm.keys()) != 2 {
		t.Fatalf("transfers %v sends %v, want one bridge and one approval plus one swap", e.privy.calls(), e.evm.keys())
	}
	if n := len(e.events(t, view.ID.UUID())); n != 8 {
		t.Fatalf("%d events, want 6 step changes, trade.submitted and trade.confirmed once each", n)
	}
}

func TestRouteExec_aBridgeFailedWithNothingBroadcastFailsTheSwapNeverSubmitted(t *testing.T) {
	t.Parallel()
	for name, steps := range map[string][]multichain.ActionStep{
		"no steps":     nil,
		"refused step": {{Network: chain.NetworkSolana, State: multichain.StepFailed}},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			e := newRouteEnv(t)
			e.privy.script(multichain.ActionFailed)
			e.privy.failedSteps = steps

			view := e.mustBuy(t, e.request(), domain.StatusFailed)

			if code := e.failure(t, view.ID.UUID()); code != string(domain.FailureNeverSubmitted) {
				t.Fatalf("failure = %s, want never_submitted", code)
			}
			wantSteps(t, e.steps(t, view.ID.UUID()),
				"bridge_out:pending:0", "bridge_out:submitted:7500", "bridge_out:failed:0")
		})
	}
}

func TestRouteExec_aBridgeFailedAfterItBroadcastParksTheSwapAndBooksNoReturn(t *testing.T) {
	t.Parallel()
	e := newRouteEnv(t)
	e.privy.script(multichain.ActionFailed, multichain.ActionFailed)
	e.privy.failedSteps = []multichain.ActionStep{
		{Network: chain.NetworkSolana, TxHash: "solana-hash", State: multichain.StepConfirmed},
		{State: multichain.StepFailed},
	}
	var logs bytes.Buffer
	ctx := observability.WithLogger(t.Context(), slog.New(slog.NewJSONHandler(&logs, nil)))

	view, err := e.buy(ctx, e.request())

	if err != nil || view.Status != domain.StatusCreated {
		t.Fatalf("Buy = %+v, %v, want the swap left created for an operator", view, err)
	}
	var code string
	if err := e.pool.QueryRow(t.Context(), `SELECT coalesce(parked_code, '') FROM swaps WHERE id = $1`, view.ID.UUID()).
		Scan(&code); err != nil {
		t.Fatal(err)
	}
	if code != "bridge_outcome_unknown" {
		t.Fatalf("parked code = %q, want bridge_outcome_unknown", code)
	}
	wantSteps(t, e.steps(t, view.ID.UUID()), "bridge_out:pending:0", "bridge_out:submitted:7500")
	for _, want := range []string{"trading.bridge.outcome_unknown", "trading.routed_swap.parked"} {
		if !strings.Contains(logs.String(), want) {
			t.Fatalf("logs = %s, want %s", logs.String(), want)
		}
	}
	e.resume(t, view.ID, domain.StatusCreated)
	if len(e.privy.calls()) != 1 {
		t.Fatalf("transfers = %v, want the one bridge and no second send", e.privy.calls())
	}
}

func TestRouteExec_aRequoteThatFindsNoRouteFailsPriceMovedWithTheCashOnEVM(t *testing.T) {
	t.Parallel()
	for name, reply := range map[string]mcfakes.Reply[multichain.Quote]{
		"refused": {Code: errs.CodeNoRoute},
		"empty":   {},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			e := newRouteEnv(t)
			e.quotes = mcfakes.New().Quoter
			e.quotes.Quotes.Push(reply)
			e.privy.script(multichain.ActionSucceeded)

			view := e.mustBuy(t, e.request(), domain.StatusFailed)

			if code := e.failure(t, view.ID.UUID()); code != string(domain.FailurePriceMoved) {
				t.Fatalf("failure = %s, want price_moved", code)
			}
			wantSteps(t, e.steps(t, view.ID.UUID()), bridged()...)
		})
	}
}

func TestRouteExec_aRequoteOutageLeavesTheBridgedSwapForTheResume(t *testing.T) {
	t.Parallel()
	e := newRouteEnv(t)
	e.quotes = mcfakes.New().Quoter
	e.quotes.Quotes.Push(mcfakes.Reply[multichain.Quote]{Code: errs.CodeUpstreamUnavailable})
	e.privy.script(multichain.ActionSucceeded)

	req := e.request()
	if _, err := e.buy(t.Context(), req); errs.CodeOf(err) != errs.CodeUpstreamUnavailable {
		t.Fatalf("Buy err = %v, want upstream_unavailable", err)
	}
	id := ids.SwapIDFrom(e.swapIDs(t, req.Source.ID)[0])
	e.requote()
	e.evm.fills = []app.EVMFill{{State: app.FillSucceeded, Out: filledOut}}
	e.resume(t, id, domain.StatusConfirmed)

	if len(e.privy.calls()) != 1 {
		t.Fatalf("transfers = %v, want the one bridge reused", e.privy.calls())
	}
}

func (e *routeEnv) bridgedToTheSwap(t *testing.T) {
	t.Helper()
	e.privy.script(multichain.ActionSucceeded)
}

func TestRouteExec_theSwapOutcomeSettlesTheSwapAndItsStep(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct {
		arrange func(e *routeEnv)
		failure domain.FailureCode
		step    string
	}{
		"plan under the floor": {
			arrange: func(e *routeEnv) { e.evm.plan.MinOut = 302_939_999 },
			failure: domain.FailurePriceMoved, step: "swap:failed:0",
		},
		"build refused": {
			arrange: func(e *routeEnv) { e.evm.buildErr = errs.New(errs.CodeInvalidInput, "uniswap.Build") },
			failure: domain.FailureNeverSubmitted, step: "swap:failed:0",
		},
		"reverted": {
			arrange: func(e *routeEnv) { e.evm.fills = []app.EVMFill{{State: app.FillReverted}} },
			failure: domain.FailureSwapReverted, step: "swap:failed:0",
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			e := newRouteEnv(t)
			e.bridgedToTheSwap(t)
			tc.arrange(e)

			view := e.mustBuy(t, e.request(), domain.StatusFailed)

			if code := e.failure(t, view.ID.UUID()); code != string(tc.failure) {
				t.Fatalf("failure = %s, want %s", code, tc.failure)
			}
			got := e.steps(t, view.ID.UUID())
			if !slices.Equal(got[:3], bridged()) || got[len(got)-1] != tc.step {
				t.Fatalf("step events = %v, want the bridge then %s", got, tc.step)
			}
			if n := len(e.privy.calls()); n != 1 {
				t.Fatalf("%d transfers, want the one bridge, its cash left on the EVM treasury", n)
			}
		})
	}
}

func TestRouteExec_anOutageLeavesTheSwapCreatedForTheResume(t *testing.T) {
	t.Parallel()
	for name, fail := range map[string]func(e *routeEnv){
		"transfer": func(e *routeEnv) { e.privy.transferErr = errs.New(errs.CodePrivyUnavailable, "privy.Transfer") },
		"build": func(e *routeEnv) {
			e.bridgedToTheSwap(t)
			e.evm.buildErr = errs.New(errs.CodePrivyUnavailable, "uniswap.Build")
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			e := newRouteEnv(t)
			fail(e)
			req := e.request()

			if _, err := e.buy(t.Context(), req); errs.CodeOf(err) != errs.CodePrivyUnavailable {
				t.Fatalf("Buy err = %v, want the outage", err)
			}
			if status, _, _, _ := e.row(t, e.swapIDs(t, req.Source.ID)[0]); status != string(domain.StatusCreated) {
				t.Fatalf("swap is %s, want created", status)
			}
		})
	}
}

func TestRouteExec_aFillPastTheColumnRollsTheConfirmBack(t *testing.T) {
	t.Parallel()
	e := newRouteEnv(t)
	e.bridgedToTheSwap(t)
	e.evm.fills = []app.EVMFill{{State: app.FillSucceeded, Out: 1 << 63}}
	req := e.request()

	if _, err := e.buy(t.Context(), req); errs.CodeOf(err) != errs.CodeDecodeFailed {
		t.Fatalf("Buy err = %v, want decode_failed", err)
	}
	swap := e.swapIDs(t, req.Source.ID)[0]
	if status, _, _, _ := e.row(t, swap); status != string(domain.StatusSubmitted) {
		t.Fatalf("swap is %s, want submitted", status)
	}
	if got := e.steps(t, swap); got[len(got)-1] != "swap:submitted:0" {
		t.Fatalf("step events = %v, want the swap step still submitted", got)
	}
}

func TestRouteExec_aBuyNoLongerWantedIsCancelledBeforeTheBridge(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct {
		wanted  func(context.Context) (bool, error)
		failure domain.FailureCode
	}{
		"cancelled": {func(context.Context) (bool, error) { return false, nil }, domain.FailureSourceCancelled},
		"unknown": {
			func(context.Context) (bool, error) { return false, errs.New(errs.CodeUpstreamUnavailable, "") },
			domain.FailureNeverSubmitted,
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			e := newRouteEnv(t)
			req := e.request()
			req.StillWanted = tc.wanted

			_, _ = e.buy(t.Context(), req)

			swap := e.swapIDs(t, req.Source.ID)[0]
			if code := e.failure(t, swap); code != string(tc.failure) || len(e.privy.calls()) != 0 {
				t.Fatalf(
					"failure = %s after %d transfers, want %s and no bridge",
					code,
					len(e.privy.calls()),
					tc.failure,
				)
			}
		})
	}
}

func TestRouteExec_aSecondBuyOfALiveSwapReturnsItWithoutDrivingIt(t *testing.T) {
	t.Parallel()
	e := newRouteEnv(t)
	req := e.request()
	first := e.mustBuy(t, req, domain.StatusCreated)

	again := e.mustBuy(t, req, domain.StatusCreated)

	if again.ID != first.ID || len(e.privy.calls()) != 1 || e.privy.polls != 7 {
		t.Fatalf("second Buy = %s after %d transfers and %d polls, want %s untouched",
			again.ID, len(e.privy.calls()), e.privy.polls, first.ID)
	}
}

func TestRouteExec_refusesWhatItCannotStartOrLoad(t *testing.T) {
	t.Parallel()
	e := newRouteEnv(t)
	e.treasury.err = errs.New(errs.CodeNotImplemented, "cabal.EnsureTreasuryEVMWallet")
	if _, err := e.buy(t.Context(), e.request()); errs.CodeOf(err) != errs.CodeNotImplemented {
		t.Fatalf("Buy without an EVM wallet err = %v, want not_implemented", err)
	}
	e.treasury.err = nil
	huge := e.request()
	huge.InAmount = 1 << 63
	if _, err := e.buy(t.Context(), huge); errs.CodeOf(err) != errs.CodeInvalidInput {
		t.Fatalf("Buy past the column err = %v, want invalid_input", err)
	}
	if err := e.exec().Resume(actorContext(t.Context()), ids.SwapIDFrom(e.ids.NewV7())); err == nil {
		t.Fatal("Resume of an unknown swap succeeded")
	}
	e.route.Ranked[0].Costs = []multichain.Cost{
		{Kind: multichain.CostBridge, Micros: money.MicrosFromUint64(buyMicros + 1)},
	}
	if _, err := e.buy(t.Context(), e.request()); err == nil || len(e.privy.calls()) != 0 {
		t.Fatalf("Buy with a fee above the trade err = %v after %d transfers, want refused", err, len(e.privy.calls()))
	}
}

func TestRouteExec_aResumeThatCannotResolveTheRouteChangesNothing(t *testing.T) {
	t.Parallel()
	e := newRouteEnv(t)
	req := e.request()
	view := e.mustBuy(t, req, domain.StatusCreated)
	e.privy.script(multichain.ActionSucceeded)

	delete(e.listings.listings, e.listing.ID)
	if err := e.exec().Resume(actorContext(t.Context()), view.ID); errs.CodeOf(err) != errs.CodeAssetNotFound {
		t.Fatalf("Resume without the listing err = %v, want asset_not_found", err)
	}
	if _, err := e.pool.Exec(t.Context(), `UPDATE swaps SET in_amount = -1 WHERE id = $1`, view.ID.UUID()); err != nil {
		t.Fatal(err)
	}
	if err := e.exec().Resume(actorContext(t.Context()), view.ID); errs.CodeOf(err) != errs.CodeDecodeFailed {
		t.Fatalf("Resume of a corrupt row err = %v, want decode_failed", err)
	}
	wantSteps(t, e.steps(t, view.ID.UUID()), "bridge_out:pending:0", "bridge_out:submitted:7500")
}

func TestRouteExec_aFillItCannotReadLeavesTheSubmittedSwapForAHuman(t *testing.T) {
	t.Parallel()
	e := newRouteEnv(t)
	e.privy.script(multichain.ActionSucceeded)
	e.evm.fillErr = errs.New(errs.CodeDecodeFailed, "evm.Fill")
	req := e.request()

	if _, err := e.buy(t.Context(), req); errs.CodeOf(err) != errs.CodeDecodeFailed {
		t.Fatalf("Buy err = %v, want decode_failed", err)
	}
	swap := e.swapIDs(t, req.Source.ID)[0]
	if status, _, _, _ := e.row(t, swap); status != string(domain.StatusSubmitted) {
		t.Fatalf("swap is %s, want submitted", status)
	}
	if got := e.steps(t, swap); got[len(got)-1] != "swap:submitted:0" {
		t.Fatalf("step events = %v, want the swap step left submitted", got)
	}
}

func TestRouteExec_aSwapSentButNotRecordedIsNeverFailedAsUnsent(t *testing.T) {
	t.Parallel()
	e := newRouteEnv(t)
	e.privy.script(multichain.ActionSucceeded)
	req := e.request()
	swapKey := func() string { return "trade:" + e.swapIDs(t, req.Source.ID)[0].String() + ":swap" }
	e.evm.afterSendAny = func(key string) {
		if key != swapKey() {
			return
		}
		if _, err := e.pool.Exec(t.Context(), `UPDATE swaps SET status = 'submitted', tx_signature = '0xother',
			submitted_at = now() WHERE source_id = $1`, req.Source.ID); err != nil {
			t.Error(err)
		}
	}

	if _, err := e.buy(t.Context(), req); errs.CodeOf(err) != errs.CodeVersionConflict {
		t.Fatalf("Buy err = %v, want version_conflict from the record", err)
	}
	swap := e.swapIDs(t, req.Source.ID)[0]
	if got := e.steps(t, swap); got[len(got)-1] != "swap:pending:0" {
		t.Fatalf("step events = %v, want the swap step left for the resume, not failed", got)
	}
	if n := e.count(t, `SELECT count(*) FROM events WHERE type = 'trade.failed' AND aggregate_id = $1`, swap); n != 0 {
		t.Fatalf("%d trade.failed, want none for a swap that was sent", n)
	}
}

func TestRouteExec_aRefusedSwapLogsPrivysReason(t *testing.T) {
	t.Parallel()
	e := newRouteEnv(t)
	e.bridgedToTheSwap(t)
	e.evm.buildErr = errs.New(errs.CodeInvalidInput, "privy.SendEVMTransaction", slog.Int("status", 400),
		slog.String("privy_error", "Transaction simulation failed"), slog.String("privy_code", "invalid_data"))
	logs := &testkit.Logs{}
	ctx := observability.WithLogger(t.Context(), observability.NewLogger(config.Config{Env: config.EnvTest}, logs))

	view, err := e.buy(ctx, e.request())
	if err != nil || view.Status != domain.StatusFailed {
		t.Fatalf("Buy = %s, %v, want failed", view.Status, err)
	}

	got := logs.Lines("trading.swap.refused", "swap_id", "step", "code", "detail")
	want := []string{view.ID.String() + " swap invalid_input " +
		"map[privy_code:invalid_data privy_error:Transaction simulation failed status:400]"}
	if !slices.Equal(got, want) {
		t.Fatalf("trading.swap.refused = %q, want %q", got, want)
	}
}
