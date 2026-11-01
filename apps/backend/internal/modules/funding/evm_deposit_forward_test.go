package funding_test

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/funding"
	"github.com/monaco/monaco/apps/backend/internal/modules/funding/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	mc "github.com/monaco/monaco/apps/backend/internal/platform/chain/multichain"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/config"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
	fakemc "github.com/monaco/monaco/apps/backend/internal/testkit/fakes/multichain"
)

const evmDepositWalletID = "privy-evm-wallet-1"

type forwardFixture struct {
	pool   *pgxpool.Pool
	user   testkit.SeededUser
	clock  *testkit.Clock
	feed   *fakemc.DepositForwarding
	logs   *testkit.Logs
	poller *app.EVMDepositForwardPoller
}

func newForwardFixture(t *testing.T, opts ...func(*app.EVMDepositForwardDeps)) *forwardFixture {
	t.Helper()
	pool := testkit.DB(t)
	f := &forwardFixture{
		pool: pool, user: testkit.SeedUser(t, pool, testkit.UserOpts{WithWallet: true}),
		clock: testkit.NewClock(clock.Real{}.Now().UTC().Truncate(time.Microsecond)),
		feed:  fakemc.New().DepositForwarding, logs: &testkit.Logs{},
	}
	if _, err := pool.Exec(t.Context(), `UPDATE user_wallets SET evm_privy_wallet_id = $2,
		evm_address = '0x5aaeb6053f3e94c9b9a09f33669435e7ef1beaed', evm_created_at = $3 WHERE user_id = $1`,
		f.user.ID.UUID(), evmDepositWalletID, f.clock.Now()); err != nil {
		t.Fatal(err)
	}
	deps := app.EVMDepositForwardDeps{
		UoW: db.New(pool, testkit.NewIDs(7), f.clock), Reads: pool, IDs: testkit.NewIDs(11), Clock: f.clock,
		Wallets: funding.EnrolledEVMWallets(identity.New(module.Deps{Pool: pool})), Feed: f.feed,
		Interval: 10 * time.Second, PageSize: app.EVMDepositWalletPage,
	}
	for _, opt := range opts {
		opt(&deps)
	}
	f.poller = app.NewEVMDepositForwardPoller(deps)
	return f
}

func (f *forwardFixture) ctx(ctx context.Context) context.Context {
	ctx = observability.WithActor(ctx, "system:poller.funding.evm_deposit_forward")
	return observability.WithLogger(ctx, observability.NewLogger(config.Config{Env: config.EnvTest}, f.logs))
}

func (f *forwardFixture) tick(t *testing.T, page ...mc.Execution) {
	t.Helper()
	if err := f.tickErr(t, page...); err != nil {
		t.Fatalf("tick: %v", err)
	}
}

func (f *forwardFixture) tickErr(t *testing.T, page ...mc.Execution) error {
	t.Helper()
	f.feed.Pages.Push(fakemc.Reply[mc.ExecutionPage]{Value: mc.ExecutionPage{Executions: page}})
	_, err := f.poller.Tick(f.ctx(t.Context()))
	return err
}

