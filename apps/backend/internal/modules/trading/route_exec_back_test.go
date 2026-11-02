package trading_test

import (
	"errors"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"golang.org/x/sync/errgroup"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	governance "github.com/monaco/monaco/apps/backend/internal/modules/governance/port"
	"github.com/monaco/monaco/apps/backend/internal/modules/trading/app"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/multichain"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

func (e *routeEnv) strand(t *testing.T) (app.ExecuteTrade, uuid.UUID) {
	t.Helper()
	e.privy.script(multichain.ActionSucceeded)
	e.evm.fills = []app.EVMFill{{State: app.FillReverted}}
	cmd := e.routedBuy(t)
	e.clk.Advance(app.SwapSweepAge + time.Second)
	return cmd, e.swapsOf(t, cmd.ProposalID)[0]
}

func (e *routeEnv) mustTick(t *testing.T, scanned, changed int) {
	t.Helper()
	if report, err := e.tick(t.Context(), t); err != nil || report.Scanned != scanned || report.Changed != changed {
		t.Fatalf("Tick = %+v, %v, want %d scanned and %d changed", report, err, scanned, changed)
	}
}

func TestBridgeBack_strandedCashOfAProposalNoLongerRetryableReturnsToSolana(t *testing.T) {
	t.Parallel()
	e := newRouteEnv(t)
	_, swap := e.strand(t)
	e.mustTick(t, 1, 0)
	e.proposals.set(governance.Status("cancelled"))
	e.privy.script(multichain.ActionSucceeded)

	e.mustTick(t, 1, 1)
	e.mustTick(t, 0, 0)

	calls := e.privy.calls()
	want := multichain.TransferSpec{
		From: chain.NetworkBase, To: chain.NetworkSolana, Destination: string(e.wallet.Address),
		AmountMicros: money.MicrosFromUint64(landedMicros),
	}
	if len(calls) != 2 || calls[1].spec != want || calls[1].wallet != "evm-wallet" ||
		calls[1].key != "trade:"+swap.String()+":bridge_back" {
		t.Fatalf("transfers = %+v, want the landed cash bridged back from the EVM treasury", calls)
	}
	got := e.steps(t, swap)
	wantSteps(t, got[len(got)-3:], "bridge_back:pending:0", "bridge_back:submitted:7500", "bridge_back:succeeded:7500")
}

func TestBridgeBack_aStartedReturnIsResumedEvenIfTheProposalIsRetryableAgain(t *testing.T) {
	t.Parallel()
	e := newRouteEnv(t)
	_, swap := e.strand(t)
	e.proposals.set(governance.Status("cancelled"))
	e.mustTick(t, 1, 1)
	e.proposals.set(governance.StatusPassed)
	e.privy.script(multichain.ActionSucceeded)

	e.mustTick(t, 1, 1)

	if status, action := e.bridgeBackOf(t, swap); status != "succeeded" || action != "privy-action-2" {
		t.Fatalf("bridge_back %s %s, want succeeded on its one action", status, action)
	}
}

func TestBridgeBack_cashARetryReusedIsNeverBridgedBack(t *testing.T) {
	t.Parallel()
	e := newRouteEnv(t)
	cmd, first := e.strand(t)
	e.requote()
	e.evm.fills = []app.EVMFill{{State: app.FillReverted}}
	if code := e.retry(t, cmd, first); code != string(errs.CodeSwapFailed) {
		t.Fatalf("retry delivery %s, want swap_failed", code)
	}
	e.proposals.set(governance.Status("cancelled"))
	e.clk.Advance(app.SwapSweepAge + time.Second)

	e.mustTick(t, 1, 1)
	if err := e.exec().BridgeBack(actorContext(t.Context()), ids.SwapIDFrom(first)); err != nil {
		t.Fatal(err)
	}

	backs := e.count(t, `SELECT count(*) FROM trade_steps WHERE step = 'bridge_back'`)
	if backs != 1 || len(e.privy.calls()) != 2 {
		t.Fatalf(
			"%d bridge_back steps after %d transfers, want the retry's cash returned once",
			backs,
			len(e.privy.calls()),
		)
	}
	if status, _ := e.bridgeBackOf(t, first); status != "" {
		t.Fatalf("the reused swap has a bridge_back step %s, want none", status)
	}
}

func TestBridgeBack_refusesWhatItCannotLoadOrJournal(t *testing.T) {
	t.Parallel()
	e := newRouteEnv(t)
	if err := e.exec().BridgeBack(actorContext(t.Context()), ids.SwapIDFrom(e.ids.NewV7())); err == nil {
		t.Fatal("BridgeBack of an unknown swap succeeded")
	}
	e.landings.missing = 1
	e.privy.script(multichain.ActionSucceeded)
	unseen := e.request()
	if _, err := e.buy(t.Context(), unseen); errs.CodeOf(err) != errs.CodeUpstreamUnavailable {
		t.Fatalf("Buy whose bridge out is not on chain yet err = %v, want upstream_unavailable", err)
	}
	stuck := e.swapIDs(t, unseen.Source.ID)[0]
	if err := e.exec().BridgeBack(actorContext(t.Context()), ids.SwapIDFrom(stuck)); err != nil {
		t.Fatalf("BridgeBack of a bridge out not seen yet err = %v, want nothing to do", err)
	}
	if status, _ := e.bridgeBackOf(t, stuck); status != "" || len(e.privy.calls()) != 1 {
		t.Fatalf("bridge back %q after %d transfers, want none before the arrival", status, len(e.privy.calls()))
	}
	e.route.Ranked[0].Costs = []multichain.Cost{
		{Kind: multichain.CostBridge, Micros: money.MicrosFromUint64(buyMicros + 1)},
	}
	big := e.request()
	if _, err := e.buy(t.Context(), big); err == nil {
		t.Fatal("a buy with a fee above the trade succeeded")
	}
	if err := e.exec().
		BridgeBack(actorContext(t.Context()), ids.SwapIDFrom(e.swapIDs(t, big.Source.ID)[0])); err == nil {
		t.Fatal("BridgeBack with a fee above the trade succeeded")
	}
}

func TestEVMTrades_reportsARetryableReadError(t *testing.T) {
	t.Parallel()
	e := newRouteEnv(t)
	e.strand(t)
	e.proposals.FailOnce("Retryable", errs.New(errs.CodeUpstreamUnavailable, "governance.Retryable"))

	if report, err := e.tick(t.Context(), t); errs.CodeOf(err) != errs.CodeUpstreamUnavailable || report.Changed != 0 {
		t.Fatalf("Tick = %+v, %v, want upstream_unavailable", report, err)
	}
}

func (e *routeEnv) bridgeBackOf(t *testing.T, swap uuid.UUID) (status, action string) {
	t.Helper()
	_ = e.pool.QueryRow(t.Context(), `SELECT status, coalesce(privy_action_id, '') FROM trade_steps
		WHERE trade_id = $1 AND step = 'bridge_back'`, swap).Scan(&status, &action)
	return status, action
}

func (e *routeEnv) forgetArrival(t *testing.T, swap uuid.UUID) {
	t.Helper()
	e.mustExec(t, `UPDATE trade_steps SET arrived_micros = NULL, arrival_tx = NULL
		WHERE trade_id = $1 AND step = 'bridge_out'`, swap)
}

func (e *routeEnv) inboundCalls() int {
	e.landings.mu.Lock()
	defer e.landings.mu.Unlock()
	return len(e.landings.seen)
}

func (e *routeEnv) wantBridgedBack(t *testing.T, before int, want money.Micros) {
	t.Helper()
	calls := e.privy.calls()
	if len(calls) != before+1 || calls[before].spec.AmountMicros != want ||
		calls[before].spec.To != chain.NetworkSolana {
		t.Fatalf("transfers = %+v, want one more sending %s to Solana", calls, want)
	}
}

func TestBridgeBack_aQueuedReturnSendsItsOwnAmountWithoutObservingTheOldBridgeOut(t *testing.T) {
	t.Parallel()
	e := newRouteEnv(t)
	_, swap := e.strand(t)
	e.forgetArrival(t, swap)
	queued := money.MicrosFromUint64(900_000)
	if _, err := e.returnCash(t, &cashFake{balance: queued}, swap); err != nil {
		t.Fatal(err)
	}
	scans, sent := e.inboundCalls(), len(e.privy.calls())
	e.privy.script(multichain.ActionSucceeded)

	e.mustTick(t, 1, 1)

	e.wantBridgedBack(t, sent, queued)
	if got := e.inboundCalls(); got != scans {
		t.Fatalf("EVMArrivals.Inbound called %d times for the return, want none", got-scans)
	}
	if status, _ := e.bridgeBackOf(t, swap); status != "succeeded" {
		t.Fatalf("bridge_back %q, want succeeded", status)
	}
}

func TestBridgeBack_aQueuedReturnOnAReusedBridgeOutSendsItsOwnAmount(t *testing.T) {
	t.Parallel()
	e := newRouteEnv(t)
	cmd, first := e.strand(t)
	e.requote()
	e.evm.fills = []app.EVMFill{{State: app.FillReverted}}
	if code := e.retry(t, cmd, first); code != string(errs.CodeSwapFailed) {
		t.Fatalf("retry delivery %s, want swap_failed", code)
	}
	holder := e.swapsOf(t, cmd.ProposalID)[1]
	e.forgetArrival(t, holder)
	queued := money.MicrosFromUint64(972_507)
	if _, err := e.returnCash(t, &cashFake{balance: queued}, first); err != nil {
		t.Fatal(err)
	}
	var reused string
	if err := e.pool.QueryRow(t.Context(), `SELECT privy_action_id FROM trade_steps
		WHERE trade_id = $1 AND step = 'bridge_out'`, holder).Scan(&reused); err != nil || reused != "reused:"+first.String() {
		t.Fatalf("holder bridge_out %q, %v, want reused from the first swap", reused, err)
	}
	scans, sent := e.inboundCalls(), len(e.privy.calls())
	e.privy.script(multichain.ActionSucceeded)

	if err := e.exec().BridgeBack(actorContext(t.Context()), ids.SwapIDFrom(holder)); err != nil {
		t.Fatal(err)
	}

	e.wantBridgedBack(t, sent, queued)
	if got := e.inboundCalls(); got != scans {
		t.Fatalf("EVMArrivals.Inbound called %d times for the return, want none", got-scans)
	}
}

func TestBridgeBack_anUnrecordedArrivalReturnsTheLessOfTheEstimateAndTheWalletBalance(t *testing.T) {
	t.Parallel()
	for name, c := range map[string]struct{ held, want money.Micros }{
		"balance below the estimate": {money.MicrosFromUint64(900_000), money.MicrosFromUint64(900_000)},
		"balance above the estimate": {money.MicrosFromUint64(landedMicros + 5), landedCash()},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			e := newRouteEnv(t)
			_, swap := e.strand(t)
			e.forgetArrival(t, swap)
			e.cash.balance = c.held
			scans, sent := e.inboundCalls(), len(e.privy.calls())
			e.privy.script(multichain.ActionSucceeded)

			if err := e.exec().BridgeBack(actorContext(t.Context()), ids.SwapIDFrom(swap)); err != nil {
				t.Fatal(err)
			}

			e.wantBridgedBack(t, sent, c.want)
			if _, amount := e.returnStepAmount(t, swap); amount != c.want.String() {
				t.Fatalf("bridge_back amount %s, want %s", amount, c.want)
			}
			if got := e.inboundCalls(); got != scans {
				t.Fatalf("EVMArrivals.Inbound called %d times, want none", got-scans)
			}
			if e.cash.network != chain.NetworkBase || e.cash.wallet != e.treasury.wallet.Address {
				t.Fatalf(
					"balance read for %s on %s, want the treasury EVM wallet on base",
					e.cash.wallet,
					e.cash.network,
				)
			}
		})
	}
}

