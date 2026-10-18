package trading_test

import (
	"slices"
	"strconv"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	governance "github.com/monaco/monaco/apps/backend/internal/modules/governance/port"
	"github.com/monaco/monaco/apps/backend/internal/modules/trading/app"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/multichain"
)

func (e *routeEnv) backAmounts(t *testing.T, swap uuid.UUID) []string {
	t.Helper()
	rows, err := e.pool.Query(t.Context(), `SELECT payload->>'status' || ':' || (payload->>'amount_micros')
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
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}

func TestBridgeBack_aRetryableBuyHoldsItsCashForADayThenReturnsIt(t *testing.T) {
	t.Parallel()
	e := newRouteEnv(t)
	_, swap := e.strand(t)
	e.privy.script(multichain.ActionSucceeded)

	e.clk.Advance(app.StrandedReturnHold - app.SwapSweepAge - 2*time.Second)
	e.mustTick(t, 1, 0)
	if status, _ := e.bridgeBackOf(t, swap); status != "" || len(e.privy.calls()) != 1 {
		t.Fatalf("bridge_back %q after %d transfers inside the hold, want none", status, len(e.privy.calls()))
	}

	e.clk.Advance(3 * time.Second)
	e.mustTick(t, 1, 1)

	if status, _ := e.bridgeBackOf(t, swap); status != "succeeded" {
		t.Fatalf("bridge_back %q past the hold, want succeeded", status)
	}
	landed := strconv.Itoa(landedMicros)
	want := []string{"pending:" + landed, "submitted:" + landed, "succeeded:" + landed}
	if got := e.backAmounts(t, swap); !slices.Equal(got, want) {
		t.Fatalf("bridge_back events = %v, want the whole landed cash sent and arrived %v", got, want)
	}
}

func TestBridgeBack_theReturnRunsOnceAndALaterRetryBridgesOutFresh(t *testing.T) {
	t.Parallel()
	e := newRouteEnv(t)
	cmd, swap := e.strand(t)
	e.privy.script(multichain.ActionSucceeded)
	e.clk.Advance(app.StrandedReturnHold)
	e.mustTick(t, 1, 1)
	sent := len(e.privy.calls())

	e.mustTick(t, 0, 0)
	e.mustTick(t, 0, 0)
	if backs := e.count(t, `SELECT count(*) FROM trade_steps WHERE step = 'bridge_back'`); backs != 1 ||
		len(e.privy.calls()) != sent {
		t.Fatalf("%d bridge_back steps and %d transfers after repeat ticks, want 1 and %d",
			backs, len(e.privy.calls()), sent)
	}

	e.requote()
	e.privy.script(multichain.ActionSucceeded)
	e.evm.fills = []app.EVMFill{{State: app.FillSucceeded, Out: filledOut}}
	if code := e.retry(t, cmd, swap); code != "ok" {
		t.Fatalf("retry after the return delivery %q, want ok", code)
	}
	retried := e.swapsOf(t, cmd.ProposalID)
	if len(retried) != 2 {
		t.Fatalf("swaps %v, want the failed buy and its retry", retried)
	}
	if status, action := e.bridgeOutOf(t, retried[1]); status != "succeeded" || action == "reused:"+swap.String() {
		t.Fatalf("retry bridge_out %s %s, want a fresh bridge from Solana", status, action)
	}
}

func TestBridgeBack_cashAReusedBuyHoldsIsReturnedOnceFromTheHolder(t *testing.T) {
	t.Parallel()
	e := newRouteEnv(t)
	cmd, first := e.strand(t)
	e.requote()
	e.evm.fills = []app.EVMFill{{State: app.FillReverted}}
	if code := e.retry(t, cmd, first); code != string(errs.CodeSwapFailed) {
		t.Fatalf("retry delivery %s, want swap_failed", code)
	}
	e.proposals.set(governance.StatusPassed)
	e.privy.script(multichain.ActionSucceeded)
	holder := e.swapsOf(t, cmd.ProposalID)[1]

	e.clk.Advance(app.StrandedReturnHold)
	for range 3 {
		if _, err := e.tick(t.Context(), t); err != nil {
			t.Fatal(err)
		}
	}

	if status, _ := e.bridgeBackOf(t, holder); status != "succeeded" {
		t.Fatalf("holder bridge_back %q, want succeeded", status)
	}
	if status, _ := e.bridgeBackOf(t, first); status != "" {
		t.Fatalf("the swap that gave its cash away has bridge_back %q, want none", status)
	}
	if backs := e.count(t, `SELECT count(*) FROM trade_steps WHERE step = 'bridge_back'`); backs != 1 {
		t.Fatalf("%d bridge_back steps, want the cash returned once", backs)
	}
}
