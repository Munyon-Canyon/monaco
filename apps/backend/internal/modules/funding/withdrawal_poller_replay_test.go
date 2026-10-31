package funding_test

import (
	"context"
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/funding/app"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/multichain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/solana"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
)

type hookClock struct {
	clock.Clock
	calls int
	at    int
	hook  func()
}

func (c *hookClock) Now() time.Time {
	c.calls++
	if c.calls == c.at {
		c.hook()
	}
	return c.Clock.Now()
}

type submitsThenRefuses struct {
	*privyActions
	hook func()
}

func (s submitsThenRefuses) Transfer(
	ctx context.Context, wallet string, spec multichain.TransferSpec, key string,
) (multichain.ActionID, error) {
	s.hook()
	return s.privyActions.Transfer(ctx, wallet, spec, key)
}

func (f *withdrawFixture) exec(t *testing.T, sql string, args ...any) {
	t.Helper()
	if _, err := f.pool.Exec(context.Background(), sql, args...); err != nil {
		t.Error(err)
	}
}

func TestWithdrawalPoller_StaleCreatedFailDoesNotOverwriteAReplaySubmittedMeanwhile(t *testing.T) {
	t.Parallel()
	f := newWithdrawFixture(t, 5_000_000)
	f.transfers.buildErr = errs.New(errs.CodeInternal, "test")
	if _, err := f.withdraw(t); err == nil {
		t.Fatal("withdraw err = nil")
	}
	f.exec(t, `UPDATE withdrawals SET status = 'created', fail_code = NULL, completed_at = NULL`)
	statuses := status(solana.StateFinalized, false, 800)
	hooked := &hookClock{Clock: f.clock, at: 2, hook: func() {
		f.exec(t, `UPDATE withdrawals SET status = 'submitted', signed_tx = '\x01', tx_signature = $1,
			last_valid_block_height = 900, submitted_at = now()`, string(withdrawSig))
	}}
	p := app.NewWithdrawalPoller(app.WithdrawalPollerDeps{
		UoW: f.uow, Reads: f.pool, Clock: hooked, Chain: statuses,
		Transfers: func() (app.Transfers, error) { return f.transfers, nil }, Hints: f.hints,
		UnsentAge: withdrawalUnsentAge,
	})
	f.clock.Advance(withdrawalUnsentAge + time.Second)
	if changed, err := f.tick(t, p); err != nil || changed != 0 {
		t.Fatalf("Tick = %d, %v, want the broadcast row left alone", changed, err)
	}
	if rows := f.rows(t); rows[0].status != "submitted" || rows[0].failCode != "" {
		t.Fatalf("rows = %+v, want submitted", rows)
	}
	if changed, err := f.tick(t, p); err != nil || changed != 1 {
		t.Fatalf("second Tick = %d, %v", changed, err)
	}
	if rows := f.rows(t); rows[0].status != "confirmed" {
		t.Fatalf("rows = %+v, want confirmed", rows)
	}
	if n := len(f.payloads(t, events.TypeWithdrawalConfirmed)); n != 1 {
		t.Fatalf("withdrawal.confirmed = %d, want 1", n)
	}
}

func TestWithdrawalPoller_EVMRefusalDoesNotOverwriteAReplaySubmittedMeanwhile(t *testing.T) {
	t.Parallel()
	base := &privyActions{transferErr: errs.New(errs.CodePrivyUnavailable, "privy.Transfer")}
	f := newEVMFixture(t, base)
	if _, err := f.withdrawEVM(f.ctx(t.Context())); err == nil {
		t.Fatal("Handle succeeded")
	}
	base.transferErr = errs.New(errs.CodeInvalidInput, "privy.Transfer")
	actions := submitsThenRefuses{privyActions: base, hook: func() {
		f.exec(t, `UPDATE withdrawals SET status = 'submitted', privy_action_id = 'privy-action-1',
			submitted_at = now()`)
	}}
	p := app.NewWithdrawalPoller(app.WithdrawalPollerDeps{
		UoW: f.uow, Reads: f.pool, Clock: f.clock, Chain: status(solana.StateNotFound, false, 0),
		Transfers: func() (app.Transfers, error) { return f.transfers, nil }, Hints: f.hints,
		UnsentAge: withdrawalUnsentAge, Actions: actions, TopUp: f.topUps,
		Wallets: signingWallet{wallet: chain.Wallet{ID: f.user.PrivyWalletID, Address: f.user.Address}},
	})
	f.clock.Advance(withdrawalUnsentAge + time.Second)
	if changed, err := f.tick(t, p); err != nil || changed != 0 {
		t.Fatalf("Tick = %d, %v, want the submitted row left alone", changed, err)
	}
	if row := f.evmRow(t); row.status != "submitted" || row.failCode != "" {
		t.Fatalf("row = %+v, want submitted", row)
	}
	f.assertInFlight(t, "2000000")
}