func TestBridgeBack_anEmptyWalletAddsNoStepAndAnUnreadableOneRetries(t *testing.T) {
	t.Parallel()
	e := newRouteEnv(t)
	_, swap := e.strand(t)
	e.forgetArrival(t, swap)
	sent := len(e.privy.calls())
	e.cash.err = errs.New(errs.CodeUpstreamUnavailable, "test.cash")

	if err := e.exec().
		BridgeBack(actorContext(t.Context()), ids.SwapIDFrom(swap)); errs.CodeOf(
		err,
	) != errs.CodeUpstreamUnavailable {
		t.Fatalf("BridgeBack with an unreadable balance err = %v, want upstream_unavailable", err)
	}
	e.cash.err, e.cash.balance = nil, money.Micros{}
	if err := e.exec().BridgeBack(actorContext(t.Context()), ids.SwapIDFrom(swap)); err != nil {
		t.Fatal(err)
	}

	if status, _ := e.bridgeBackOf(t, swap); status != "" || len(e.privy.calls()) != sent {
		t.Fatalf(
			"bridge_back %q after %d more transfers, want none for an empty wallet",
			status,
			len(e.privy.calls())-sent,
		)
	}
}

func TestBridgeBack_aRecordedArrivalStillSetsTheLandedCash(t *testing.T) {
	t.Parallel()
	e := newRouteEnv(t)
	_, swap := e.strand(t)
	e.cash.balance = money.MicrosFromUint64(5)
	sent := len(e.privy.calls())
	e.privy.script(multichain.ActionSucceeded)

	if err := e.exec().BridgeBack(actorContext(t.Context()), ids.SwapIDFrom(swap)); err != nil {
		t.Fatal(err)
	}

	e.wantBridgedBack(t, sent, landedCash())
	if e.cash.reads != 0 {
		t.Fatalf("balance read %d times, want none when the arrival is recorded", e.cash.reads)
	}
}

