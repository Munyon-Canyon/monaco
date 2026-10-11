package funding_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/funding/app"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/multichain"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

func TestWithdrawEVM_InsufficientFundsSendsNothing(t *testing.T) {
	t.Parallel()
	actions := &privyActions{}
	f := newWithdrawFixture(t, 1_000_000, func(d *app.WithdrawDeps) { d.Actions = actions })
	if _, err := f.withdrawEVM(f.ctx(t.Context())); errs.CodeOf(err) != errs.CodeInsufficientFunds {
		t.Fatalf("Handle err = %v, want insufficient_funds", err)
	}
	if rows := f.rows(t); len(rows) != 0 || len(actions.calls) != 0 {
		t.Fatalf("rows = %+v transfers = %d", rows, len(actions.calls))
	}
}

type racingActions struct {
	*privyActions
	during func()
}

func (r racingActions) Transfer(
	ctx context.Context, wallet string, spec multichain.TransferSpec, key string,
) (multichain.ActionID, error) {
	id, err := r.privyActions.Transfer(ctx, wallet, spec, key)
	if r.during != nil {
		r.during()
	}
	return id, err
}

func TestWithdrawEVM_APollerRecoveringASlowCreateSubmitsOnce(t *testing.T) {
	t.Parallel()
	actions := &privyActions{}
	racing := &racingActions{privyActions: actions}
	f := newWithdrawFixture(t, 5_000_000, func(d *app.WithdrawDeps) { d.Actions = racing })
	racing.during = func() {
		racing.during = nil
		f.clock.Advance(withdrawalUnsentAge + time.Second)
		if changed, err := f.tick(t, f.evmPoller(actions)); err != nil || changed != 1 {
			t.Errorf("Tick during the create = %d, %v", changed, err)
		}
	}
	got, err := f.withdrawEVM(f.ctx(t.Context()))
	if err != nil || got.Status != "submitted" {
		t.Fatalf("Handle = %+v, %v", got, err)
	}
	assertOneAction(t, f, actions)
}

func TestWithdrawEVM_APollerSkipsAnUndecodableRow(t *testing.T) {
	t.Parallel()
	actions := &privyActions{}
	f := newEVMFixture(t, actions)
	const zero = "0x0000000000000000000000000000000000000000"
	if _, err := f.pool.Exec(t.Context(), `INSERT INTO withdrawals (id, user_id, amount_micros, to_address, status,
		created_at, destination_kind, network, evm_address) VALUES ($1, $2, 2000000, $3, 'created', $4, 'evm', 'base', $3)`,
		ids.Real{}.NewV7(), f.user.ID.UUID(), zero, f.now); err != nil {
		t.Fatal(err)
	}
	f.clock.Advance(withdrawalUnsentAge + time.Second)
	if _, err := f.tick(t, f.evmPoller(actions)); errs.CodeOf(err) != errs.CodeDecodeFailed {
		t.Fatalf("Tick = %v, want decode_failed", err)
	}
	if row := f.evmRow(t); row.status != "created" || len(actions.calls) != 0 {
		t.Fatalf("row = %+v transfers = %d", row, len(actions.calls))
	}
}

func TestWithdrawEVM_APollerRefusesAnUnexpectedAction(t *testing.T) {
	t.Parallel()
	for name, status := range map[string]multichain.ActionStatus{
		"succeeded without a solana step": {State: multichain.ActionSucceeded},
		"unknown state":                   {State: "paused"},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			actions := &privyActions{status: status}
			f := newEVMFixture(t, actions)
			if _, err := f.withdrawEVM(f.ctx(t.Context())); err != nil {
				t.Fatal(err)
			}
			if _, err := f.tick(t, f.evmPoller(actions)); errs.CodeOf(err) != errs.CodeDecodeFailed {
				t.Fatalf("Tick = %v, want decode_failed", err)
			}
			if row := f.evmRow(t); row.status != "submitted" {
				t.Fatalf("row = %+v", row)
			}
		})
	}
}

type evmPair struct {
	f       *withdrawFixture
	actions *privyActions
}

