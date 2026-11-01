package trading_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/trading/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/trading/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/multichain"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

func (e *routeEnv) receipts() app.RoutedReceipts {
	return app.RoutedReceipts{Reads: e.pool, Listings: e.listings, Wallets: e.treasury, Venue: e.evm}
}

func (e *routeEnv) forceRouted(
	t *testing.T, swap uuid.UUID, to domain.Status, out uint64, override bool,
) (app.SwapView, error) {
	t.Helper()
	cmd := app.ForceResolveSwap{
		Signature: e.signature(t, swap), To: to, Reason: "operator checked the explorer", Actor: "operator:alice",
		IdempotencyKey: "k1", Override: override,
	}
	if to == domain.StatusConfirmed {
		cmd.OutAmount = money.NewBaseUnits(out, 8)
	}
	handler := app.NewForceResolveSwapHandler(e.uow, e.pool, e.clk, e.ids, e.hints, nil, e.receipts())
	return handler.Handle(t.Context(), cmd)
}

func (e *routeEnv) audit(t *testing.T, swap uuid.UUID) string {
	t.Helper()
	var detail string
	const q = `SELECT detail::text FROM swap_operator_actions WHERE swap_id = $1`
	if err := e.pool.QueryRow(t.Context(), q, swap).Scan(&detail); err != nil {
		t.Fatalf("no operator audit row: %v", err)
	}
	return detail
}

func (e *routeEnv) wantUntouched(t *testing.T, swap uuid.UUID) {
	t.Helper()
	e.wantParked(t, swap, app.ParkedSwapReceiptMissing)
	if got := e.operatorActions(t, swap); len(got) != 0 {
		t.Fatalf("operator actions = %q, want none", got)
	}
	if step := e.swapStep(t, swap); step != "pending" && step != "submitted" {
		t.Fatalf("swap step is %s, want it left open", step)
	}
}

func TestForceResolveRouted_readsTheTreasuryTransferFromTheSwapsOwnReceipt(t *testing.T) {
	t.Parallel()
	e := newRouteEnv(t)
	swap := e.parkedWithoutReceipt(t)
	e.evm.inspects = []app.EVMFill{{State: app.FillSucceeded, Out: filledOut}}

	if _, err := e.forceRouted(t, swap, domain.StatusConfirmed, filledOut, false); err != nil {
		t.Fatal(err)
	}
	want := inspectCall{
		network: chain.NetworkBase, hash: string(e.signature(t, swap)), out: evmAddress(t, e.listing.TokenAddress),
		owner: evmAddress(t, evmTreasury),
	}
	if len(e.evm.inspected) != 1 || e.evm.inspected[0] != want {
		t.Fatalf("receipt reads = %+v, want one read %+v", e.evm.inspected, want)
	}
	detail := e.audit(t, swap)
	for _, part := range []string{`"chain_state": "landed"`, `"chain_out_amount": 305000000`, `"override": false`} {
		if !strings.Contains(detail, part) {
			t.Fatalf("audit = %s, want %s", detail, part)
		}
	}
}

func TestForceResolveRouted_refusesWhatTheReceiptContradicts(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct {
		fill app.EVMFill
		to   domain.Status
		out  uint64
		want string
	}{
		"failed but it landed":        {app.EVMFill{State: app.FillSucceeded, Out: filledOut}, domain.StatusFailed, 0, "landed successfully"},
		"confirmed but it reverted":   {app.EVMFill{State: app.FillReverted}, domain.StatusConfirmed, filledOut, "reverted"},
		"confirmed with wrong amount": {app.EVMFill{State: app.FillSucceeded, Out: filledOut}, domain.StatusConfirmed, filledOut + 1, "received 305000000, not 305000001"},
		"confirmed with no transfer":  {app.EVMFill{State: app.FillSucceeded}, domain.StatusConfirmed, filledOut, "received 0, not 305000000"},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			e := newRouteEnv(t)
			swap := e.parkedWithoutReceipt(t)
			e.evm.inspects = []app.EVMFill{tc.fill}

			_, err := e.forceRouted(t, swap, tc.to, tc.out, false)
			if errs.CodeOf(err) != errs.CodeInvalidInput || err == nil || !strings.Contains(err.Error(), tc.want) ||
				!strings.Contains(err.Error(), "--override") {
				t.Fatalf("Handle err = %v, want invalid_input naming %q and --override", err, tc.want)
			}
			e.wantUntouched(t, swap)
		})
	}
}

