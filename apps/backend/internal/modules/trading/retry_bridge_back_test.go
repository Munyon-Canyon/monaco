package trading_test

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/trading/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/trading/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/multichain"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

func (e *routeEnv) retrier() *app.RetryBridgeBackHandler {
	return app.NewRetryBridgeBackHandler(db.New(e.pool, e.ids, e.clk), e.pool, e.clk, e.ids)
}

func (e *routeEnv) failedSale(t *testing.T, refuse bool, state multichain.ActionState) uuid.UUID {
	t.Helper()
	if refuse {
		e.privy.transferErr = errs.New(errs.CodeInvalidInput, "privy.Transfer")
	} else {
		e.privy.script(state)
	}
	view := e.mustBuy(t, e.selling(), domain.StatusConfirmed)
	if status, _ := e.bridgeBackOf(t, view.ID.UUID()); status != "failed" {
		t.Fatalf("bridge_back %s, want failed", status)
	}
	return view.ID.UUID()
}

func (e *routeEnv) retryBack(t *testing.T, swap uuid.UUID) domain.TradeStep {
	t.Helper()
	step, err := e.retrier().Handle(actorContext(t.Context()), app.RetryBridgeBack{
		SwapID: ids.SwapIDFrom(swap), Actor: "operator:alice", Reason: "privy rejected, balance restored",
	})
	if err != nil {
		t.Fatal(err)
	}
	e.clk.Advance(app.SwapSweepAge + time.Second)
	return step
}

func (e *routeEnv) attempts(t *testing.T, swap uuid.UUID) (attempt int, prior string, actions int) {
	t.Helper()
	if err := e.pool.QueryRow(t.Context(), `SELECT attempt, array_to_string(prior_privy_action_ids, ','),
		(SELECT count(*) FROM swap_operator_actions WHERE swap_id = $1 AND action = 'retry_bridge_back')
		FROM trade_steps WHERE trade_id = $1 AND step = 'bridge_back'`, swap).Scan(&attempt, &prior, &actions); err != nil {
		t.Fatal(err)
	}
	return attempt, prior, actions
}

func (e *routeEnv) stepAttempts(t *testing.T, swap uuid.UUID) []string {
	t.Helper()
	rows, err := e.pool.Query(t.Context(), `SELECT payload->>'status' || ':' || coalesce(payload->>'attempt', '-')
		FROM events WHERE type = 'trade.step_changed' AND aggregate_id = $1 AND payload->>'step' = 'bridge_back'
		ORDER BY id`, swap)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			t.Fatal(err)
		}
		out = append(out, s)
	}
	return out
}

func TestRetryBridgeBack_aRejectedReturnIsSentAgainUnderTheNextAttemptKey(t *testing.T) {
	t.Parallel()
	e := newRouteEnv(t)
	swap := e.failedSale(t, false, multichain.ActionRejected)

	after := e.retryBack(t, swap)
	if after.Status != domain.StepPending || after.Attempt != 2 ||
		!slices.Equal(after.PriorPrivyActionIDs, []string{"privy-action-1"}) {
		t.Fatalf("retried step = %+v, want pending attempt 2 after privy-action-1", after)
	}
	e.privy.script(multichain.ActionRejected, multichain.ActionSucceeded)
	e.mustTick(t, 1, 1)
	e.mustTick(t, 0, 0)

	calls := e.privy.calls()
	if len(calls) != 2 || calls[1].key != "trade:"+swap.String()+":bridge_back:2" || calls[1].spec != calls[0].spec {
		t.Fatalf("transfers = %+v, want the same return sent again under attempt 2's key", calls)
	}
	if status, action := e.bridgeBackOf(t, swap); status != "succeeded" || action != "privy-action-2" {
		t.Fatalf("bridge_back %s on %s, want succeeded on privy-action-2", status, action)
	}
	if attempt, prior, actions := e.attempts(t, swap); attempt != 2 || prior != "privy-action-1" || actions != 1 {
		t.Fatalf(
			"attempt %d after %s with %d operator actions, want 2 after privy-action-1 with 1",
			attempt,
			prior,
			actions,
		)
	}
	wantSteps(t, e.stepAttempts(t, swap), "pending:-", "submitted:-", "failed:-", "pending:2", "submitted:2",
		"succeeded:2")
}

func TestRetryBridgeBack_aPriorActionThatLandedLateIsAdoptedAndNothingIsSent(t *testing.T) {
	t.Parallel()
	e := newRouteEnv(t)
	swap := e.failedSale(t, false, multichain.ActionFailed)
	e.retryBack(t, swap)
	e.privy.script(multichain.ActionSucceeded, multichain.ActionSucceeded)

	e.mustTick(t, 1, 1)
	e.mustTick(t, 0, 0)

	if calls := e.privy.calls(); len(calls) != 1 {
		t.Fatalf("transfers = %+v, want only the first send", calls)
	}
	if status, action := e.bridgeBackOf(t, swap); status != "succeeded" || action != "privy-action-1" {
		t.Fatalf("bridge_back %s on %s, want succeeded on the prior privy-action-1", status, action)
	}
	wantSteps(t, e.stepAttempts(t, swap), "pending:-", "submitted:-", "failed:-", "pending:2", "submitted:2",
		"succeeded:2")
}