func TestBridgeBack_aReturnWhoseSolanaLegIsExternalSettlesOnWhatPrivyDelivered(t *testing.T) {
	t.Parallel()
	e := newRouteEnv(t)
	_, swap := e.strand(t)
	e.mustTick(t, 1, 0)
	e.proposals.set(governance.Status("cancelled"))
	e.privy.delivered = landedMicros - 21_282
	e.privy.script(multichain.ActionPending)

	e.mustTick(t, 1, 1)
	if status, _ := e.bridgeBackOf(t, swap); status != "submitted" {
		t.Fatalf("bridge_back %q while Privy is pending, want submitted", status)
	}

	e.privy.script(multichain.ActionSucceeded)
	e.mustTick(t, 1, 1)

	got := e.steps(t, swap)
	wantSteps(t, got[len(got)-3:], "bridge_back:pending:0", "bridge_back:submitted:7500", "bridge_back:succeeded:21282")
}

func TestBridgeBack_aReturnWhoseDeliveredAmountIsBelowTheQuotedFloorBooksNothing(t *testing.T) {
	t.Parallel()
	e := newRouteEnv(t)
	_, swap := e.strand(t)
	e.proposals.set(governance.Status("cancelled"))
	e.privy.delivered = landedMicros - 1_000_000
	e.privy.script(multichain.ActionSucceeded)

	err := e.exec().BridgeBack(actorContext(t.Context()), ids.SwapIDFrom(swap))

	if errs.CodeOf(err) != errs.CodeUpstreamUnavailable {
		t.Fatalf("BridgeBack err = %v, want upstream_unavailable", err)
	}
	if status, _ := e.bridgeBackOf(t, swap); status != "submitted" {
		t.Fatalf("bridge_back %q, want submitted with nothing booked", status)
	}
	e.privy.delivered = landedMicros - 100_000
	if err := e.exec().BridgeBack(actorContext(t.Context()), ids.SwapIDFrom(swap)); err != nil {
		t.Fatalf("BridgeBack with a delivery inside the bound err = %v", err)
	}
	got := e.steps(t, swap)
	wantSteps(t, got[len(got)-1:], "bridge_back:succeeded:100000")

	e.privy.delivered = landedMicros - 1_000_000
	stranded, strandedErr := e.exec().Stranded(actorContext(t.Context()), ids.SwapIDFrom(swap))
	if errs.CodeOf(strandedErr) != errs.CodeUpstreamUnavailable {
		t.Fatalf("Stranded = %+v, %v, want upstream_unavailable", stranded, strandedErr)
	}
}

