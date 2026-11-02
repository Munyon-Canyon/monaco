package trading_test

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/trading/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/trading/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/multichain"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

const chosenArrival = 24_990_937

type transfersFake struct {
	arrival app.EVMArrival
	found   bool
	err     error
	before  func()
	hashes  []string
	token   chain.EVMAddress
	owner   chain.EVMAddress
}

func (f *transfersFake) Transfer(
	_ context.Context, _ chain.Network, token, owner chain.EVMAddress, txHash string,
) (app.EVMArrival, bool, error) {
	f.hashes, f.token, f.owner = append(f.hashes, txHash), token, owner
	if f.before != nil {
		f.before()
	}
	return f.arrival, f.found, f.err
}

func transferOf(amount uint64) *transfersFake {
	return &transfersFake{found: true, arrival: app.EVMArrival{Amount: money.MicrosFromUint64(amount)}}
}

func (e *routeEnv) ambiguous(t *testing.T) uuid.UUID {
	t.Helper()
	e.landings.short, e.landings.missing = realBridgeFee, 1
	e.privy.script(multichain.ActionSucceeded, multichain.ActionSucceeded)
	e.evm.fills = []app.EVMFill{{State: app.FillSucceeded, Out: filledOut}}
	req := e.request()
	if _, err := e.buy(t.Context(), req); errs.CodeOf(err) != errs.CodeUpstreamUnavailable {
		t.Fatalf("Buy err = %v, want upstream_unavailable until the transfer shows", err)
	}
	swap := e.swapIDs(t, req.Source.ID)[0]
	e.exec1(t, `UPDATE trade_steps SET status = 'succeeded' WHERE trade_id = $1 AND step = 'bridge_out'`, swap)
	return swap
}

func (e *routeEnv) exec1(t *testing.T, query string, args ...any) {
	t.Helper()
	if _, err := e.pool.Exec(t.Context(), query, args...); err != nil {
		t.Fatal(err)
	}
}

func (e *routeEnv) chooser(transfers app.EVMTransfers) *app.ChooseBridgeArrivalHandler {
	return app.NewChooseBridgeArrivalHandler(e.uow, e.pool, e.clk, e.ids, e.treasury, transfers)
}

func chooseCmd(swap uuid.UUID, tx string) app.ChooseBridgeArrival {
	return app.ChooseBridgeArrival{
		SwapID: ids.SwapIDFrom(swap), Network: chain.NetworkBase, TxHash: tx, Actor: "operator:alice",
		Reason: "two transfers reached the treasury; this one carries the bridge", IdempotencyKey: "choose:" + tx,
	}
}

func (e *routeEnv) choose(
	t *testing.T, transfers app.EVMTransfers, cmd app.ChooseBridgeArrival,
) (app.BridgeArrivalChoice, error) {
	t.Helper()
	return e.chooser(transfers).Handle(t.Context(), cmd)
}

func (e *routeEnv) chosen(t *testing.T, swap uuid.UUID) (arrived int64, tx string, actions int) {
	t.Helper()
	arrived, tx, _ = e.arrival(t, swap)
	actions = e.count(t,
		`SELECT count(*) FROM swap_operator_actions WHERE swap_id = $1 AND action = 'choose_arrival'`, swap)
	return arrived, tx, actions
}

func (e *routeEnv) wantAudit(t *testing.T, swap uuid.UUID, tx string) {
	t.Helper()
	var actor, reason, detail string
	if err := e.pool.QueryRow(t.Context(), `SELECT actor, reason, detail::text FROM swap_operator_actions
		WHERE swap_id = $1`, swap).Scan(&actor, &reason, &detail); err != nil {
		t.Fatal(err)
	}
	if actor != "operator:alice" || !strings.Contains(reason, "carries the bridge") ||
		!strings.Contains(detail, tx) || !strings.Contains(detail, "choose:"+tx) {
		t.Fatalf("audit %q %q %s, want the actor, the reason, the tx and the key", actor, reason, detail)
	}
}

func wantChooseRefused(t *testing.T, err error, code errs.Code, reason string) {
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
}

func (e *routeEnv) wantNotChosen(t *testing.T, swap uuid.UUID) {
	t.Helper()
	if arrived, tx, actions := e.chosen(t, swap); arrived != 0 || tx != "" || actions != 0 {
		t.Fatalf("arrival %d %q with %d audit rows after a refusal, want nothing booked", arrived, tx, actions)
	}
}