func TestForceResolveRouted_overrideBooksTheContradictionAndAuditsTheFacts(t *testing.T) {
	t.Parallel()
	e := newRouteEnv(t)
	swap := e.parkedWithoutReceipt(t)
	e.evm.inspects = []app.EVMFill{{State: app.FillSucceeded, Out: filledOut}}

	view, err := e.forceRouted(t, swap, domain.StatusFailed, 0, true)
	if err != nil || view.Status != domain.StatusFailed {
		t.Fatalf("Handle = %+v, %v, want the swap booked failed", view, err)
	}
	detail := e.audit(t, swap)
	for _, part := range []string{
		`"chain_state": "landed"`, `"override": true`, `"contradiction": "its receipt shows it landed successfully"`,
	} {
		if !strings.Contains(detail, part) {
			t.Fatalf("audit = %s, want %s", detail, part)
		}
	}
}

func TestForceResolveRouted_booksWhatTheReceiptAgreesWith(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct {
		fill  app.EVMFill
		to    domain.Status
		state string
	}{
		"reverted and failed":   {app.EVMFill{State: app.FillReverted}, domain.StatusFailed, "failed"},
		"no receipt and failed": {app.EVMFill{State: app.FillPending}, domain.StatusFailed, "not_found"},
		"no receipt and confirmed": {
			app.EVMFill{State: app.FillPending}, domain.StatusConfirmed, "not_found",
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			e := newRouteEnv(t)
			swap := e.parkedWithoutReceipt(t)
			e.evm.inspects = []app.EVMFill{tc.fill}

			view, err := e.forceRouted(t, swap, tc.to, filledOut, false)
			if err != nil || view.Status != tc.to {
				t.Fatalf("Handle = %+v, %v, want the swap booked %s", view, err, tc.to)
			}
			if detail := e.audit(t, swap); !strings.Contains(detail, `"chain_state": "`+tc.state+`"`) ||
				strings.Contains(detail, "contradiction") {
				t.Fatalf("audit = %s, want chain_state %s and no contradiction", detail, tc.state)
			}
		})
	}
}

func TestForceResolveRouted_refusesWhenTheReceiptCannotBeReadUnlessOverridden(t *testing.T) {
	t.Parallel()
	e := newRouteEnv(t)
	swap := e.parkedWithoutReceipt(t)
	e.evm.inspectErr = errs.New(errs.CodeRPCUnavailable, "test")

	if _, err := e.forceRouted(t, swap, domain.StatusFailed, 0, false); errs.CodeOf(err) != errs.CodeRPCUnavailable {
		t.Fatalf("Handle err = %v, want rpc_unavailable", err)
	}
	e.wantUntouched(t, swap)

	view, err := e.forceRouted(t, swap, domain.StatusFailed, 0, true)
	if err != nil || view.Status != domain.StatusFailed {
		t.Fatalf("overridden Handle = %+v, %v, want failed", view, err)
	}
	if detail := e.audit(t, swap); !strings.Contains(detail, `"chain_state": "unknown"`) {
		t.Fatalf("audit = %s, want chain_state unknown", detail)
	}
}

func TestForceResolveRouted_doesNotReadAReceiptForASwapItWillNotClose(t *testing.T) {
	t.Parallel()
	e := newRouteEnv(t)
	swap := e.submittedWithoutReceipt(t)

	if _, err := e.forceRouted(t, swap, domain.StatusFailed, 0, false); errs.CodeOf(err) != errs.CodeSwapNotStuck {
		t.Fatalf("Handle err = %v, want swap_not_stuck", err)
	}
	if len(e.evm.inspected) != 0 {
		t.Fatalf("receipt reads = %+v, want none for a swap that is not parked", e.evm.inspected)
	}
}

