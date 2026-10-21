package funding_test

import (
	"context"
	"sync/atomic"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/funding/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/funding/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/funding/port"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

func (f *withdrawFixture) withdrawKeyed(t *testing.T, hash string) (app.WithdrawResult, error) {
	t.Helper()
	return f.withdrawKeyedIn(f.ctx(t.Context()), hash)
}

func (f *withdrawFixture) withdrawKeyedIn(ctx context.Context, hash string) (app.WithdrawResult, error) {
	return f.handler.Handle(ctx, app.Withdraw{
		ID: ids.Real{}.NewV7(), UserID: f.user.ID,
		Request: domain.WithdrawalRequest{Amount: money.MicrosFromUint64(withdrawMicros), To: withdrawTo},
		Intent: db.IntentKey{
			ActorKey: "user:" + f.user.ID.String(), Key: "k1", Operation: app.WithdrawOperation,
			RequestHash: []byte(hash),
		},
	})
}

func (f *withdrawFixture) intents(t *testing.T) int {
	t.Helper()
	var n int
	if err := f.pool.QueryRow(t.Context(), `SELECT count(*) FROM money_intents`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestWithdraw_aReplayWithTheSameKeyReturnsTheFirstWithdrawalAndSendsOnce(t *testing.T) {
	t.Parallel()
	f := newWithdrawFixture(t, 9_000_000)
	first, err := f.withdrawKeyed(t, "h1")
	if err != nil {
		t.Fatal(err)
	}
	again, err := f.withdrawKeyed(t, "h1")
	if err != nil {
		t.Fatal(err)
	}
	if again.ID != first.ID || again.Status != domain.WithdrawalSubmitted || again.TxSignature != withdrawSig {
		t.Fatalf("replay = %+v, want the first withdrawal %+v", again, first)
	}
	if rows := f.rows(t); len(rows) != 1 || len(f.transfers.builds) != 1 || len(f.transfers.sent) != 1 {
		t.Fatalf("rows = %d builds = %d sent = %d, want one of each",
			len(rows), len(f.transfers.builds), len(f.transfers.sent))
	}
	if got := len(f.submittedEvents(t)); got != 1 || f.intents(t) != 1 {
		t.Fatalf("submitted events = %d intents = %d, want 1 and 1", got, f.intents(t))
	}
}

type observeSwitch struct {
	port.Balances
	down atomic.Bool
}

func (b *observeSwitch) Observe(ctx context.Context, user ids.UserID) (port.Observation, error) {
	if b.down.Load() {
		return port.Observation{}, errs.New(errs.CodeRPCUnavailable, "test")
	}
	return b.Balances.Observe(ctx, user)
}

func TestWithdraw_aReplayDoesNotNeedTheChainReadToSucceed(t *testing.T) {
	t.Parallel()
	var balances *observeSwitch
	f := newWithdrawFixture(t, 9_000_000, func(d *app.WithdrawDeps) {
		balances = &observeSwitch{Balances: d.Balances}
		d.Balances = balances
	})
	first, err := f.withdrawKeyed(t, "h1")
	if err != nil {
		t.Fatal(err)
	}
	balances.down.Store(true)
	again, err := f.withdrawKeyed(t, "h1")
	if err != nil || again.ID != first.ID || len(f.rows(t)) != 1 {
		t.Fatalf("replay with the chain read down = %+v, %v rows = %d, want the first withdrawal %+v",
			again, err, len(f.rows(t)), first)
	}
}

func TestWithdraw_aReplayAfterACrashBeforeTheSignedBytesResumesTheSameRow(t *testing.T) {
	t.Parallel()
	f := newWithdrawFixture(t, 9_000_000)
	f.transfers.beforeSign = func() { panic("killed while signing") }
	func() {
		defer func() { _ = recover() }()
		_, _ = f.withdrawKeyed(t, "h1")
	}()
	f.transfers.beforeSign = nil
	if rows := f.rows(t); len(rows) != 1 || rows[0].status != "created" || len(f.transfers.sent) != 0 {
		t.Fatalf("after the crash rows = %+v sent = %d", rows, len(f.transfers.sent))
	}
	got, err := f.withdrawKeyed(t, "h1")
	if err != nil {
		t.Fatal(err)
	}
	rows := f.rows(t)
	if len(rows) != 1 || rows[0].status != "submitted" || rows[0].signature != string(withdrawSig) ||
		len(f.transfers.sent) != 1 || got.Status != domain.WithdrawalSubmitted || got.TxSignature != withdrawSig {
		t.Fatalf("resume = %+v rows = %+v sent = %d, want the one row submitted and sent once",
			got, rows, len(f.transfers.sent))
	}
}

func TestWithdraw_aReplayAfterTheBroadcastFailedReturnsTheSubmittedWithdrawalWithoutSigningAgain(t *testing.T) {
	t.Parallel()
	f := newWithdrawFixture(t, 9_000_000)
	f.transfers.sendErr = errs.New(errs.CodeRPCUnavailable, "test")
	first, err := f.withdrawKeyed(t, "h1")
	if err != nil {
		t.Fatal(err)
	}
	again, err := f.withdrawKeyed(t, "h1")
	if err != nil || again.ID != first.ID || len(f.transfers.builds) != 1 || len(f.rows(t)) != 1 {
		t.Fatalf("replay = (%+v, %v) builds = %d rows = %d", again, err, len(f.transfers.builds), len(f.rows(t)))
	}
}

func TestWithdraw_theSameKeyWithAnotherRequestIsAMismatch(t *testing.T) {
	t.Parallel()
	f := newWithdrawFixture(t, 9_000_000)
	if _, err := f.withdrawKeyed(t, "h1"); err != nil {
		t.Fatal(err)
	}
	if _, err := f.withdrawKeyed(t, "h2"); errs.CodeOf(err) != errs.CodeIdempotencyMismatch {
		t.Fatalf("another request under the key = %v, want %s", err, errs.CodeIdempotencyMismatch)
	}
	if rows := f.rows(t); len(rows) != 1 || len(f.transfers.sent) != 1 {
		t.Fatalf("rows = %d sent = %d, want 1 and 1", len(rows), len(f.transfers.sent))
	}
}

func TestWithdraw_aFailureBeforeAnythingWasSentFreesTheKey(t *testing.T) {
	t.Parallel()
	f := newWithdrawFixture(t, 9_000_000)
	f.transfers.buildErr = errs.New(errs.CodePrivyUnavailable, "test")
	if _, err := f.withdrawKeyed(t, "h1"); errs.CodeOf(err) != errs.CodePrivyUnavailable {
		t.Fatalf("first attempt = %v, want %s", err, errs.CodePrivyUnavailable)
	}
	if f.intents(t) != 0 {
		t.Fatalf("intents = %d after an unsent failure, want the key freed", f.intents(t))
	}
	f.transfers.buildErr = nil
	got, err := f.withdrawKeyed(t, "h1")
	if err != nil || got.Status != domain.WithdrawalSubmitted {
		t.Fatalf("retry = (%+v, %v), want a submitted withdrawal", got, err)
	}
	if rows := f.rows(t); len(rows) != 2 || rows[0].status != "failed" || rows[1].status != "submitted" {
		t.Fatalf("rows = %+v, want the unsent one failed and the retry submitted", rows)
	}
}

func (f *withdrawFixture) withdrawEVMKeyed(t *testing.T) (app.WithdrawResult, error) {
	t.Helper()
	request, err := domain.ParseWithdrawalRequest(domain.RawWithdrawal{
		Amount: "2000000", Kind: "evm", Network: "base", To: evmTo,
	})
	if err != nil {
		t.Fatal(err)
	}
	return f.handler.Handle(f.ctx(t.Context()), app.Withdraw{
		ID: ids.Real{}.NewV7(), UserID: f.user.ID, Request: request,
		Intent: db.IntentKey{
			ActorKey: "user:" + f.user.ID.String(), Key: "k1", Operation: app.WithdrawOperation,
			RequestHash: []byte("h1"),
		},
	})
}

func TestWithdrawEVM_aReplayWithTheSameKeyReturnsTheFirstWithdrawalAndTransfersOnce(t *testing.T) {
	t.Parallel()
	actions := &privyActions{}
	f := newEVMFixture(t, actions)
	first, err := f.withdrawEVMKeyed(t)
	if err != nil {
		t.Fatal(err)
	}
	again, err := f.withdrawEVMKeyed(t)
	if err != nil || again.ID != first.ID || again.Status != domain.WithdrawalSubmitted {
		t.Fatalf("replay = (%+v, %v), want the first withdrawal %+v", again, err, first)
	}
	if len(actions.calls) != 1 || len(f.rows(t)) != 1 {
		t.Fatalf("transfers = %d rows = %d, want 1 and 1", len(actions.calls), len(f.rows(t)))
	}
}

func TestWithdrawEVM_aReplayAfterATransientFailureResumesTheSameRow(t *testing.T) {
	t.Parallel()
	actions := &privyActions{transferErr: errs.New(errs.CodeRPCUnavailable, "test")}
	f := newEVMFixture(t, actions)
	if _, err := f.withdrawEVMKeyed(t); errs.CodeOf(err) != errs.CodeRPCUnavailable {
		t.Fatalf("first attempt = %v, want %s", err, errs.CodeRPCUnavailable)
	}
	actions.transferErr = nil
	got, err := f.withdrawEVMKeyed(t)
	if err != nil || got.Status != domain.WithdrawalSubmitted {
		t.Fatalf("resume = (%+v, %v), want a submitted withdrawal", got, err)
	}
	if rows := f.rows(t); len(rows) != 1 || rows[0].status != "submitted" || actions.actionsCreated() != 1 {
		t.Fatalf("rows = %+v actions = %d, want the one row submitted with one Privy action",
			rows, actions.actionsCreated())
	}
}
