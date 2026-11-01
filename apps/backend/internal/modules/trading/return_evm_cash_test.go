package trading_test

import (
	"context"
	"slices"
	"strconv"
	"testing"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/trading/app"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/multichain"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

type cashFake struct {
	balance money.Micros
	err     error
	before  func()
	reads   int
	wallet  chain.EVMAddress
	network chain.Network
}

func (c *cashFake) Balance(_ context.Context, network chain.Network, owner chain.EVMAddress) (money.Micros, error) {
	c.reads++
	c.wallet, c.network = owner, network
	if c.before != nil {
		c.before()
	}
	return c.balance, c.err
}

type writeOffsFake struct {
	offs []app.EVMCashWriteOff
	err  error
}

func (w *writeOffsFake) WriteOff(_ context.Context, _ db.Tx, off app.EVMCashWriteOff) error {
	w.offs = append(w.offs, off)
	return w.err
}

func (e *routeEnv) returner(cash app.EVMCash) *app.ReturnEVMCashHandler {
	return e.returnerWith(cash, &writeOffsFake{})
}

func (e *routeEnv) returnerWith(cash app.EVMCash, offs app.EVMCashWriteOffs) *app.ReturnEVMCashHandler {
	return app.NewReturnEVMCashHandler(e.uow, e.pool, e.clk, e.ids, e.treasury, cash, offs)
}

func (e *routeEnv) returnCash(t *testing.T, cash app.EVMCash, swap uuid.UUID) (app.EVMCashReturn, error) {
	t.Helper()
	return e.returner(cash).Handle(actorContext(t.Context()), app.ReturnEVMCash{
		SwapID: ids.SwapIDFrom(swap), Actor: "operator:alice", Reason: "stranded since 2026-10-12",
	})
}

func (e *routeEnv) returnStepAmount(t *testing.T, swap uuid.UUID) (status, amount string) {
	t.Helper()
	_ = e.pool.QueryRow(t.Context(), `SELECT status, amount_micros::text FROM trade_steps
		WHERE trade_id = $1 AND step = 'bridge_back'`, swap).Scan(&status, &amount)
	return status, amount
}

func (e *routeEnv) returnActions(t *testing.T, swap uuid.UUID) int {
	t.Helper()
	return e.count(
		t,
		`SELECT count(*) FROM swap_operator_actions WHERE swap_id = $1 AND action = 'return_evm_cash'`,
		swap,
	)
}

func (e *routeEnv) wantRefused(t *testing.T, err error, code errs.Code, reason string) {
	t.Helper()
	var got string
	for _, a := range errs.Detail(err) {
		if a.Key == "reason" {
			got = a.Value.String()
		}
	}
	if errs.CodeOf(err) != code || got != reason {
		t.Fatalf("err = %v (reason %q), want %s for %s", err, got, code, reason)
	}
	if backs := e.count(t, `SELECT count(*) FROM trade_steps WHERE step = 'bridge_back'`); backs != 0 {
		t.Fatalf("%d bridge_back steps after a refusal, want none", backs)
	}
}

func landedCash() money.Micros { return money.MicrosFromUint64(landedMicros) }

func TestReturnEVMCash_queuesTheLandedCashWithoutSendingIt(t *testing.T) {
	t.Parallel()
	e := newRouteEnv(t)
	_, swap := e.strand(t)
	cash := &cashFake{balance: landedCash()}

	got, err := e.returnCash(t, cash, swap)

	want := app.EVMCashReturn{
		Requested: ids.SwapIDFrom(swap), Holder: ids.SwapIDFrom(swap), CabalID: e.cabal, Network: chain.NetworkBase,
		Landed: landedCash(), OnChain: landedCash(), Amount: landedCash(),
	}
	if err != nil || got != want {
		t.Fatalf("return = %+v, %v, want %+v", got, err, want)
	}
	if cash.wallet != e.treasury.wallet.Address || cash.network != chain.NetworkBase || len(e.privy.calls()) != 1 {
		t.Fatalf("balance read for %s on %s after %d transfers, want the treasury wallet and no send",
			cash.wallet, cash.network, len(e.privy.calls()))
	}
	if status, amount := e.returnStepAmount(t, swap); status != "pending" || amount != strconv.Itoa(landedMicros) {
		t.Fatalf("bridge_back %s %s, want pending for the landed cash", status, amount)
	}
	if actions := e.returnActions(t, swap); actions != 1 {
		t.Fatalf("%d return_evm_cash operator actions, want 1", actions)
	}
}

func TestReturnEVMCash_thePollerSendsAQueuedReturnOnce(t *testing.T) {
	t.Parallel()
	e := newRouteEnv(t)
	_, swap := e.strand(t)
	if _, err := e.returnCash(t, &cashFake{balance: landedCash()}, swap); err != nil {
		t.Fatal(err)
	}

	e.privy.script(multichain.ActionSucceeded)
	e.mustTick(t, 1, 1)
	e.mustTick(t, 0, 0)

	calls := e.privy.calls()
	if len(calls) != 2 || calls[1].spec.AmountMicros != landedCash() ||
		calls[1].key != "trade:"+swap.String()+":bridge_back" {
		t.Fatalf("transfers = %+v, want the queued amount sent once under the bridge_back key", calls)
	}
	if status, _ := e.bridgeBackOf(t, swap); status != "succeeded" {
		t.Fatalf("bridge_back %q, want succeeded", status)
	}
}

func TestReturnEVMCash_aSwapWhoseCashWasReusedActsOnTheHolder(t *testing.T) {
	t.Parallel()
	e := newRouteEnv(t)
	cmd, first := e.strand(t)
	e.requote()
	e.evm.fills = []app.EVMFill{{State: app.FillReverted}}
	if code := e.retry(t, cmd, first); code != string(errs.CodeSwapFailed) {
		t.Fatalf("retry delivery %s, want swap_failed", code)
	}
	holder := e.swapsOf(t, cmd.ProposalID)[1]

	got, err := e.returnCash(t, &cashFake{balance: landedCash()}, first)

	if err != nil || got.Holder.UUID() != holder || got.Requested.UUID() != first {
		t.Fatalf("return = %+v, %v, want the holder %s for the requested %s", got, err, holder, first)
	}
	if status, _ := e.bridgeBackOf(t, first); status != "" {
		t.Fatalf("the swap that gave its cash away has bridge_back %q, want none", status)
	}
	if status, _ := e.returnStepAmount(t, holder); status != "pending" {
		t.Fatalf("holder bridge_back %q, want pending", status)
	}
	var detail string
	if err := e.pool.QueryRow(t.Context(), `SELECT detail->>'requested_swap_id' FROM swap_operator_actions
		WHERE swap_id = $1 AND action = 'return_evm_cash'`, holder).Scan(&detail); err != nil || detail != first.String() {
		t.Fatalf("operator action requested %q, %v, want %s", detail, err, first)
	}
}

func TestReturnEVMCash_neverSendsMoreThanTheWalletHolds(t *testing.T) {
	t.Parallel()
	e := newRouteEnv(t)
	_, swap := e.strand(t)
	held := money.MicrosFromUint64(landedMicros - 10_000)

	got, err := e.returnCash(t, &cashFake{balance: held}, swap)

	if err != nil || got.Amount != held || got.Landed != landedCash() || got.OnChain != held {
		t.Fatalf("return = %+v, %v, want the amount capped at the on-chain balance", got, err)
	}
	if _, amount := e.returnStepAmount(t, swap); amount != held.String() {
		t.Fatalf("bridge_back amount %s, want %s", amount, held)
	}
}

func TestReturnEVMCash_refusesAnEmptyWalletAndABalanceItCannotRead(t *testing.T) {
	t.Parallel()
	e := newRouteEnv(t)
	_, swap := e.strand(t)

	_, err := e.returnCash(t, &cashFake{}, swap)
	e.wantRefused(t, err, errs.CodeInsufficientFunds, "evm_balance_zero")

	_, err = e.returnCash(t, &cashFake{err: errs.New(errs.CodeRPCUnavailable, "test")}, swap)
	if errs.CodeOf(err) != errs.CodeRPCUnavailable {
		t.Fatalf("err = %v, want rpc_unavailable", err)
	}
	e.treasury.err = errs.New(errs.CodeInternal, "test")
	if _, err = e.returnCash(t, &cashFake{balance: landedCash()}, swap); errs.CodeOf(err) != errs.CodeInternal {
		t.Fatalf("err = %v, want the wallet lookup failure", err)
	}
}

func TestReturnEVMCash_aSecondRunRefusesAndAddsNoStep(t *testing.T) {
	t.Parallel()
	e := newRouteEnv(t)
	_, swap := e.strand(t)
	cash := &cashFake{balance: landedCash()}
	if _, err := e.returnCash(t, cash, swap); err != nil {
		t.Fatal(err)
	}

	_, err := e.returnCash(t, cash, swap)

	if errs.CodeOf(err) != errs.CodeStrandedCashTaken {
		t.Fatalf("second run err = %v, want stranded_cash_taken", err)
	}
	if steps, actions := e.count(t, `SELECT count(*) FROM trade_steps WHERE step = 'bridge_back'`),
		e.count(
			t,
			`SELECT count(*) FROM swap_operator_actions WHERE action = 'return_evm_cash'`,
		); steps != 1 || actions != 1 {
		t.Fatalf("%d steps and %d operator actions, want one of each", steps, actions)
	}
}

func TestReturnEVMCash_refusesACashARetryAlreadySpent(t *testing.T) {
	t.Parallel()
	e := newRouteEnv(t)
	cmd, first := e.strand(t)
	e.requote()
	e.privy.script(multichain.ActionSucceeded)
	e.evm.fills = []app.EVMFill{{State: app.FillSucceeded, Out: filledOut}}
	if code := e.retry(t, cmd, first); code != "ok" {
		t.Fatalf("retry delivery %q, want ok", code)
	}

	_, err := e.returnCash(t, &cashFake{balance: landedCash()}, first)

	e.wantRefused(t, err, errs.CodeStrandedCashTaken, "cash_spent")
}

func TestReturnEVMCash_refusesWhatIsNotAFailedBuyWithLandedCash(t *testing.T) {
	t.Parallel()
	cases := map[string]struct {
		prepare func(t *testing.T, e *routeEnv, swap uuid.UUID)
		code    errs.Code
		reason  string
	}{
		"a swap step still in flight": {func(t *testing.T, e *routeEnv, swap uuid.UUID) {
			t.Helper()
			e.mustExec(
				t,
				`UPDATE trade_steps SET status = 'submitted', error = NULL WHERE trade_id = $1 AND step = 'swap'`,
				swap,
			)
		}, errs.CodeStrandedCashTaken, "cash_spent"},
		"a swap still running": {func(t *testing.T, e *routeEnv, swap uuid.UUID) {
			t.Helper()
			e.mustExec(t, `UPDATE swaps SET status = 'submitted' WHERE id = $1`, swap)
		}, errs.CodeSwapNotStuck, "not_failed"},
		"a parked swap": {func(t *testing.T, e *routeEnv, swap uuid.UUID) {
			t.Helper()
			e.mustExec(
				t,
				`UPDATE swaps SET status = 'created', parked_at = now(), parked_code = 'x' WHERE id = $1`,
				swap,
			)
		}, errs.CodeSwapNotStuck, "parked_use_unpark_or_fail_parked"},
		"a bridge_out that did not land": {func(t *testing.T, e *routeEnv, swap uuid.UUID) {
			t.Helper()
			e.mustExec(
				t,
				`UPDATE trade_steps SET status = 'failed', error = 'x' WHERE trade_id = $1 AND step = 'bridge_out'`,
				swap,
			)
		}, errs.CodeSwapNotStuck, "no_landed_bridge_out"},
		"an arrival and a swap amount nobody recorded": {func(t *testing.T, e *routeEnv, swap uuid.UUID) {
			t.Helper()
			e.mustExec(t, `UPDATE trade_steps SET arrived_micros = NULL, arrival_tx = NULL WHERE trade_id = $1`, swap)
			e.mustExec(t, `DELETE FROM trade_steps WHERE trade_id = $1 AND step = 'swap'`, swap)
		}, errs.CodeSwapNotStuck, "arrival_not_observed"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			e := newRouteEnv(t)
			_, swap := e.strand(t)
			tc.prepare(t, e, swap)

			_, err := e.returnCash(t, &cashFake{balance: landedCash()}, swap)

			e.wantRefused(t, err, tc.code, tc.reason)
		})
	}
}