func (f *forwardFixture) count(t *testing.T, query string, args ...any) int {
	t.Helper()
	var n int
	if err := f.pool.QueryRow(t.Context(), query, args...).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func (f *forwardFixture) forwards(t *testing.T, status string) int {
	t.Helper()
	return f.count(t, `SELECT count(*) FROM evm_deposit_forwards WHERE user_id = $1 AND status = $2`,
		f.user.ID.UUID(), status)
}

func (f *forwardFixture) forwarded(t *testing.T) int {
	t.Helper()
	return f.count(t, `SELECT count(*) FROM events WHERE type = $1`, events.TypeDepositEVMForwarded)
}

func (f *forwardFixture) logged(msg observability.Msg) int {
	return bytes.Count(f.logs.Bytes(), []byte(`"msg":"`+msg.Name+`"`))
}

func execution(id string, state mc.ActionState) mc.Execution {
	ex := mc.Execution{
		ID: id, ActionID: mc.ActionID("action-" + id), Network: chain.NetworkBase, SourceTx: "0xsource-" + id,
		AmountMicros: money.MicrosFromUint64(1_000_000), State: state,
	}
	if state == mc.ActionSucceeded {
		ex.Signature = "sig-" + id
	}
	return ex
}

func TestEVMDepositForward_ReplayedExecutionsKeepOneRowAndAnnounceOnce(t *testing.T) {
	t.Parallel()
	f := newForwardFixture(t)
	f.tick(t, execution("a", mc.ActionPending))
	if got := f.forwards(t, "pending"); got != 1 || f.forwarded(t) != 0 {
		t.Fatalf("after a pending execution: %d pending rows, %d events, want 1 row and no event", got, f.forwarded(t))
	}
	f.tick(t, execution("a", mc.ActionSucceeded), execution("b", mc.ActionSucceeded))
	f.tick(t, execution("a", mc.ActionSucceeded), execution("b", mc.ActionSucceeded))
	if got := f.forwards(t, "pending"); got != 2 {
		t.Fatalf("pending rows = %d, want 2 while the Solana credit is outstanding", got)
	}
	if got := f.forwarded(t); got != 2 {
		t.Fatalf("deposit.evm_forwarded events = %d, want one per execution", got)
	}
	var signature string
	if err := f.pool.QueryRow(t.Context(), `SELECT solana_signature FROM evm_deposit_forwards
		WHERE privy_execution_id = 'a'`).Scan(&signature); err != nil || signature != "sig-a" {
		t.Fatalf("solana_signature = %q, %v, want the execution's sig-a", signature, err)
	}
	if got := f.logged(observability.FundingEVMForwardMoved); got != 2 {
		t.Fatalf("moved logs = %d, want one per new row", got)
	}
}

func TestEVMDepositForward_FailedExecutionWithoutActionReindexesOnce(t *testing.T) {
	t.Parallel()
	f := newForwardFixture(t)
	funded := unrouted("d")
	funded.AmountMicros = money.MicrosFromUint64(1_000_000)
	f.tick(t, unrouted("c"), funded)
	f.tick(t, unrouted("c"), funded)
	if got := f.feed.Reindexes.Calls(); got != 2 {
		t.Fatalf("Reindex calls = %d, want 1 per execution", got)
	}
	if got := f.logged(observability.FundingEVMForwardUnrouted); got != 2 {
		t.Fatalf("unrouted alerts = %d, want 1 per execution", got)
	}
	if got := f.forwards(t, "failed"); got != 1 || f.forwarded(t) != 0 {
		t.Fatalf("failed rows = %d, events = %d, want the funded row failed and no event", got, f.forwarded(t))
	}
	if got := f.count(t, `SELECT count(*) FROM evm_deposit_forwards WHERE privy_execution_id = 'c'`); got != 0 {
		t.Fatalf("rows for the zero-amount unrouted execution = %d, want none", got)
	}
}

func TestEVMDepositForward_StuckPendingAlertsOnce(t *testing.T) {
	t.Parallel()
	f := newForwardFixture(t)
	f.tick(t, execution("e", mc.ActionPending))
	f.clock.Advance(app.EVMDepositForwardStuckAfter - time.Second)
	f.tick(t, execution("e", mc.ActionPending))
	if got := f.logged(observability.FundingEVMForwardStuck); got != 0 {
		t.Fatalf("stuck alerts before 30 minutes = %d, want 0", got)
	}
	f.clock.Advance(2 * time.Second)
	f.tick(t, execution("e", mc.ActionPending))
	f.tick(t, execution("e", mc.ActionPending))
	if got := f.logged(observability.FundingEVMForwardStuck); got != 1 {
		t.Fatalf("stuck alerts = %d, want 1", got)
	}
}

func TestEVMDepositForward_FailedForwardAlertsAtOnceAndOnlyOnce(t *testing.T) {
	t.Parallel()
	f := newForwardFixture(t)
	f.tick(t, execution("k", mc.ActionPending))
	if got := f.logged(observability.FundingEVMForwardFailed); got != 0 {
		t.Fatalf("failed alerts for a pending forward = %d, want 0", got)
	}
	f.tick(t, execution("k", mc.ActionFailed))
	f.tick(t, execution("k", mc.ActionFailed))
	f.clock.Advance(time.Hour)
	f.tick(t, execution("k", mc.ActionFailed))
	if got := f.logged(observability.FundingEVMForwardFailed); got != 1 {
		t.Fatalf("failed alerts = %d, want 1 the moment the forward failed", got)
	}
	if got := f.logged(observability.FundingEVMForwardStuck); got != 0 {
		t.Fatalf("stuck alerts for a failed forward = %d, want 0", got)
	}
}

func TestEVMDepositForward_ExecutionOwnedByAnotherUserIsAnInternalError(t *testing.T) {
	t.Parallel()
	f := newForwardFixture(t)
	other := testkit.SeedUser(t, f.pool, testkit.UserOpts{WithWallet: true})
	if _, err := f.pool.Exec(t.Context(), `INSERT INTO evm_deposit_forwards (id, user_id, network, source_tx,
		privy_execution_id, amount_micros, status, created_at)
		VALUES ($1, $2, 'base', '0xsource', 'shared', 1000000, 'pending', now())`,
		testkit.NewIDs(23).NewV7(), other.ID.UUID()); err != nil {
		t.Fatal(err)
	}
	err := f.tickErr(t, execution("shared", mc.ActionFailed))
	if errs.CodeOf(err) != errs.CodeInternal {
		t.Fatalf("tick error = %v, want internal", err)
	}
	if got := f.count(t, `SELECT count(*) FROM evm_deposit_forwards WHERE user_id = $1 AND status = 'pending'`,
		other.ID.UUID()); got != 1 || f.forwards(t, "failed") != 0 {
		t.Fatalf("other user's pending rows = %d, want their row untouched", got)
	}
}

func TestEVMDepositForward_FailedRowStaysFailedWhenTheExecutionLaterSucceeds(t *testing.T) {
	t.Parallel()
	f := newForwardFixture(t)
	f.tick(t, execution("g", mc.ActionFailed))
	f.tick(t, execution("g", mc.ActionSucceeded))
	if got := f.forwards(t, "failed"); got != 1 || f.forwarded(t) != 0 {
		t.Fatalf("failed rows = %d, events = %d, want the row failed and no event", got, f.forwarded(t))
	}
	if got := f.feed.Reindexes.Calls(); got != 0 {
		t.Fatalf("Reindex calls = %d, want none for a failure that has an action", got)
	}
}

func TestEVMDepositForward_ReadFailuresFailTheTickAsInternal(t *testing.T) {
	t.Parallel()
	f := newForwardFixture(t, func(d *app.EVMDepositForwardDeps) { d.Reads = closedPool(t, d.Reads.(*pgxpool.Pool)) })
	err := f.tickErr(t, unrouted("h"))
	if errs.CodeOf(err) != errs.CodeInternal || !strings.Contains(err.Error(), "funding.EVMDepositForward.status") ||
		!strings.Contains(err.Error(), "funding.EVMDepositForward.stuck") ||
		!strings.Contains(err.Error(), "funding.EVMDepositForward.failed") {
		t.Fatalf("tick error = %v, want internal status and stuck read failures", err)
	}
	if got := f.feed.Reindexes.Calls(); got != 0 {
		t.Fatalf("Reindex calls = %d, want none when the status read fails", got)
	}
}

type listedWallets struct {
	wallets []app.EVMDepositWallet
	err     error
}

func (l listedWallets) EVMDepositWallets(context.Context, ids.UserID, int) ([]app.EVMDepositWallet, error) {
	return l.wallets, l.err
}

func TestEVMDepositForward_PagesThroughEveryEnrolledWallet(t *testing.T) {
	t.Parallel()
	f := newForwardFixture(t, func(d *app.EVMDepositForwardDeps) { d.PageSize = 1 })
	second := testkit.SeedUser(t, f.pool, testkit.UserOpts{WithWallet: true})
	if _, err := f.pool.Exec(t.Context(), `UPDATE user_wallets SET evm_privy_wallet_id = 'privy-evm-wallet-2',
		evm_address = '0x00000000000000000000000000000000000000aa', evm_created_at = now() WHERE user_id = $1`,
		second.ID.UUID()); err != nil {
		t.Fatal(err)
	}
	f.feed.Pages.Push(fakemc.Reply[mc.ExecutionPage]{Value: mc.ExecutionPage{
		Executions: []mc.Execution{execution("j1", mc.ActionPending)},
	}})
	f.tick(t, execution("j2", mc.ActionPending))
	if got := f.count(t, `SELECT count(*) FROM evm_deposit_forwards WHERE status = 'pending'`); got != 2 {
		t.Fatalf("pending rows = %d, want one per enrolled wallet", got)
	}
}

func TestEVMDepositForward_FollowsTheExecutionCursor(t *testing.T) {
	t.Parallel()
	f := newForwardFixture(t)
	f.feed.Pages.Push(fakemc.Reply[mc.ExecutionPage]{Value: mc.ExecutionPage{
		Executions: []mc.Execution{execution("k1", mc.ActionPending)}, Next: "page-2",
	}})
	f.tick(t, execution("k2", mc.ActionPending))
	if got := f.forwards(t, "pending"); got != 2 {
		t.Fatalf("pending rows = %d, want both pages recorded", got)
	}
}

func unrouted(id string) mc.Execution {
	ex := execution(id, mc.ActionFailed)
	ex.ActionID = ""
	ex.AmountMicros = money.Micros{}
	return ex
}

func TestEVMDepositForward_FailuresFailTheTickAndRecordNothing(t *testing.T) {
	t.Parallel()
	orphan := app.EVMDepositWallet{UserID: ids.UserIDFrom(testkit.NewIDs(29).NewV7()), WalletID: "orphan"}
	solana, zero := execution("n2", mc.ActionPending), execution("n3", mc.ActionPending)
	solana.Network, zero.AmountMicros = chain.NetworkSolana, money.Micros{}
	for name, tc := range map[string]struct {
		deps func(*app.EVMDepositForwardDeps)
		feed func(*fakemc.DepositForwarding)
		page []mc.Execution
		want errs.Code
	}{
		"wallet list": {deps: func(d *app.EVMDepositForwardDeps) {
			d.Wallets = listedWallets{err: errs.New(errs.CodeDBUnavailable, "test.EVMDepositWallets")}
		}, want: errs.CodeDBUnavailable},
		"executions": {feed: func(fd *fakemc.DepositForwarding) {
			fd.Pages.Push(fakemc.Reply[mc.ExecutionPage]{Code: errs.CodePrivyUnavailable})
		}, want: errs.CodePrivyUnavailable},
		"reindex": {feed: func(fd *fakemc.DepositForwarding) {
			fd.Reindexes.Push(fakemc.Reply[struct{}]{Code: errs.CodePrivyUnavailable})
		}, page: []mc.Execution{unrouted("n0")}, want: errs.CodePrivyUnavailable},
		"unknown user": {deps: func(d *app.EVMDepositForwardDeps) {
			d.Wallets = listedWallets{wallets: []app.EVMDepositWallet{orphan}}
		}, page: []mc.Execution{execution("n1", mc.ActionPending)}, want: errs.CodeInternal},
		"missing id": {page: []mc.Execution{execution("", mc.ActionPending)}, want: errs.CodeDecodeFailed},
		"solana":     {page: []mc.Execution{solana}, want: errs.CodeDecodeFailed},
		"zero":       {page: []mc.Execution{zero}, want: errs.CodeDecodeFailed},
		"state":      {page: []mc.Execution{execution("n4", "queued")}, want: errs.CodeDecodeFailed},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			var opts []func(*app.EVMDepositForwardDeps)
			if tc.deps != nil {
				opts = append(opts, tc.deps)
			}
			f := newForwardFixture(t, opts...)
			if tc.feed != nil {
				tc.feed(f.feed)
			}
			f.feed.Pages.Push(fakemc.Reply[mc.ExecutionPage]{Value: mc.ExecutionPage{Executions: tc.page}})
			if _, err := f.poller.Tick(f.ctx(t.Context())); errs.CodeOf(err) != tc.want {
				t.Fatalf("tick error = %v, want %s", err, tc.want)
			}
			if got := f.count(t, `SELECT count(*) FROM evm_deposit_forwards`); got != 0 {
				t.Fatalf("forward rows = %d, want none", got)
			}
		})
	}
}
