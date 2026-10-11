package trading_test

import (
	"context"
	"encoding/json"
	"math"
	"os"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	funding "github.com/monaco/monaco/apps/backend/internal/modules/funding/port"
	"github.com/monaco/monaco/apps/backend/internal/modules/trading/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/trading/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/jupiter"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/multichain"
	"github.com/monaco/monaco/apps/backend/internal/platform/config"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

func TestExecuteTrade_feeHeadroomComesFromTheSolanaWinnersMint(t *testing.T) {
	t.Parallel()
	for name, c := range map[string]struct {
		winnerFeeBps uint16
		blocked      bool
	}{
		"winner charges a transfer fee": {winnerFeeBps: 100, blocked: true},
		"winner charges none":           {winnerFeeBps: 0},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			e := newEngineEnv(t)
			ondo := chain.Mint{Address: "AAPLonDo1111111111111111111111111111111111111", Decimals: 9}
			e.ledger.SetTokens(e.wallet.Address, usdcToken(), buyMicros)
			e.balances.setFee(aaplxMint, 500, math.MaxUint64)
			e.balances.setFee(ondo.Address, c.winnerFeeBps, math.MaxUint64)
			e.quote(usdcToken(), ondo, 210_000_000, true)
			order := jupiter.Order{RequestID: "req-1", Transaction: swapTx()}
			e.jup.SetOrder(jupiterMint(usdcToken()), jupiterMint(ondo), order)
			cmd := e.buy()
			cmd.Route = e.comparison(t, e.solanaWinner(t, ondo, true))

			if err := e.handle(t, e.delivery(t, cmd), cmd); err != nil {
				t.Fatal(err)
			}
			blocked := e.blocked(t, cmd.ProposalID)
			if len(blocked) == 1 != c.blocked {
				t.Fatalf("blocked = %v, want blocked %v by the winner's own fee", blocked, c.blocked)
			}
			if c.blocked && blocked[0]["need"] != "25250000" {
				t.Fatalf("need %v, want the trade plus the winner's 1%% fee", blocked[0]["need"])
			}
		})
	}
}

func TestSellForCashOut_aShortPlanAcrossChainsSellsEverythingAndAlertsOnce(t *testing.T) {
	t.Parallel()
	e := newEVMCashOut(t, 50_000_000)
	logs := &testkit.Logs{}
	ctx := observability.WithLogger(actorContext(t.Context()),
		observability.NewLogger(config.Config{Env: config.EnvTest}, logs))
	const needed = 200_000_000

	for range 2 {
		if err := e.cashOutIn(ctx, needed); err != nil {
			t.Fatal(err)
		}
	}

	want := []cashOutLeg{
		{InMint: aaplxMint, Status: string(domain.StatusConfirmed), InAmount: 1_000, Batch: 2},
		{InMint: e.token(), Status: string(domain.StatusConfirmed), InAmount: soldUnits, Batch: 2},
	}
	if legs := e.legs(t); !slices.Equal(legs, want) {
		t.Fatalf("legs = %+v, want every unit of both lots sold", legs)
	}
	alerts := logs.Lines(observability.TradingCashOutPlanShort.Name, "level", "job_id", "cabal_id")
	wantAlert := "ERROR " + e.job.String() + " " + e.cabal.String()
	amounts := strings.Count(string(logs.Bytes()), `"needed_micros":200000000,"planned_micros":89970000`)
	if len(alerts) != 1 || alerts[0] != wantAlert || amounts != 1 {
		t.Fatalf("plan_short alerts = %q, want one %q", alerts, wantAlert)
	}
}

func TestRouteExec_aVoteTimeQuoteFarAboveTheRequoteNeverRaisesTheFloor(t *testing.T) {
	t.Parallel()
	e := newRouteEnv(t)
	e.privy.script(multichain.ActionSucceeded)
	e.evm.fills = []app.EVMFill{{State: app.FillSucceeded, Out: filledOut}}
	req := e.request()
	req.QuoteOutAmount = requotedOut * 10

	view := e.mustBuy(t, req, domain.StatusConfirmed)

	e.wantEVMRow(t, view.ID.UUID(), swapHash)
	e.wantOneBridgeAndOneBuild(t, view.ID)
}

type bridgeBackWinsTheRace struct {
	app.Pauses
	t     *testing.T
	e     *routeEnv
	swap  uuid.UUID
	raced bool
}

func (r *bridgeBackWinsTheRace) IsPaused(ctx context.Context, cabal ids.CabalID) (funding.Pause, error) {
	if !r.raced {
		r.raced = true
		if _, err := r.e.pool.Exec(ctx, `INSERT INTO trade_steps
			(id, trade_id, step, network, privy_action_id, amount_micros, status, created_at, updated_at)
			VALUES ($1, $2, 'bridge_back', 'base', 'privy-back', 24992500, 'pending', $3, $3)`,
			r.e.ids.NewV7(), r.swap, r.e.clk.Now()); err != nil {
			r.t.Fatal(err)
		}
	}
	return r.Pauses.IsPaused(ctx, cabal)
}