func TestReturnEVMCash_refusesAnUnknownSwapAndABadRequest(t *testing.T) {
	t.Parallel()
	e := newRouteEnv(t)

	if _, err := e.returnCash(t, &cashFake{}, e.ids.NewV7()); errs.CodeOf(err) != errs.CodeSwapNotFound {
		t.Fatalf("unknown swap err = %v, want swap_not_found", err)
	}
	bad := app.ReturnEVMCash{SwapID: ids.SwapIDFrom(e.ids.NewV7()), Actor: "operator:alice"}
	if _, err := e.returner(&cashFake{}).Handle(t.Context(), bad); errs.CodeOf(err) != errs.CodeInvalidInput {
		t.Fatalf("no reason err = %v, want invalid_input", err)
	}
	if _, err := e.returner(&cashFake{}).Plan(t.Context(), ids.SwapIDFrom(e.ids.NewV7())); err == nil {
		t.Fatal("Plan of an unknown swap succeeded")
	}
}

func TestReturnEVMCash_aPollerThatTakesTheCashFirstFailsTheClaim(t *testing.T) {
	t.Parallel()
	e := newRouteEnv(t)
	_, swap := e.strand(t)
	cash := &cashFake{balance: landedCash()}
	cash.before = func() {
		e.mustExec(t, `INSERT INTO trade_steps (id, trade_id, step, network, privy_action_id, amount_micros, status,
			created_at, updated_at) VALUES ($1, $2, 'bridge_back', 'base', 'privy-back', 24992500, 'pending', now(), now())`,
			e.ids.NewV7(), swap)
	}

	_, err := e.returnCash(t, cash, swap)

	if errs.CodeOf(err) != errs.CodeStrandedCashTaken {
		t.Fatalf("err = %v, want stranded_cash_taken", err)
	}
	if actions := e.returnActions(t, swap); actions != 0 {
		t.Fatalf("%d operator actions after a lost claim, want none", actions)
	}
}