func TestRetryBridgeBack_aPriorActionStillPendingWaitsForTheNextTick(t *testing.T) {
	t.Parallel()
	e := newRouteEnv(t)
	swap := e.failedSale(t, false, multichain.ActionFailed)
	e.retryBack(t, swap)
	e.privy.script(multichain.ActionPending)

	e.mustTick(t, 1, 1)

	if status, _ := e.bridgeBackOf(t, swap); status != "pending" || len(e.privy.calls()) != 1 {
		t.Fatalf("bridge_back %s after %d transfers, want pending after 1", status, len(e.privy.calls()))
	}
}

func TestRetryBridgeBack_aSendPrivyRefusedRetriesWithoutLookingUpAnAction(t *testing.T) {
	t.Parallel()
	e := newRouteEnv(t)
	swap := e.failedSale(t, true, "")
	if after := e.retryBack(t, swap); len(after.PriorPrivyActionIDs) != 0 {
		t.Fatalf("prior actions %v, want none", after.PriorPrivyActionIDs)
	}
	e.privy.script(multichain.ActionSucceeded)
	polls := e.privy.polls

	e.mustTick(t, 1, 1)

	calls := e.privy.calls()
	if len(calls) != 1 || calls[0].key != "trade:"+swap.String()+":bridge_back:2" || e.privy.polls != polls+1 {
		t.Fatalf("transfers %+v after %d polls, want one under attempt 2 and only its own poll", calls,
			e.privy.polls-polls)
	}
	if status, _ := e.bridgeBackOf(t, swap); status != "succeeded" {
		t.Fatalf("bridge_back %s, want succeeded", status)
	}
}

func TestRetryBridgeBack_refusesAReturnThatIsNotFailedAndChangesNothing(t *testing.T) {
	t.Parallel()
	e := newRouteEnv(t)
	e.privy.script(multichain.ActionSucceeded)
	sold := e.mustBuy(t, e.selling(), domain.StatusConfirmed).ID.UUID()
	retried := e.failedSale(t, false, multichain.ActionRejected)
	e.retryBack(t, retried)
	cmd := func(swap uuid.UUID) app.RetryBridgeBack {
		return app.RetryBridgeBack{SwapID: ids.SwapIDFrom(swap), Actor: "operator:alice", Reason: "again"}
	}

	for _, status := range []string{"pending", "submitted", "succeeded"} {
		if _, err := e.pool.Exec(t.Context(), `UPDATE trade_steps SET status = $2
			WHERE trade_id = $1 AND step = 'bridge_back'`, sold, status); err != nil {
			t.Fatal(err)
		}
		if _, err := e.retrier().Handle(t.Context(), cmd(sold)); errs.CodeOf(err) != errs.CodeSwapNotStuck {
			t.Fatalf("retry of a %s return = %v, want swap_not_stuck", status, err)
		}
		if attempt, _, actions := e.attempts(t, sold); attempt != 1 || actions != 0 {
			t.Fatalf("%s return is at attempt %d with %d operator actions, want untouched", status, attempt, actions)
		}
	}
	if _, err := e.retrier().Handle(t.Context(), cmd(retried)); errs.CodeOf(err) != errs.CodeSwapNotStuck {
		t.Fatalf("second retry = %v, want swap_not_stuck while attempt 2 is in flight", err)
	}
	if attempt, _, actions := e.attempts(t, retried); attempt != 2 || actions != 1 {
		t.Fatalf("retried return is at attempt %d with %d operator actions, want 2 with 1", attempt, actions)
	}
	if _, err := e.retrier().Handle(t.Context(), cmd(e.ids.NewV7())); errs.CodeOf(err) != errs.CodeSwapNotFound {
		t.Fatalf("retry of an unknown swap = %v, want swap_not_found", err)
	}
	if _, err := e.retrier().Handle(t.Context(), app.RetryBridgeBack{
		SwapID: ids.SwapIDFrom(retried),
		Actor:  "operator:alice",
	}); errs.CodeOf(err) != errs.CodeInvalidInput {
		t.Fatalf("retry with no reason = %v, want invalid_input", err)
	}
}

