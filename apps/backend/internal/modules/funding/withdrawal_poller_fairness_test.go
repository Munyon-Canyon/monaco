package funding_test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/funding/app"
	fundingsqlc "github.com/monaco/monaco/apps/backend/internal/modules/funding/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/multichain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/solana"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
	"github.com/monaco/monaco/apps/backend/internal/platform/poller"
)

const (
	stuckWithdrawals = 100
	fairnessTicks    = 8
)

type scriptedActions struct {
	*privyActions
	statuses map[multichain.ActionID]multichain.ActionStatus
	hang     map[multichain.ActionID]bool
	onlySend string
	onCall   func()
}

type noExec struct{ fundingsqlc.DBTX }

func (noExec) Exec(context.Context, string, ...any) (pgconn.CommandTag, error) {
	return pgconn.CommandTag{}, errNoExec
}

var errNoExec = errs.New(errs.CodeInternal, "test.noExec")

func (a scriptedActions) Transfer(
	ctx context.Context, wallet string, spec multichain.TransferSpec, key string,
) (multichain.ActionID, error) {
	if a.onCall != nil {
		a.onCall()
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if a.onlySend != "" && key != a.onlySend {
		return "", errs.New(errs.CodePrivyUnavailable, "privy.Transfer")
	}
	return a.privyActions.Transfer(ctx, wallet, spec, key)
}

func (a scriptedActions) Action(
	ctx context.Context, _ string, id multichain.ActionID,
) (multichain.ActionStatus, error) {
	if a.onCall != nil {
		a.onCall()
	}
	if err := ctx.Err(); err != nil {
		return multichain.ActionStatus{}, err
	}
	if a.hang[id] {
		<-ctx.Done()
		return multichain.ActionStatus{}, ctx.Err()
	}
	s := a.statuses[id]
	s.ID = id
	return s, nil
}

func sentThen(state multichain.ActionState, hash string) multichain.ActionStatus {
	return multichain.ActionStatus{State: state, Steps: []multichain.ActionStep{
		{Network: chain.NetworkSolana, TxHash: hash, State: multichain.StepConfirmed},
	}}
}

func sentAndSucceeded() multichain.ActionStatus {
	return sentThen(multichain.ActionSucceeded, "healthy-source-signature")
}

type fairnessFixture struct {
	*withdrawFixture
	actions scriptedActions
	healthy multichain.ActionID
}

func newFairnessFixture(t *testing.T) *fairnessFixture {
	t.Helper()
	f := &fairnessFixture{
		withdrawFixture: newWithdrawFixture(t, 5_000_000),
		actions: scriptedActions{
			privyActions: &privyActions{},
			statuses:     map[multichain.ActionID]multichain.ActionStatus{},
			hang:         map[multichain.ActionID]bool{},
		},
		healthy: "healthy-action",
	}
	f.seedSubmitted(t, "stuck", stuckWithdrawals, -2*time.Hour)
	f.seedSubmitted(t, "healthy", 1, -time.Hour)
	f.actions.statuses[f.healthy] = multichain.ActionStatus{State: multichain.ActionPending}
	for i := range stuckWithdrawals {
		f.actions.statuses[stuckAction(i)] = sentThen(multichain.ActionFailed, fmt.Sprintf("stuck-source-%03d", i))
	}
	return f
}

func stuckAction(i int) multichain.ActionID {
	return multichain.ActionID(fmt.Sprintf("stuck-action-%03d", i))
}

func (f *fairnessFixture) seedSubmitted(t *testing.T, kind string, n int, age time.Duration) {
	t.Helper()
	start := f.clock.Now().Add(age)
	for i := range n {
		action := string(f.healthy)
		if kind == "stuck" {
			action = string(stuckAction(i))
		}
		at := start.Add(time.Duration(i) * time.Second)
		if _, err := f.pool.Exec(t.Context(), `INSERT INTO withdrawals (id, user_id, amount_micros, to_address, status,
			created_at, submitted_at, destination_kind, network, evm_address, privy_action_id)
			VALUES ($1, $2, 2000000, $3, 'submitted', $4, $4, 'evm', 'base', $3, $5)`,
			ids.Real{}.NewV7(), f.user.ID.UUID(), evmTo, at, action); err != nil {
			t.Fatal(err)
		}
	}
}

func (f *fairnessFixture) newPoller(itemBudget time.Duration) *app.WithdrawalPoller {
	return f.newPollerReading(f.pool, itemBudget)
}

func (f *fairnessFixture) newPollerReading(reads fundingsqlc.DBTX, itemBudget time.Duration) *app.WithdrawalPoller {
	return app.NewWithdrawalPoller(app.WithdrawalPollerDeps{
		UoW: f.uow, Reads: reads, Clock: f.clock, Chain: status(solana.StateNotFound, false, 0),
		Transfers: func() (app.Transfers, error) { return f.transfers, nil }, Hints: f.hints,
		UnsentAge: withdrawalUnsentAge, Actions: f.actions, TopUp: f.topUps, ItemBudget: itemBudget,
		Wallets: signingWallet{wallet: chain.Wallet{ID: f.user.PrivyWalletID, Address: f.user.Address}},
	})
}

func (f *fairnessFixture) tickWithinBudget(t *testing.T, p *app.WithdrawalPoller) error {
	t.Helper()
	ctx, cancel := context.WithTimeout(observability.WithActor(t.Context(), "system:poller.funding.withdrawals"),
		poller.TickBudget(p))
	defer cancel()
	_, err := p.Tick(ctx)
	f.clock.Advance(app.WithdrawalPollInterval)
	if ctx.Err() != nil {
		return nil
	}
	return err
}

func (f *fairnessFixture) tickUntilConfirmed(t *testing.T, p *app.WithdrawalPoller) {
	t.Helper()
	for range fairnessTicks {
		if err := f.tickWithinBudget(t, p); err != nil {
			t.Fatal(err)
		}
		if f.healthyStatus(t) == "confirmed" {
			return
		}
	}
	t.Fatalf("healthy withdrawal is %s after %d ticks, want confirmed", f.healthyStatus(t), fairnessTicks)
}

func (f *fairnessFixture) healthyStatus(t *testing.T) string {
	t.Helper()
	var s string
	if err := f.pool.QueryRow(t.Context(),
		`SELECT status FROM withdrawals WHERE privy_action_id = $1`, string(f.healthy)).Scan(&s); err != nil {
		t.Fatal(err)
	}
	return s
}

func (f *fairnessFixture) assertStuckKeepTheirReservations(t *testing.T) {
	t.Helper()
	var submitted int
	if err := f.pool.QueryRow(t.Context(),
		`SELECT count(*) FROM withdrawals WHERE status = 'submitted' AND privy_action_id LIKE 'stuck-action-%'`).
		Scan(&submitted); err != nil || submitted != stuckWithdrawals {
		t.Fatalf("stuck submitted = %d, %v, want %d", submitted, err, stuckWithdrawals)
	}
	f.assertInFlight(t, "200000000")
}

func TestWithdrawalPoller_aHundredStuckEVMWithdrawalsDoNotLockOutAHealthyOne(t *testing.T) {
	t.Parallel()
	f := newFairnessFixture(t)
	f.actions.statuses[f.healthy] = sentAndSucceeded()
	f.tickUntilConfirmed(t, f.newPoller(0))
	f.assertStuckKeepTheirReservations(t)
}

func TestWithdrawalPoller_anEVMWithdrawalThatSucceedsLaterGetsItsTurnAfterARestart(t *testing.T) {
	t.Parallel()
	f := newFairnessFixture(t)
	p := f.newPoller(0)
	for range 2 {
		if err := f.tickWithinBudget(t, p); err != nil {
			t.Fatal(err)
		}
	}
	if got := f.healthyStatus(t); got != "submitted" {
		t.Fatalf("healthy withdrawal = %s while its action is pending, want submitted", got)
	}
	f.actions.statuses[f.healthy] = sentAndSucceeded()
	f.tickUntilConfirmed(t, f.newPoller(0))
	f.assertStuckKeepTheirReservations(t)
}

func TestWithdrawalPoller_aSlowEVMWithdrawalIsCutOffAndTheRestStillSettle(t *testing.T) {
	t.Parallel()
	f := newFairnessFixture(t)
	f.actions.statuses[f.healthy] = sentAndSucceeded()
	for i := range stuckWithdrawals {
		f.actions.hang[stuckAction(i)] = i == 0
	}
	p := f.newPoller(time.Second)
	for range fairnessTicks {
		if f.healthyStatus(t) == "confirmed" {
			break
		}
		_ = f.tickWithinBudget(t, p)
	}
	if got := f.healthyStatus(t); got != "confirmed" {
		t.Fatalf("healthy withdrawal = %s, want confirmed", got)
	}
}

func TestWithdrawalPoller_aHundredStuckUnsentEVMWithdrawalsDoNotLockOutAHealthyOne(t *testing.T) {
	t.Parallel()
	f := newFairnessFixture(t)
	healthy := ids.Real{}.NewV7()
	start := f.clock.Now().Add(-3 * time.Hour)
	for i := range stuckWithdrawals + 1 {
		id, at := ids.Real{}.NewV7(), start.Add(time.Duration(i)*time.Second)
		if i == stuckWithdrawals {
			id = healthy
		}
		if _, err := f.pool.Exec(t.Context(), `INSERT INTO withdrawals (id, user_id, amount_micros, to_address, status,
			created_at, destination_kind, network, evm_address) VALUES ($1, $2, 2000000, $3, 'created', $4, 'evm',
			'base', $3)`, id, f.user.ID.UUID(), evmTo, at); err != nil {
			t.Fatal(err)
		}
	}
	f.actions.onlySend = healthy.String()
	p := f.newPoller(0)
	for range fairnessTicks {
		_ = f.tickWithinBudget(t, p)
	}
	var got string
	err := f.pool.QueryRow(t.Context(), `SELECT status FROM withdrawals WHERE id = $1`, healthy).Scan(&got)
	if err != nil {
		t.Fatal(err)
	}
	if got != "submitted" {
		t.Fatalf("healthy unsent withdrawal = %s after %d ticks, want submitted", got, fairnessTicks)
	}
}

func (f *fairnessFixture) attempted(t *testing.T) int {
	t.Helper()
	var n int
	if err := f.pool.QueryRow(t.Context(), `SELECT count(*) FROM withdrawals WHERE last_attempt_at IS NOT NULL`).
		Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func (f *fairnessFixture) systemCtx(t *testing.T) (context.Context, context.CancelFunc) {
	t.Helper()
	return context.WithCancel(observability.WithActor(t.Context(), "system:poller.funding.withdrawals"))
}

func TestWithdrawalPoller_aTickOutOfTimeStopsWithoutAnErrorAndKeepsTheRestInLine(t *testing.T) {
	t.Parallel()
	f := newFairnessFixture(t)
	ctx, cancel := f.systemCtx(t)
	defer cancel()
	f.actions.onCall = cancel
	if _, err := f.newPoller(0).Tick(ctx); err != nil {
		t.Fatalf("Tick out of time = %v, want nil", err)
	}
	if got := f.attempted(t); got != 1 {
		t.Fatalf("rows stamped = %d, want only the one being worked", got)
	}
}

func TestWithdrawalPoller_aTickOutOfTimeStopsRecoveringUnsentRows(t *testing.T) {
	t.Parallel()
	f := newFairnessFixture(t)
	for i := range 3 {
		if _, err := f.pool.Exec(t.Context(), `INSERT INTO withdrawals (id, user_id, amount_micros, to_address, status,
			created_at, destination_kind, network, evm_address) VALUES ($1, $2, 2000000, $3, 'created', $4, 'evm',
			'base', $3)`, ids.Real{}.NewV7(), f.user.ID.UUID(), evmTo,
			f.clock.Now().Add(-3*time.Hour+time.Duration(i)*time.Second)); err != nil {
			t.Fatal(err)
		}
	}
	ctx, cancel := f.systemCtx(t)
	defer cancel()
	f.actions.onCall = cancel
	if _, err := f.newPoller(0).Tick(ctx); err != nil {
		t.Fatalf("Tick out of time = %v, want nil", err)
	}
	if got := f.attempted(t); got != 1 {
		t.Fatalf("rows stamped = %d, want only the one being worked", got)
	}
}

func TestWithdrawalPoller_aRowItCannotStampIsNotWorked(t *testing.T) {
	t.Parallel()
	f := newFairnessFixture(t)
	f.actions.onCall = func() { t.Error("a row was worked without its turn being recorded") }
	ctx, cancel := f.systemCtx(t)
	defer cancel()
	if _, err := f.newPollerReading(noExec{f.pool}, 0).Tick(ctx); !errors.Is(err, errNoExec) {
		t.Fatalf("Tick = %v, want %v", err, errNoExec)
	}
}