func TestReturnEVMCash_refusesAnUnreadableSwap(t *testing.T) {
	t.Parallel()
	e := newRouteEnv(t)
	_, swap := e.strand(t)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	if _, err := e.returner(&cashFake{}).Plan(ctx, ids.SwapIDFrom(swap)); errs.CodeOf(err) != errs.CodeInternal {
		t.Fatalf("Plan on a cancelled context err = %v, want internal", err)
	}
}

func TestReturnEVMCash_aBridgeOutWithoutAnObservedArrivalLandsTheSwapStepsAmount(t *testing.T) {
	t.Parallel()
	e := newRouteEnv(t)
	_, swap := e.strand(t)
	e.mustExec(t, `UPDATE trade_steps SET arrived_micros = NULL, arrival_tx = NULL WHERE trade_id = $1`, swap)
	e.mustExec(t, `UPDATE trade_steps SET amount_micros = 974070 WHERE trade_id = $1 AND step = 'swap'`, swap)

	got, err := e.returnCash(t, &cashFake{balance: money.MicrosFromUint64(2_000_000)}, swap)

	if err != nil || got.Landed != money.MicrosFromUint64(974_070) || got.Amount != got.Landed {
		t.Fatalf("return = %+v, %v, want the swap step's 974070 as the landed cash", got, err)
	}
	short := money.MicrosFromUint64(900_000)
	e.mustExec(t, `DELETE FROM trade_steps WHERE trade_id = $1 AND step = 'bridge_back'`, swap)
	if got, err = e.returnCash(t, &cashFake{balance: short}, swap); err != nil || got.Amount != short {
		t.Fatalf("return = %+v, %v, want the amount capped at the 900000 on chain", got, err)
	}
}