func TestChooseBridgeArrival_booksTheChosenTransferOnceAndTheRouteResumes(t *testing.T) {
	t.Parallel()
	e := newRouteEnv(t)
	swap := e.ambiguous(t)
	transfers := transferOf(chosenArrival)
	scans := len(e.landings.seen)

	got, err := e.choose(t, transfers, chooseCmd(swap, txHash(0xabc)))

	want := app.BridgeArrivalChoice{
		SwapID: ids.SwapIDFrom(swap), CabalID: e.cabal, Network: chain.NetworkBase, TxHash: txHash(0xabc),
		Amount: money.MicrosFromUint64(chosenArrival), Fee: money.MicrosFromUint64(9_063), Changed: true,
	}
	if err != nil || got != want {
		t.Fatalf("choose = %+v, %v, want %+v", got, err, want)
	}
	if arrived, tx, actions := e.chosen(t, swap); arrived != chosenArrival || tx != txHash(0xabc) || actions != 1 {
		t.Fatalf("arrival %d %q with %d audit rows, want the chosen transfer and one audit row", arrived, tx, actions)
	}
	if transfers.owner != e.treasury.wallet.Address || transfers.token.String() != baseCash {
		t.Fatalf("transfer read for %s on %s, want the cash transfer to the treasury", transfers.owner, transfers.token)
	}
	e.wantAudit(t, swap, txHash(0xabc))
	steps := e.steps(t, swap)
	if last := steps[len(steps)-1]; last != "bridge_out:succeeded:9063" {
		t.Fatalf("last step event %s, want bridge_out:succeeded:9063", last)
	}

	e.resume(t, ids.SwapIDFrom(swap), domain.StatusConfirmed)

	e.wantSwappedTheArrival(t, swap)
	if len(e.landings.seen) != scans {
		t.Fatalf("%d arrival scans after the choice, want the %d before it", len(e.landings.seen), scans)
	}
}

func TestChooseBridgeArrival_aSecondCallChangesNothing(t *testing.T) {
	t.Parallel()
	e := newRouteEnv(t)
	swap := e.ambiguous(t)
	if _, err := e.choose(t, transferOf(chosenArrival), chooseCmd(swap, txHash(0xabc))); err != nil {
		t.Fatal(err)
	}
	events := e.count(t, `SELECT count(*) FROM events WHERE aggregate_id = $1`, swap)
	var updated string
	if err := e.pool.QueryRow(t.Context(), `SELECT updated_at::text FROM trade_steps
		WHERE trade_id = $1 AND step = 'bridge_out'`, swap).Scan(&updated); err != nil {
		t.Fatal(err)
	}
	again := transferOf(chosenArrival)

	got, err := e.choose(t, again, chooseCmd(swap, strings.ToUpper(txHash(0xabc)[:2])+txHash(0xabc)[2:]))

	if err != nil || got.Changed || got.TxHash != txHash(0xabc) || len(again.hashes) != 0 {
		t.Fatalf("second choose = %+v, %v after %d reads, want the booked choice unchanged",
			got, err, len(again.hashes))
	}
	var after string
	if err := e.pool.QueryRow(t.Context(), `SELECT updated_at::text FROM trade_steps
		WHERE trade_id = $1 AND step = 'bridge_out'`, swap).Scan(&after); err != nil {
		t.Fatal(err)
	}
	if _, _, actions := e.chosen(t, swap); actions != 1 || after != updated ||
		e.count(t, `SELECT count(*) FROM events WHERE aggregate_id = $1`, swap) != events {
		t.Fatalf("%d audit rows, updated %s -> %s, want no new row, event or update", actions, updated, after)
	}
}

func TestChooseBridgeArrival_refusesAnotherTransferOnceOneIsBooked(t *testing.T) {
	t.Parallel()
	e := newRouteEnv(t)
	swap := e.ambiguous(t)
	if _, err := e.choose(t, transferOf(chosenArrival), chooseCmd(swap, txHash(0xabc))); err != nil {
		t.Fatal(err)
	}

	_, err := e.choose(t, transferOf(chosenArrival), chooseCmd(swap, txHash(0xdef)))

	wantChooseRefused(t, err, errs.CodeSwapNotStuck, "arrival_already_booked")
	if _, tx, actions := e.chosen(t, swap); tx != txHash(0xabc) || actions != 1 {
		t.Fatalf("arrival %q with %d audit rows, want the first choice kept", tx, actions)
	}
}