func TestResolveParkedSwap_failParkedRefusesASwapWhoseReceiptShowsItLanded(t *testing.T) {
	t.Parallel()
	e := newRouteEnv(t)
	swap := e.parkedWithoutReceipt(t)
	e.evm.inspects = []app.EVMFill{{State: app.FillSucceeded, Out: filledOut}}

	err := e.resolveParked(t, swap, app.ParkedFail)
	if errs.CodeOf(err) != errs.CodeInvalidInput || err == nil ||
		!strings.Contains(err.Error(), "landed successfully") {
		t.Fatalf("fail-parked err = %v, want invalid_input saying it landed", err)
	}
	e.wantUntouched(t, swap)
	if len(e.evm.inspected) != 1 || e.evm.inspected[0].hash != string(e.signature(t, swap)) {
		t.Fatalf("receipt reads = %+v, want one read of the swap's hash", e.evm.inspected)
	}
}

func TestResolveParkedSwap_failParkedRecordsTheReceiptItChecked(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct {
		fill  app.EVMFill
		state string
	}{
		"reverted": {app.EVMFill{State: app.FillReverted}, "failed"},
		"missing":  {app.EVMFill{State: app.FillPending}, "not_found"},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			e := newRouteEnv(t)
			swap := e.parkedWithoutReceipt(t)
			e.evm.inspects = []app.EVMFill{tc.fill}

			if err := e.resolveParked(t, swap, app.ParkedFail); err != nil {
				t.Fatal(err)
			}
			if status, _, _, _ := e.row(t, swap); status != string(domain.StatusFailed) {
				t.Fatalf("swap is %s, want failed", status)
			}
			if detail := e.audit(t, swap); !strings.Contains(detail, `"chain_state": "`+tc.state+`"`) {
				t.Fatalf("audit = %s, want chain_state %s", detail, tc.state)
			}
		})
	}
}

func TestResolveParkedSwap_failParkedRefusesWhenTheReceiptCannotBeRead(t *testing.T) {
	t.Parallel()
	e := newRouteEnv(t)
	swap := e.parkedWithoutReceipt(t)
	e.evm.inspectErr = errs.New(errs.CodeRPCUnavailable, "test")

	if err := e.resolveParked(t, swap, app.ParkedFail); errs.CodeOf(err) != errs.CodeRPCUnavailable {
		t.Fatalf("fail-parked err = %v, want rpc_unavailable", err)
	}
	e.wantUntouched(t, swap)
}

func TestResolveParkedSwap_failParkedReadsNoReceiptForASwapThatNeverBroadcast(t *testing.T) {
	t.Parallel()
	e := newRouteEnv(t)
	swap := e.parkedBridged(t)

	if err := e.resolveParked(t, swap, app.ParkedFail); err != nil {
		t.Fatal(err)
	}
	if len(e.evm.inspected) != 0 {
		t.Fatalf("receipt reads = %+v, want none", e.evm.inspected)
	}
	if detail := e.audit(t, swap); !strings.Contains(detail, `"chain_state": "not_checked"`) {
		t.Fatalf("audit = %s, want chain_state not_checked", detail)
	}
}

func TestResolveParkedSwap_unparkReadsNoReceipt(t *testing.T) {
	t.Parallel()
	e := newRouteEnv(t)
	swap := e.parkedBridged(t)

	if err := e.resolveParked(t, swap, app.ParkedUnpark); err != nil {
		t.Fatal(err)
	}
	if len(e.evm.inspected) != 0 {
		t.Fatalf("receipt reads = %+v, want none", e.evm.inspected)
	}
}

func TestResolveParkedSwap_aSellsReceiptIsReadForTheCashItPays(t *testing.T) {
	t.Parallel()
	e := newRouteEnv(t)
	req := e.selling()
	e.evm.fills = nil
	e.privy.script(multichain.ActionSucceeded)
	swap := e.mustBuy(t, req, domain.StatusSubmitted).ID.UUID()
	e.clk.Advance(app.SwapSweepAge + time.Second)
	if _, err := e.tick(t.Context(), t); err != nil {
		t.Fatal(err)
	}
	e.clk.Advance(11 * time.Minute)
	if _, err := e.tick(t.Context(), t); err != nil {
		t.Fatal(err)
	}
	e.wantParked(t, swap, app.ParkedSwapReceiptMissing)
	e.evm.inspects = []app.EVMFill{{State: app.FillSucceeded, Out: proceeds}}

	if err := e.resolveParked(t, swap, app.ParkedFail); errs.CodeOf(err) != errs.CodeInvalidInput {
		t.Fatalf("fail-parked err = %v, want invalid_input for a sell that landed", err)
	}
	if len(e.evm.inspected) != 1 || e.evm.inspected[0].out != evmAddress(t, baseCash) ||
		e.evm.inspected[0].owner != evmAddress(t, evmTreasury) {
		t.Fatalf("receipt reads = %+v, want one read of the cash paid to the treasury", e.evm.inspected)
	}
}

