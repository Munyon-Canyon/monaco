package trading_test

import (
	"slices"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/trading/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/trading/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
)

func (e evmSwapEnv) approveKey() string { return e.key() + ":approve:0" }

func TestEVMSwap_sendsTheSwapOnlyAfterTheApprovalReceiptSucceeds(t *testing.T) {
	t.Parallel()
	e := newEVMSwapEnv(t)
	e.evm.receipts = []app.EVMFillState{app.FillPending, app.FillPending, app.FillSucceeded}
	e.evm.fills = []app.EVMFill{{State: app.FillSucceeded, Out: 300}}

	if _, err := e.run(t.Context()); err != nil {
		t.Fatal(err)
	}

	want := []string{
		"send:" + e.approveKey(), "receipt:0xhash1", "receipt:0xhash1", "receipt:0xhash1",
		"send:" + e.key(),
	}
	if got := e.evm.steps(); !slices.Equal(got, want) {
		t.Fatalf("trail = %v, want the approval mined before the swap is sent", got)
	}
}

func TestEVMSwap_aRevertedApprovalFailsTheSwapBeforeItIsSent(t *testing.T) {
	t.Parallel()
	e := newEVMSwapEnv(t)
	e.evm.receipts = []app.EVMFillState{app.FillReverted}

	_, err := e.run(t.Context())

	if errs.CodeOf(err) != errs.CodeSwapFailed || slices.Contains(e.evm.keys(), e.key()) {
		t.Fatalf("Run err = %v after sends %v, want swap_failed and no swap", err, e.evm.keys())
	}
	if !slicesContainsAttr(err, "approval_reverted") {
		t.Fatalf("err %v, want the reason approval_reverted", err)
	}
}

func TestEVMSwap_anUnconfirmedApprovalIsRetriedAndTheResumeSendsTheSwapOnce(t *testing.T) {
	t.Parallel()
	e := newEVMSwapEnv(t)
	pending := make([]app.EVMFillState, 64)
	for i := range pending {
		pending[i] = app.FillPending
	}
	e.evm.receipts = pending

	_, err := e.run(t.Context())

	if !errs.Retryable(errs.CodeOf(err)) || slices.Contains(e.evm.keys(), e.key()) {
		t.Fatalf("Run err = %v after sends %v, want a retryable error and no swap", err, e.evm.keys())
	}
	if s := e.step(t); s.Status != domain.StepPending {
		t.Fatalf("step = %+v, want the swap step still pending", s)
	}

	e.evm.receipts = nil
	e.evm.fills = []app.EVMFill{{State: app.FillSucceeded, Out: 300}}
	if _, err := e.run(t.Context()); err != nil {
		t.Fatal(err)
	}
	keys := e.evm.keys()
	swaps := 0
	for _, k := range keys {
		if k == e.key() {
			swaps++
		}
	}
	if swaps != 1 || keys[0] != e.approveKey() || keys[1] != e.approveKey() || e.evm.hashes() != 2 {
		t.Fatalf("sends %v over %d hashes, want the approval re-sent under its key and one swap", keys, e.evm.hashes())
	}
}

func TestEVMSwap_aReceiptItCannotReadStopsBeforeTheSwap(t *testing.T) {
	t.Parallel()
	e := newEVMSwapEnv(t)
	e.evm.receiptErr = errs.New(errs.CodeRPCUnavailable, "evm.receipt")

	if _, err := e.run(t.Context()); errs.CodeOf(err) != errs.CodeRPCUnavailable ||
		slices.Contains(e.evm.keys(), e.key()) {
		t.Fatalf("Run err = %v after sends %v, want rpc_unavailable and no swap", err, e.evm.keys())
	}
}

func slicesContainsAttr(err error, value string) bool {
	for _, a := range errs.Detail(err) {
		if a.Value.String() == value {
			return true
		}
	}
	return false
}

func TestEVMVenue_receiptReadsTheTransactionStatus(t *testing.T) {
	t.Parallel()
	for name, c := range map[string]struct {
		reply string
		want  app.EVMFillState
	}{
		"mined":    {receiptReply("0x1"), app.FillSucceeded},
		"reverted": {receiptReply("0x0"), app.FillReverted},
		"pending":  {`{"jsonrpc":"2.0","id":1,"result":null}`, app.FillPending},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			e := newVenueEnv(t)
			e.rpc.replies["/rpc"] = []string{c.reply}
			if got, err := e.venue.Receipt(t.Context(), chain.NetworkBase, venueHash); err != nil || got != c.want {
				t.Fatalf("Receipt = %v, %v, want %v", got, err, c.want)
			}
		})
	}
}