func TestChooseBridgeArrival_refusesATransferAnotherSwapAlreadyBooked(t *testing.T) {
	t.Parallel()
	e := newRouteEnv(t)
	first := e.ambiguous(t)
	if _, err := e.choose(t, transferOf(chosenArrival), chooseCmd(first, txHash(0xabc))); err != nil {
		t.Fatal(err)
	}
	second := e.ambiguous(t)

	_, err := e.choose(t, transferOf(chosenArrival), chooseCmd(second, txHash(0xabc)))

	wantChooseRefused(t, err, errs.CodeInvalidInput, "arrival_tx_already_booked")
	e.wantNotChosen(t, second)
}

func TestChooseBridgeArrival_refusesWhatTheStepRangeAndTheTreasuryDoNotAllow(t *testing.T) {
	t.Parallel()
	floor := uint64(25_000_000 - bridgeFeeMicros - 100_000 - 250_000)
	for name, tc := range map[string]struct {
		transfers *transfersFake
		code      errs.Code
		reason    string
	}{
		"under the floor": {transferOf(floor - 1), errs.CodeInvalidInput, "amount_out_of_range"},
		"over the amount": {transferOf(25_000_001), errs.CodeInvalidInput, "amount_out_of_range"},
		"nothing":         {transferOf(0), errs.CodeInvalidInput, "amount_out_of_range"},
		"not to the treasury": {
			&transfersFake{}, errs.CodeInvalidInput, "not_a_cash_transfer_to_treasury",
		},
		"unreadable": {&transfersFake{err: errs.New(errs.CodeRPCUnavailable, "test")}, errs.CodeRPCUnavailable, ""},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			e := newRouteEnv(t)
			swap := e.ambiguous(t)

			_, err := e.choose(t, tc.transfers, chooseCmd(swap, txHash(0xabc)))

			if tc.reason == "" && errs.CodeOf(err) != tc.code {
				t.Fatalf("err = %v, want %s", err, tc.code)
			}
			if tc.reason != "" {
				wantChooseRefused(t, err, tc.code, tc.reason)
			}
			e.wantNotChosen(t, swap)
		})
	}
}

func TestChooseBridgeArrival_acceptsTheEdgesOfTheRange(t *testing.T) {
	t.Parallel()
	floor := uint64(25_000_000 - bridgeFeeMicros - 100_000 - 250_000)
	for name, amount := range map[string]uint64{"the floor": floor, "the amount sent": 25_000_000} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			e := newRouteEnv(t)
			swap := e.ambiguous(t)

			got, err := e.choose(t, transferOf(amount), chooseCmd(swap, txHash(0xabc)))

			if err != nil || !got.Changed || got.Amount.Uint64() != amount {
				t.Fatalf("choose = %+v, %v, want %d booked", got, err, amount)
			}
		})
	}
}

func TestChooseBridgeArrival_refusesASwapThatIsNotWaitingForAnArrival(t *testing.T) {
	t.Parallel()
	for name, change := range map[string]string{
		"submitted":     `UPDATE trade_steps SET status = 'submitted' WHERE trade_id = $1 AND step = 'bridge_out'`,
		"failed":        `UPDATE trade_steps SET status = 'failed', error = 'privy_failed' WHERE trade_id = $1 AND step = 'bridge_out'`,
		"no bridge out": `DELETE FROM trade_steps WHERE trade_id = $1 AND step = 'bridge_out'`,
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			e := newRouteEnv(t)
			swap := e.ambiguous(t)
			e.exec1(t, change, swap)
			transfers := transferOf(chosenArrival)

			_, err := e.choose(t, transfers, chooseCmd(swap, txHash(0xabc)))

			wantChooseRefused(t, err, errs.CodeSwapNotStuck, "bridge_out_not_succeeded")
			if len(transfers.hashes) != 0 {
				t.Fatalf("read %v from the chain, want no read for a swap that is not waiting", transfers.hashes)
			}
		})
	}
}