func TestRetry_aBridgeBackThatTakesTheStrandedCashFirstFailsTheClaimAndRechecksFunds(t *testing.T) {
	t.Parallel()
	e := newRouteEnv(t)
	e.privy.script(multichain.ActionSucceeded)
	e.evm.fills = []app.EVMFill{{State: app.FillReverted}}
	cmd := e.routedBuy(t)
	first := e.swapsOf(t, cmd.ProposalID)[0]
	e.ledger.SetTokens(e.wallet.Address, usdcToken(), 0)
	ports := e.ports()
	ports.Pauses = &bridgeBackWinsTheRace{Pauses: ports.Pauses, t: t, e: e, swap: first}
	h := app.NewExecuteTradeHandler(app.ExecuteTradeDeps{
		Layer: e.layer(), Routes: e.exec(), Listings: e.listings, UoW: e.uow, Reads: e.reads, Venue: e.venue,
		Ports: ports, USDC: usdcToken(), EVM: true,
	})
	retry := cmd
	retry.Route, retry.Retry = nil, &app.Retry{Of: ids.SwapIDFrom(first), SlippageBps: 100}
	d := e.delivery(t, retry)

	err := h.Handle(actorContext(t.Context()), d, retry, nil)
	if !errs.Retryable(errs.CodeOf(err)) || len(e.swapsOf(t, cmd.ProposalID)) != 1 || len(e.privy.calls()) != 1 {
		t.Fatalf("Handle err = %v with %d swaps after %d transfers, want a retryable error, no retry swap, "+
			"and no second bridge", err, len(e.swapsOf(t, cmd.ProposalID)), len(e.privy.calls()))
	}
	if err := h.Handle(actorContext(t.Context()), d, retry, nil); err != nil {
		t.Fatal(err)
	}
	if code := e.deliveryCode(t, d); code != string(errs.CodeInsufficientFunds) || len(e.privy.calls()) != 1 {
		t.Fatalf("redelivery %s after %d transfers, want the Solana funds check to refuse it", code,
			len(e.privy.calls()))
	}
}

type recordedQuote struct {
	Body map[string]any `json:"body"`
}

func recordedSellQuote(t *testing.T, owner string, floor uint64) (string, string, string) {
	t.Helper()
	raw, err := os.ReadFile("../../testkit/fakes/uniswap/testdata/sell_aaplon_ethereum.json")
	if err != nil {
		t.Fatal(err)
	}
	var rec recordedQuote
	if err := json.Unmarshal(raw, &rec); err != nil {
		t.Fatal(err)
	}
	quote, _ := rec.Body["quote"].(map[string]any)
	input, _ := quote["input"].(map[string]any)
	output, _ := quote["output"].(map[string]any)
	quote["aggregatedOutputs"] = []map[string]any{{
		"token": output["token"], "amount": output["amount"], "recipient": owner,
		"minAmount": strconv.FormatUint(floor, 10),
	}}
	body, err := json.Marshal(rec.Body)
	if err != nil {
		t.Fatal(err)
	}
	in, _ := input["amount"].(string)
	out, _ := output["amount"].(string)
	return string(body), in, out
}

func TestEVMVenue_buildsARecordedSellWithAFloorFromTheQuoteAndTheSlippage(t *testing.T) {
	t.Parallel()
	const (
		owner    = "0x000000000000000000000000000000000000dead"
		aaplon   = "0x14c3abf95cb9c93a8b82c1cdcb76d72cb87b2d4c"
		ethUSDC  = "0xa0b86991c6218b36c1d19d4a2e9eb0ce3606eb48"
		ethRoute = "0x66a9893cc07d91d95644aedd05d03f95e1dba8af"
		floor    = 99_000_000
	)
	quote, in, out := recordedSellQuote(t, owner, floor)
	amountIn, err := strconv.ParseUint(in, 10, 64)
	if err != nil {
		t.Fatal(err)
	}
	e := newVenueEnv(t)
	e.script(approvalReply("null"), quote, quote, swapReply(evmTxJSON(ethRoute, "0x3593564c", "0x0")))
	spec := app.EVMSwapSpec{
		Network: chain.NetworkEthereum, Owner: evmAddress(t, owner), TokenIn: evmAddress(t, aaplon),
		TokenOut: evmAddress(t, ethUSDC), AmountIn: amountIn, SlippageBps: 100,
	}

	plan, err := e.venue.Build(t.Context(), spec)
	if err != nil {
		t.Fatal(err)
	}
	if strconv.FormatUint(plan.OutAmount, 10) != out || plan.MinOut != floor {
		t.Fatalf("plan out %d floor %d, want the recorded %s out and a 1%% floor", plan.OutAmount, plan.MinOut, out)
	}
	calls := e.uniswap.calls
	if calls[1].body["amount"] != in || calls[1].body["slippageTolerance"] != 0.0 ||
		calls[2].body["slippageTolerance"] != 0.99 || calls[2].body["tokenOut"] != ethUSDC {
		t.Fatalf("quotes = %+v, want the held units quoted, then requoted at the tolerance keeping the floor", calls)
	}
	floored := domain.SlippageOf(spec.SlippageBps).MinOut(plan.OutAmount)
	if floored != floor || plan.MinOut < floored {
		t.Fatalf("route floor %d against plan floor %d, want the plan to clear it", floored, plan.MinOut)
	}
}

func TestEVMVenue_refusesASellWhoseQuoteRaisesNothing(t *testing.T) {
	t.Parallel()
	e := newVenueEnv(t)
	e.script(approvalReply("null"), quoteReply("0", "0", "null"), "", "")
	spec := e.spec(t)
	spec.MinOut, spec.SlippageBps = 0, 100

	if _, err := e.venue.Build(t.Context(), spec); errs.CodeOf(err) != errs.CodePriceMoved {
		t.Fatalf("Build err = %v, want price_moved", err)
	}
	spec.SlippageBps = 0
	if _, err := e.venue.Build(t.Context(), spec); errs.CodeOf(err) != errs.CodeInvalidInput {
		t.Fatalf("Build without a floor or a slippage err = %v, want invalid_input", err)
	}
}