func TestRoutedReceipts_readsNothingForASwapWithNoSwapStep(t *testing.T) {
	t.Parallel()
	e := newRouteEnv(t)
	swap := e.parkedBridged(t)

	fill, err := e.receipts().Receipt(t.Context(), ids.SwapIDFrom(swap))
	if err != nil || fill.State != app.FillPending || len(e.evm.inspected) != 0 {
		t.Fatalf("Receipt = %+v, %v with %d reads, want pending and no read", fill, err, len(e.evm.inspected))
	}
}

func TestRoutedReceipts_refusesWhatItCannotResolve(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct {
		breakIt func(t *testing.T, e *routeEnv, swap uuid.UUID, r *app.RoutedReceipts)
		want    errs.Code
	}{
		"listing gone": {func(_ *testing.T, _ *routeEnv, _ uuid.UUID, r *app.RoutedReceipts) { r.Listings = listingsFake{} }, errs.CodeAssetNotFound},
		"wallet gone": {
			func(_ *testing.T, e *routeEnv, _ uuid.UUID, _ *app.RoutedReceipts) {
				e.treasury.err = errs.New(errs.CodeNotFound, "test")
			},
			errs.CodeNotFound,
		},
		"token unreadable": {func(_ *testing.T, e *routeEnv, _ uuid.UUID, r *app.RoutedReceipts) {
			bad := e.listing
			bad.TokenAddress = "not-an-address"
			r.Listings = listingsFake{listings: map[multichain.ListingID]multichain.Listing{bad.ID: bad}}
		}, errs.CodeDecodeFailed},
		"swap unreadable": {func(t *testing.T, e *routeEnv, swap uuid.UUID, _ *app.RoutedReceipts) {
			t.Helper()
			e.mustExec(t, `UPDATE trade_routes SET comparison = '[]'::jsonb WHERE trade_id = $1`, swap)
		}, errs.CodeInternal},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			e := newRouteEnv(t)
			swap := e.parkedWithoutReceipt(t)
			receipts := e.receipts()
			tc.breakIt(t, e, swap, &receipts)

			if _, err := receipts.Receipt(t.Context(), ids.SwapIDFrom(swap)); errs.CodeOf(err) != tc.want {
				t.Fatalf("Receipt err = %v, want %s", err, tc.want)
			}
		})
	}
}

func TestRoutedResolution_refusesWhenNoReceiptReaderIsWired(t *testing.T) {
	t.Parallel()
	e := newRouteEnv(t)
	swap := e.parkedWithoutReceipt(t)
	force := app.NewForceResolveSwapHandler(e.uow, e.pool, e.clk, e.ids, e.hints, nil, nil)
	_, err := force.Handle(t.Context(), app.ForceResolveSwap{
		Signature: e.signature(t, swap), To: domain.StatusFailed, Reason: "no reader", Actor: "operator:alice",
		IdempotencyKey: "k1",
	})
	if errs.CodeOf(err) != errs.CodeNotImplemented {
		t.Fatalf("force-resolve err = %v, want not_implemented", err)
	}
	parked := app.NewResolveParkedSwapHandler(e.uow, e.pool, e.clk, e.ids, e.hints, nil)
	_, err = parked.Handle(t.Context(), app.ResolveParkedSwap{
		SwapID: ids.SwapIDFrom(swap), Action: app.ParkedFail, Actor: parkedActor, Reason: "no reader",
	})
	if errs.CodeOf(err) != errs.CodeNotImplemented {
		t.Fatalf("fail-parked err = %v, want not_implemented", err)
	}
	e.wantUntouched(t, swap)
}

func TestRoutedReceipts_refusesWhenTheStepCannotBeRead(t *testing.T) {
	t.Parallel()
	e := newRouteEnv(t)
	swap := e.parkedWithoutReceipt(t)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	if _, err := e.receipts().Receipt(ctx, ids.SwapIDFrom(swap)); errs.CodeOf(err) != errs.CodeInternal {
		t.Fatalf("Receipt err = %v, want internal", err)
	}
}