func TestReturnEVMCash_aFullyCoveredReturnWritesNothingOff(t *testing.T) {
	t.Parallel()
	e := newRouteEnv(t)
	_, swap := e.strand(t)
	offs := &writeOffsFake{}

	got, err := e.returnerWith(&cashFake{balance: landedCash()}, offs).Handle(actorContext(t.Context()),
		app.ReturnEVMCash{SwapID: ids.SwapIDFrom(swap), Actor: "operator:alice", Reason: "stranded"})

	if err != nil || !got.Uncovered.IsZero() || len(offs.offs) != 0 {
		t.Fatalf("return = %+v, %v with write-offs %+v, want no uncovered cash and no write-off", got, err, offs.offs)
	}
}

func TestReturnEVMCash_theCashTheWalletLacksIsWrittenOffWithTheReturn(t *testing.T) {
	t.Parallel()
	e := newRouteEnv(t)
	_, swap := e.strand(t)
	held := money.MicrosFromUint64(landedMicros - 3_000_000)
	offs := &writeOffsFake{}

	got, err := e.returnerWith(&cashFake{balance: held}, offs).Handle(actorContext(t.Context()),
		app.ReturnEVMCash{SwapID: ids.SwapIDFrom(swap), Actor: "operator:alice", Reason: "stranded"})

	want := []app.EVMCashWriteOff{
		{
			SwapID: ids.SwapIDFrom(
				swap,
			),
			CabalID: e.cabal,
			Network: chain.NetworkBase,
			Amount:  money.MicrosFromUint64(3_000_000),
		},
	}
	if err != nil || got.Amount != held || got.Uncovered != want[0].Amount || !slices.Equal(offs.offs, want) {
		t.Fatalf("return = %+v, %v with write-offs %+v, want the 3 USDC gap written off once", got, err, offs.offs)
	}
	if status, amount := e.returnStepAmount(t, swap); status != "pending" || amount != held.String() {
		t.Fatalf("bridge_back %s %s, want pending for what the wallet holds", status, amount)
	}
	var detail string
	if err := e.pool.QueryRow(t.Context(), `SELECT detail->>'uncovered_micros' FROM swap_operator_actions
		WHERE swap_id = $1 AND action = 'return_evm_cash'`, swap).Scan(&detail); err != nil || detail != "3000000" {
		t.Fatalf("operator action uncovered %q, %v, want 3000000", detail, err)
	}
}

func TestReturnEVMCash_aWriteOffThatFailsQueuesNothing(t *testing.T) {
	t.Parallel()
	e := newRouteEnv(t)
	_, swap := e.strand(t)
	offs := &writeOffsFake{err: errs.New(errs.CodeInsufficientFunds, "test.writeoff")}

	_, err := e.returnerWith(&cashFake{balance: money.MicrosFromUint64(landedMicros - 1)}, offs).
		Handle(actorContext(t.Context()),
			app.ReturnEVMCash{SwapID: ids.SwapIDFrom(swap), Actor: "operator:alice", Reason: "stranded"})

	if errs.CodeOf(err) != errs.CodeInsufficientFunds {
		t.Fatalf("return err = %v, want the write-off failure", err)
	}
	if status, _ := e.returnStepAmount(t, swap); status != "" || e.returnActions(t, swap) != 0 {
		t.Fatalf(
			"bridge_back %q and %d operator actions after the failure, want neither",
			status,
			e.returnActions(t, swap),
		)
	}
}