func (e *routeEnv) queries() app.Queries { return app.NewQueries(e.pool) }

func TestBridgeBack_theFillIsOwnedOnceTheReturnSucceedsAndExpectedWhileItIsInFlight(t *testing.T) {
	t.Parallel()
	e := newRouteEnv(t)
	_, swap := e.strand(t)
	e.mustTick(t, 1, 0)
	e.proposals.set(governance.Status("cancelled"))
	e.privy.script(multichain.ActionPending)

	e.mustTick(t, 1, 1)

	bands, err := e.queries().BridgeBacksInFlight(t.Context(), e.cabal)
	sent := money.MicrosFromUint64(landedMicros)
	if err != nil || len(bands) != 1 || bands[0].Max != sent || bands[0].Min.Cmp(sent) >= 0 || bands[0].Min.IsZero() {
		t.Fatalf("bands = %+v, %v, want one band ending at %d while the return is submitted", bands, err, landedMicros)
	}
	_, action := e.bridgeBackOf(t, swap)
	fill := chain.Signature(arrivalPrefix + action)
	if owned, err := e.queries().OwnsSignature(t.Context(), fill); err != nil || owned {
		t.Fatalf("OwnsSignature(fill) = %t, %v, want false before the return succeeds", owned, err)
	}

	e.privy.script(multichain.ActionSucceeded)
	e.mustTick(t, 1, 1)

	if owned, err := e.queries().OwnsSignature(t.Context(), fill); err != nil || !owned {
		t.Fatalf("OwnsSignature(fill) = %t, %v, want true once the return succeeded", owned, err)
	}
	if bands, err := e.queries().BridgeBacksInFlight(t.Context(), e.cabal); err != nil || len(bands) != 0 {
		t.Fatalf("bands = %+v, %v, want none once the return succeeded", bands, err)
	}
}

