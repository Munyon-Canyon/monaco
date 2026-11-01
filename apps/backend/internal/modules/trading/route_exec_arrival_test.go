package trading_test

import (
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/trading/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/trading/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/multichain"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

const realBridgeFee = bridgeFeeMicros + 1_563

func (e *routeEnv) arrival(t *testing.T, swap uuid.UUID) (int64, string, time.Time) {
	t.Helper()
	var arrived int64
	var tx string
	var created time.Time
	if err := e.pool.QueryRow(t.Context(), `SELECT coalesce(arrived_micros, 0), coalesce(arrival_tx, ''), created_at
		FROM trade_steps WHERE trade_id = $1 AND step = 'bridge_out'`, swap).Scan(&arrived, &tx, &created); err != nil {
		t.Fatal(err)
	}
	return arrived, tx, created
}

func (e *routeEnv) wantSwappedTheArrival(t *testing.T, swap uuid.UUID) {
	t.Helper()
	confirmed := e.payload(t, swap, "trade.confirmed")
	if confirmed["in_amount"] != "24990937" || confirmed["fee_micros"] != "9063" ||
		confirmed["usdc_micros"] != "25000000" {
		t.Fatalf("trade.confirmed = %v, want the 24990937 that arrived and the real 9063 fee", confirmed)
	}
	specs := e.evm.specs
	if last := specs[len(specs)-1]; last.AmountIn != 24_990_937 {
		t.Fatalf("swap built for %d, want the 24990937 that arrived", last.AmountIn)
	}
}

func TestRouteExec_aBridgeThatLandsLessThanEstimatedSwapsWhatArrived(t *testing.T) {
	t.Parallel()
	e := newRouteEnv(t)
	e.landings.short = realBridgeFee
	e.bridgedToTheSwap(t)
	e.evm.fills = []app.EVMFill{{State: app.FillSucceeded, Out: filledOut}}

	view := e.mustBuy(t, e.request(), domain.StatusConfirmed)

	e.wantSwappedTheArrival(t, view.ID.UUID())
	if arrived, tx, _ := e.arrival(t, view.ID.UUID()); arrived != 24_990_937 || tx == "" {
		t.Fatalf("bridge_out arrival = %d %q, want the 24990937 transfer recorded", arrived, tx)
	}
	wantSteps(t, e.steps(t, view.ID.UUID()), "bridge_out:pending:0", "bridge_out:submitted:7500",
		"bridge_out:succeeded:9063", "swap:pending:0", "swap:submitted:0", "swap:succeeded:0")
	wantFloor := money.MicrosFromUint64(25_000_000 - bridgeFeeMicros - 100_000 - 250_000)
	if floors := e.landings.floors; len(floors) != 1 || floors[0] != wantFloor {
		t.Fatalf("arrival scan floors = %v, want one at the quoted fee less its slack, %v", floors, wantFloor)
	}
}

func TestRouteExec_aBridgeNotYetOnChainStaysSubmittedAndResumes(t *testing.T) {
	t.Parallel()
	e := newRouteEnv(t)
	e.landings.short, e.landings.missing = realBridgeFee, 1
	e.privy.script(multichain.ActionSucceeded, multichain.ActionSucceeded)
	e.evm.fills = []app.EVMFill{{State: app.FillSucceeded, Out: filledOut}}
	req := e.request()

	if _, err := e.buy(t.Context(), req); errs.CodeOf(err) != errs.CodeUpstreamUnavailable {
		t.Fatalf("Buy err = %v, want upstream_unavailable until the transfer shows", err)
	}
	swap := e.swapIDs(t, req.Source.ID)[0]
	if status, _ := e.bridgeOutOf(t, swap); status != string(domain.StepSubmitted) {
		t.Fatalf("bridge_out is %s, want submitted", status)
	}
	if status, _, _, _ := e.row(t, swap); status != string(domain.StatusCreated) {
		t.Fatalf("swap is %s, want created", status)
	}
	e.resume(t, ids.SwapIDFrom(swap), domain.StatusConfirmed)

	e.wantSwappedTheArrival(t, swap)
	if len(e.privy.calls()) != 1 {
		t.Fatalf("%d transfers, want the one bridge", len(e.privy.calls()))
	}
}

func TestRetry_aReusedBridgeOutWithoutAnArrivalObservesItOnce(t *testing.T) {
	t.Parallel()
	e := newRouteEnv(t)
	e.landings.short = realBridgeFee
	e.privy.script(multichain.ActionSucceeded)
	e.evm.fills = []app.EVMFill{{State: app.FillReverted}}
	cmd := e.routedBuy(t)
	first := e.swapsOf(t, cmd.ProposalID)[0]
	if _, err := e.pool.Exec(t.Context(), `UPDATE trade_steps SET arrived_micros = NULL, arrival_tx = NULL
		WHERE trade_id = $1`, first); err != nil {
		t.Fatal(err)
	}
	_, _, sentAt := e.arrival(t, first)
	e.ledger.SetTokens(e.wallet.Address, usdcToken(), 0)
	e.requote()
	e.evm.fills = []app.EVMFill{{State: app.FillSucceeded, Out: filledOut}}
	seen := len(e.landings.seen)

	if code := e.retry(t, cmd, first); code != "ok" {
		t.Fatalf("retry delivery %s, want ok", code)
	}
	retry := e.swapsOf(t, cmd.ProposalID)[1]
	e.resume(t, ids.SwapIDFrom(retry), domain.StatusConfirmed)
	if _, err := e.exec().Stranded(actorContext(t.Context()), ids.SwapIDFrom(retry)); err != nil {
		t.Fatal(err)
	}

	if looks := e.landings.seen[seen:]; len(looks) != 1 || !looks[0].Equal(sentAt) {
		t.Fatalf("arrival lookups %v, want one from the original bridge out at %s", looks, sentAt)
	}
	if arrived, _, _ := e.arrival(t, retry); arrived != 24_990_937 {
		t.Fatalf("reused bridge_out arrival = %d, want 24990937", arrived)
	}
	wantSteps(t, e.steps(t, retry), "bridge_out:succeeded:9063", "swap:pending:0", "swap:submitted:0",
		"swap:succeeded:0")
	e.wantSwappedTheArrival(t, retry)
}

func TestRouteExec_strandedCashIsWhatArrived(t *testing.T) {
	t.Parallel()
	e := newRouteEnv(t)
	e.landings.short = realBridgeFee
	_, swap := e.strand(t)

	got, err := e.exec().Stranded(actorContext(t.Context()), ids.SwapIDFrom(swap))

	if err != nil || got.OnEVM.Uint64() != 24_990_937 || got.Back.Uint64() != 24_990_937-bridgeFeeMicros {
		t.Fatalf("Stranded = %+v, %v, want the arrival on EVM less the estimated fee back", got, err)
	}
}