func TestChooseBridgeArrival_refusesInputThatCannotBeRight(t *testing.T) {
	t.Parallel()
	e := newRouteEnv(t)
	swap := e.ambiguous(t)
	for name, edit := range map[string]func(c *app.ChooseBridgeArrival){
		"no actor":      func(c *app.ChooseBridgeArrival) { c.Actor = "" },
		"no reason":     func(c *app.ChooseBridgeArrival) { c.Reason = "" },
		"long reason":   func(c *app.ChooseBridgeArrival) { c.Reason = strings.Repeat("é", 501) },
		"no key":        func(c *app.ChooseBridgeArrival) { c.IdempotencyKey = "" },
		"solana":        func(c *app.ChooseBridgeArrival) { c.Network = chain.NetworkSolana },
		"short hash":    func(c *app.ChooseBridgeArrival) { c.TxHash = "0x12" },
		"no prefix":     func(c *app.ChooseBridgeArrival) { c.TxHash = strings.TrimPrefix(txHash(1), "0x") },
		"not hex":       func(c *app.ChooseBridgeArrival) { c.TxHash = "0x" + strings.Repeat("z", 64) },
		"other network": func(c *app.ChooseBridgeArrival) { c.Network = chain.NetworkArbitrum },
	} {
		cmd := chooseCmd(swap, txHash(0xabc))
		edit(&cmd)

		_, err := e.chooser(transferOf(chosenArrival)).Handle(t.Context(), cmd)

		want := errs.CodeInvalidInput
		if errs.CodeOf(err) != want {
			t.Fatalf("%s: err = %v, want %s", name, err, want)
		}
		if _, plan := e.chooser(transferOf(chosenArrival)).Plan(t.Context(), cmd); errs.CodeOf(plan) != want {
			t.Fatalf("%s: plan err = %v, want %s", name, plan, want)
		}
	}
	e.wantNotChosen(t, swap)
}

func TestChooseBridgeArrival_refusesWhatItCannotFind(t *testing.T) {
	t.Parallel()
	e := newRouteEnv(t)
	swap := e.ambiguous(t)

	_, unknown := e.choose(t, transferOf(chosenArrival), chooseCmd(e.ids.NewV7(), txHash(0xabc)))
	e.treasury.err = errs.New(errs.CodeRPCUnavailable, "test")
	_, noWallet := e.choose(t, transferOf(chosenArrival), chooseCmd(swap, txHash(0xabc)))
	e.treasury.err = nil
	e.exec1(t, `UPDATE trade_routes SET comparison = '"x"'::jsonb WHERE trade_id = $1`, swap)
	_, garbled := e.choose(t, transferOf(chosenArrival), chooseCmd(swap, txHash(0xabc)))

	if errs.CodeOf(unknown) != errs.CodeSwapNotFound || errs.CodeOf(noWallet) != errs.CodeRPCUnavailable ||
		errs.CodeOf(garbled) != errs.CodeInternal {
		t.Fatalf("errs = %v, %v, %v, want swap_not_found, rpc_unavailable, internal", unknown, noWallet, garbled)
	}
	e.wantNotChosen(t, swap)
}

func TestChooseBridgeArrival_losesToAnArrivalBookedWhileItReadsTheChain(t *testing.T) {
	t.Parallel()
	book := func(e *routeEnv, swap uuid.UUID, tx string) func() {
		return func() {
			e.exec1(t, `UPDATE trade_steps SET arrived_micros = 24990000, arrival_tx = $2
				WHERE trade_id = $1 AND step = 'bridge_out'`, swap, tx)
		}
	}
	t.Run("another transfer", func(t *testing.T) {
		t.Parallel()
		e := newRouteEnv(t)
		swap := e.ambiguous(t)
		transfers := transferOf(chosenArrival)
		transfers.before = book(e, swap, txHash(0xdef))

		_, err := e.choose(t, transfers, chooseCmd(swap, txHash(0xabc)))

		wantChooseRefused(t, err, errs.CodeSwapNotStuck, "arrival_already_booked")
		if _, tx, actions := e.chosen(t, swap); tx != txHash(0xdef) || actions != 0 {
			t.Fatalf("arrival %q with %d audit rows, want the other booking kept and no audit row", tx, actions)
		}
	})
	t.Run("the same transfer", func(t *testing.T) {
		t.Parallel()
		e := newRouteEnv(t)
		swap := e.ambiguous(t)
		transfers := transferOf(chosenArrival)
		transfers.before = book(e, swap, txHash(0xabc))

		got, err := e.choose(t, transfers, chooseCmd(swap, txHash(0xabc)))

		if err != nil || got.Changed {
			t.Fatalf("choose = %+v, %v, want an unchanged choice", got, err)
		}
		if _, _, actions := e.chosen(t, swap); actions != 0 {
			t.Fatalf("%d audit rows, want none for a booking someone else made", actions)
		}
	})
	t.Run("the step is gone", func(t *testing.T) {
		t.Parallel()
		e := newRouteEnv(t)
		swap := e.ambiguous(t)
		transfers := transferOf(chosenArrival)
		transfers.before = func() {
			e.exec1(t, `DELETE FROM trade_steps WHERE trade_id = $1 AND step = 'bridge_out'`, swap)
		}

		_, err := e.choose(t, transfers, chooseCmd(swap, txHash(0xabc)))

		if errs.CodeOf(err) != errs.CodeNotFound {
			t.Fatalf("choose err = %v, want not_found", err)
		}
	})
}