func TestBridgeBack_aClaimIsKeptInTheDatabaseAndOnlyOneSignatureHoldsIt(t *testing.T) {
	t.Parallel()
	e := newRouteEnv(t)
	_, _ = e.strand(t)
	e.mustTick(t, 1, 0)
	e.proposals.set(governance.Status("cancelled"))
	e.privy.script(multichain.ActionPending)
	e.mustTick(t, 1, 1)
	bands, err := e.queries().BridgeBacksInFlight(t.Context(), e.cabal)
	if err != nil || len(bands) != 1 || bands[0].Claimed != "" {
		t.Fatalf("bands = %+v, %v, want one unclaimed band", bands, err)
	}
	band := bands[0].ID

	first, firstErr := e.queries().ClaimArrival(t.Context(), band, "first")
	again, againErr := app.NewQueries(e.pool).ClaimArrival(t.Context(), band, "first")
	second, secondErr := app.NewQueries(e.pool).ClaimArrival(t.Context(), band, "second")

	if err := errors.Join(firstErr, againErr, secondErr); err != nil || !first || !again || second {
		t.Fatalf("claims = %t %t %t, %v, want first and again to hold it and second refused", first, again, second, err)
	}
	bands, err = app.NewQueries(e.pool).BridgeBacksInFlight(t.Context(), e.cabal)
	if err != nil || len(bands) != 1 || bands[0].Claimed != "first" {
		t.Fatalf("bands = %+v, %v, want the band claimed by first for a fresh reader", bands, err)
	}
}

func TestBridgeBack_twoClaimersOnOneBandCannotBothWin(t *testing.T) {
	t.Parallel()
	e := newRouteEnv(t)
	_, _ = e.strand(t)
	e.mustTick(t, 1, 0)
	e.proposals.set(governance.Status("cancelled"))
	e.privy.script(multichain.ActionPending)
	e.mustTick(t, 1, 1)
	bands, err := e.queries().BridgeBacksInFlight(t.Context(), e.cabal)
	if err != nil || len(bands) != 1 {
		t.Fatalf("bands = %+v, %v, want one", bands, err)
	}
	var wins atomic.Int32
	var group errgroup.Group
	for i := range 8 {
		sig := chain.Signature(fmt.Sprintf("sig-%d", i))
		group.Go(func() error {
			won, err := app.NewQueries(e.pool).ClaimArrival(t.Context(), bands[0].ID, sig)
			if won {
				wins.Add(1)
			}
			return err
		})
	}

	if err := group.Wait(); err != nil || wins.Load() != 1 {
		t.Fatalf("wins = %d, %v, want exactly one signature to take the band", wins.Load(), err)
	}
}

func TestBridgeBack_aRouteTheBandCannotReadFailsInsteadOfGuessing(t *testing.T) {
	t.Parallel()
	e := newRouteEnv(t)
	_, swap := e.strand(t)
	e.mustTick(t, 1, 0)
	e.proposals.set(governance.Status("cancelled"))
	e.privy.script(multichain.ActionPending)
	e.mustTick(t, 1, 1)
	if _, err := e.pool.Exec(
		t.Context(),
		`UPDATE trade_routes SET comparison = '"unreadable"' WHERE trade_id = $1`,
		swap,
	); err != nil {
		t.Fatal(err)
	}

	bands, err := e.queries().BridgeBacksInFlight(t.Context(), e.cabal)

	if errs.CodeOf(err) != errs.CodeDecodeFailed || len(bands) != 0 {
		t.Fatalf("bands = %+v, err = %v, want a decode failure and no bands", bands, err)
	}
}