func TestRetryBridgeBack_listsEachFailedReturn(t *testing.T) {
	t.Parallel()
	e := newRouteEnv(t)
	swap := e.failedSale(t, false, multichain.ActionRejected)

	failed, err := e.retrier().ListFailed(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	var got *app.FailedBridgeBack
	for i := range failed {
		if failed[i].SwapID.UUID() == swap {
			got = &failed[i]
		}
	}
	if got == nil || got.CabalID != e.cabal || got.Source != domain.SourceProposal || got.Attempt != 1 ||
		got.Error != "privy_rejected" || got.Amount.Uint64() != proceeds {
		t.Fatalf("failed returns = %+v, want swap %s at attempt 1 rejected by Privy", failed, swap)
	}
}

func TestRetryBridgeBack_aPriorActionLookupOutageLeavesTheStepPendingAndSendsNothing(t *testing.T) {
	t.Parallel()
	e := newRouteEnv(t)
	swap := e.failedSale(t, false, multichain.ActionFailed)
	e.retryBack(t, swap)
	e.privy.actionErr = errs.New(errs.CodePrivyUnavailable, "privy.Action")

	if _, err := e.tick(t.Context(), t); errs.CodeOf(err) != errs.CodePrivyUnavailable {
		t.Fatalf("Tick err = %v, want privy_unavailable", err)
	}

	if status, _ := e.bridgeBackOf(t, swap); status != "pending" || len(e.privy.calls()) != 1 {
		t.Fatalf("bridge_back %s after %d transfers, want pending after 1", status, len(e.privy.calls()))
	}
}

func TestRetryBridgeBack_readsTheFailedStepAndReportsAReadItCannotMakeAsInternal(t *testing.T) {
	t.Parallel()
	e := newRouteEnv(t)
	swap := e.failedSale(t, false, multichain.ActionRejected)
	if before, err := e.retrier().Step(t.Context(), ids.SwapIDFrom(swap)); err != nil ||
		before.Status != domain.StepFailed || before.Attempt != 1 || before.PrivyActionID != "privy-action-1" {
		t.Fatalf("Step = %+v, %v, want attempt 1 failed on privy-action-1", before, err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	if _, err := e.retrier().Step(ctx, ids.SwapIDFrom(e.ids.NewV7())); errs.CodeOf(err) != errs.CodeInternal {
		t.Fatalf("Step on a cancelled context = %v, want internal", err)
	}
	if _, err := e.retrier().ListFailed(ctx); errs.CodeOf(err) != errs.CodeInternal {
		t.Fatalf("ListFailed on a cancelled context = %v, want internal", err)
	}
}

func TestRetryBridgeBack_aFirstActionThatLandsLateIsFoundAfterARefusedSecondAttempt(t *testing.T) {
	t.Parallel()
	e := newRouteEnv(t)
	swap := e.failedSale(t, false, multichain.ActionFailed)
	e.retryBack(t, swap)
	e.privy.script(multichain.ActionPending)
	e.mustTick(t, 1, 1)
	e.privy.script(multichain.ActionFailed)
	e.privy.transferErr = errs.New(errs.CodeInvalidInput, "privy.Transfer")
	e.mustTick(t, 1, 1)
	if status, _ := e.bridgeBackOf(t, swap); status != "failed" {
		t.Fatalf("attempt 2 is %s, want failed by Privy's refusal", status)
	}

	if after := e.retryBack(t, swap); after.Attempt != 3 ||
		!slices.Equal(after.PriorPrivyActionIDs, []string{"privy-action-1"}) {
		t.Fatalf("retried step = %+v, want attempt 3 still holding privy-action-1", after)
	}
	e.privy.script(multichain.ActionSucceeded, multichain.ActionSucceeded)
	e.mustTick(t, 1, 1)

	if calls := e.privy.calls(); len(calls) != 1 {
		t.Fatalf("transfers = %+v, want only attempt 1's send", calls)
	}
	if status, action := e.bridgeBackOf(t, swap); status != "succeeded" || action != "privy-action-1" {
		t.Fatalf("bridge_back %s on %s, want succeeded on attempt 1's privy-action-1", status, action)
	}
}

func TestRetryBridgeBack_checksEveryEarlierAttemptsActionBeforeSending(t *testing.T) {
	t.Parallel()
	e := newRouteEnv(t)
	swap := e.failedSale(t, false, multichain.ActionFailed)
	e.retryBack(t, swap)
	e.privy.script(multichain.ActionFailed, multichain.ActionRejected)
	e.mustTick(t, 1, 1)

	if after := e.retryBack(t, swap); !slices.Equal(after.PriorPrivyActionIDs,
		[]string{"privy-action-1", "privy-action-2"}) {
		t.Fatalf("prior actions %v, want both earlier attempts", after.PriorPrivyActionIDs)
	}
	e.privy.script(multichain.ActionRejected, multichain.ActionSucceeded, multichain.ActionSucceeded)
	e.mustTick(t, 1, 1)

	if calls := e.privy.calls(); len(calls) != 2 {
		t.Fatalf("transfers = %+v, want attempts 1 and 2 only", calls)
	}
	if status, action := e.bridgeBackOf(t, swap); status != "succeeded" || action != "privy-action-2" {
		t.Fatalf("bridge_back %s on %s, want succeeded on attempt 2's late privy-action-2", status, action)
	}
}