func newEVMPair(t *testing.T) evmPair {
	t.Helper()
	actions := &privyActions{}
	f := newEVMFixture(t, actions)
	if _, err := f.withdrawEVM(f.ctx(t.Context())); err != nil {
		t.Fatal(err)
	}
	actions.transferErr = errs.New(errs.CodePrivyUnavailable, "privy.Transfer")
	if _, err := f.withdrawEVM(f.ctx(t.Context())); err == nil {
		t.Fatal("second Handle succeeded")
	}
	actions.transferErr = nil
	actions.status = multichain.ActionStatus{State: multichain.ActionSucceeded, Steps: []multichain.ActionStep{
		{Network: chain.NetworkSolana, TxHash: string(evmSourceSig), State: multichain.StepConfirmed},
	}}
	f.clock.Advance(withdrawalUnsentAge + time.Second)
	return evmPair{f: f, actions: actions}
}

func (e evmPair) assertUntouched(t *testing.T) {
	t.Helper()
	var statuses []string
	rows, err := e.f.pool.Query(t.Context(), `SELECT status || '/' || COALESCE(tx_signature, '')
		FROM withdrawals ORDER BY created_at, id`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			t.Fatal(err)
		}
		statuses = append(statuses, s)
	}
	if len(statuses) != 2 || statuses[0] != "submitted/" || statuses[1] != "created/" {
		t.Fatalf("withdrawals = %v", statuses)
	}
	e.f.assertInFlight(t, "4000000")
}

func TestWithdrawEVM_APollerWaitsWhenTheWalletLookupFails(t *testing.T) {
	t.Parallel()
	e := newEVMPair(t)
	boom := errs.New(errs.CodeInternal, "identity.MemberWallet")
	p := app.NewWithdrawalPoller(app.WithdrawalPollerDeps{
		UoW: e.f.uow, Reads: e.f.pool, Clock: e.f.clock, Hints: e.f.hints, UnsentAge: withdrawalUnsentAge,
		Actions: e.actions, Wallets: signingWallet{err: boom},
	})
	if _, err := e.f.tick(t, p); !errors.Is(err, boom) {
		t.Fatalf("Tick = %v, want %v", err, boom)
	}
	if len(e.actions.calls) != 2 {
		t.Fatalf("transfers = %d, want the two from Handle", len(e.actions.calls))
	}
	e.assertUntouched(t)
}

type failingActions struct {
	*privyActions
	err error
}

func (f failingActions) Action(context.Context, string, multichain.ActionID) (multichain.ActionStatus, error) {
	return multichain.ActionStatus{}, f.err
}

func TestWithdrawEVM_APollerWaitsWhenPrivyCannotReadTheAction(t *testing.T) {
	t.Parallel()
	e := newEVMPair(t)
	boom := errs.New(errs.CodePrivyUnavailable, "privy.Action")
	p := app.NewWithdrawalPoller(app.WithdrawalPollerDeps{
		UoW: e.f.uow, Reads: e.f.pool, Clock: e.f.clock, Hints: e.f.hints, UnsentAge: withdrawalUnsentAge,
		Actions: failingActions{privyActions: e.actions, err: boom},
		Wallets: signingWallet{wallet: chain.Wallet{ID: e.f.user.PrivyWalletID, Address: e.f.user.Address}},
	})
	if changed, err := e.f.tick(t, p); !errors.Is(err, boom) || changed != 1 {
		t.Fatalf("Tick = %d, %v, want the created row recovered and %v", changed, err, boom)
	}
}

func TestWithdrawEVM_APollerWaitsWhenItCannotStore(t *testing.T) {
	t.Parallel()
	e := newEVMPair(t)
	p := app.NewWithdrawalPoller(app.WithdrawalPollerDeps{
		UoW: db.New(closedPool(t, e.f.pool), testkit.NewIDs(6), e.f.clock), Reads: e.f.pool, Clock: e.f.clock,
		Hints: e.f.hints, UnsentAge: withdrawalUnsentAge, Actions: e.actions,
		Wallets: signingWallet{wallet: chain.Wallet{ID: e.f.user.PrivyWalletID, Address: e.f.user.Address}},
	})
	if changed, err := e.f.tick(t, p); err == nil || changed != 0 {
		t.Fatalf("Tick = %d, %v, want a store error", changed, err)
	}
	e.assertUntouched(t)
}
