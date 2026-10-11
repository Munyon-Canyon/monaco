package funding_test

import (
	"context"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/funding"
	"github.com/monaco/monaco/apps/backend/internal/modules/funding/adapters"
	"github.com/monaco/monaco/apps/backend/internal/modules/funding/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/funding/port"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury"
	"github.com/monaco/monaco/apps/backend/internal/platform/auth"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	mc "github.com/monaco/monaco/apps/backend/internal/platform/chain/multichain"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/config"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpclient"
	api "github.com/monaco/monaco/apps/backend/internal/platform/httpx/api/fundingapi"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
	"github.com/monaco/monaco/apps/backend/internal/testkit/fakes"
	"github.com/monaco/monaco/apps/backend/internal/testkit/flows"
	"github.com/monaco/monaco/apps/backend/internal/testkit/scenario"
)

type steadyForwarding struct {
	mc.Pending
	page mc.ExecutionPage
}

func (f steadyForwarding) Executions(context.Context, string, string) (mc.ExecutionPage, error) {
	return f.page, nil
}

func TestF30Step_EVMDepositCreditedOnceAndArrivingDropsToZero(t *testing.T) {
	t.Parallel()
	execution := flows.ForwardedEVMDepositExecution()
	forwarding := steadyForwarding{page: mc.ExecutionPage{Executions: []mc.Execution{execution}}}
	upstreams := fakes.New()
	srv := httptest.NewServer(upstreams)
	t.Cleanup(srv.Close)
	s := scenario.New(t,
		scenario.WithPrivy(upstreams, "app"),
		scenario.WithModules(func(d module.Deps) module.Module {
			d.Config = config.Config{
				Solana: config.Solana{RPCURL: srv.URL + "/rpc/", USDCMint: string(testkit.USDCMint)},
				Funding: config.Funding{
					DepositPollInterval: app.DepositPollInterval, DepositRPCRate: 1000, DepositTickBudget: 480,
					DepositRotation: 6 * time.Hour, DepositRecoverySlots: 216000, DepositDiscovery: 6 * time.Hour,
					EVMForwardInterval: 10 * time.Second,
				},
				Timeouts: config.Timeouts{RPC: time.Second},
			}
			d.HTTPClient = httpclient.New
			return funding.New(d, funding.WithDepositForwarding(forwarding))
		}, func(d module.Deps) module.Module {
			d.Config.Solana.USDCMint = string(testkit.USDCMint)
			return treasury.New(d)
		}),
	)
	for _, step := range flows.Steps() {
		if step.Flow == "30" && step.Name == "EVM deposit credited" {
			step.Script(s)
			return
		}
	}
	t.Fatal("flow 30 has no EVM deposit credited step")
}

const creditMicros = 5_000_000

type arrivalFixture struct {
	pool *pgxpool.Pool
	user testkit.SeededUser
	now  time.Time
	uow  *db.UnitOfWork
	ids  *testkit.IDs
}

func newArrivalFixture(t *testing.T) *arrivalFixture {
	t.Helper()
	pool := testkit.DB(t)
	now := clock.Real{}.Now().UTC().Truncate(time.Microsecond)
	return &arrivalFixture{
		pool: pool, user: testkit.SeedUser(t, pool, testkit.UserOpts{WithWallet: true}), now: now,
		uow: db.New(pool, testkit.NewIDs(13), testkit.NewClock(now)), ids: testkit.NewIDs(17),
	}
}

func (f *arrivalFixture) forward(
	t *testing.T, execution string, micros uint64, age time.Duration, sig string,
) uuid.UUID {
	t.Helper()
	id := f.ids.NewV7()
	var signature any
	if sig != "" {
		signature = sig
	}
	if _, err := f.pool.Exec(t.Context(), `INSERT INTO evm_deposit_forwards (id, user_id, network, source_tx,
		privy_execution_id, amount_micros, status, solana_signature, created_at)
		VALUES ($1, $2, 'base', '0xsource', $3, $4, 'pending', $5, $6)`,
		id, f.user.ID.UUID(), execution, micros, signature, f.now.Add(-age)); err != nil {
		t.Fatal(err)
	}
	return id
}

func (f *arrivalFixture) credit(t *testing.T, sig string) {
	t.Helper()
	e := events.DepositCredited{
		V: 1, DepositID: f.ids.NewV7(), UserID: f.user.ID.UUID(), WalletAddress: f.user.Address,
		AmountMicros: money.MicrosFromUint64(creditMicros), TxSignature: chain.Signature(sig),
	}
	if err := f.uow.Do(t.Context(), func(ctx context.Context, tx db.Tx) error {
		return adapters.EVMDepositArrival{}.Handle(ctx, tx, e, f.now)
	}); err != nil {
		t.Fatal(err)
	}
}

func (f *arrivalFixture) status(t *testing.T, id uuid.UUID) string {
	t.Helper()
	var status string
	if err := f.pool.QueryRow(t.Context(), `SELECT status FROM evm_deposit_forwards WHERE id = $1`, id).
		Scan(&status); err != nil {
		t.Fatal(err)
	}
	return status
}

func (f *arrivalFixture) arriving(t *testing.T) string {
	t.Helper()
	got, err := app.EVMDepositArrivals{Reads: f.pool}.ArrivingMicros(t.Context(), f.user.ID)
	if err != nil {
		t.Fatal(err)
	}
	return got.String()
}

func TestEVMDepositArrival_MatchesBySignatureBeforeAmount(t *testing.T) {
	t.Parallel()
	f := newArrivalFixture(t)
	byAmount := f.forward(t, "older-same-amount", creditMicros, time.Hour, "")
	bySignature := f.forward(t, "signed", creditMicros, time.Minute, "sig-1")
	if got := f.arriving(t); got != "10000000" {
		t.Fatalf("arriving = %s, want both forwards in flight", got)
	}
	f.credit(t, "sig-1")
	if f.status(t, bySignature) != "arrived" || f.status(t, byAmount) != "pending" {
		t.Fatalf("signed forward %s, unsigned %s, want the signature match to arrive alone",
			f.status(t, bySignature), f.status(t, byAmount))
	}
	f.credit(t, "sig-2")
	if got := f.status(t, byAmount); got != "arrived" {
		t.Fatalf("unsigned forward = %s, want arrived by member wallet and amount", got)
	}
	if got := f.arriving(t); got != "0" {
		t.Fatalf("arriving = %s, want 0 once both credits landed", got)
	}
}

func TestEVMDepositArrival_LeavesForwardsThatDoNotMatch(t *testing.T) {
	t.Parallel()
	f := newArrivalFixture(t)
	otherAmount := f.forward(t, "other-amount", 7_000_000, time.Minute, "")
	stale := f.forward(t, "stale", creditMicros, adapters.EVMDepositArrivalWindow+time.Minute, "")
	otherSignature := f.forward(t, "other-signature", creditMicros, time.Minute, "sig-other")
	f.credit(t, "sig-new")
	for name, id := range map[string]uuid.UUID{"amount": otherAmount, "stale": stale, "signature": otherSignature} {
		if got := f.status(t, id); got != "pending" {
			t.Errorf("forward with a different %s = %s, want pending", name, got)
		}
	}
}

func TestEVMDepositArrival_ReplayedCreditClosesOneForward(t *testing.T) {
	t.Parallel()
	f := newArrivalFixture(t)
	first := f.forward(t, "first", creditMicros, 2*time.Minute, "")
	second := f.forward(t, "second", creditMicros, time.Minute, "")
	f.credit(t, "sig-once")
	f.credit(t, "sig-once")
	if f.status(t, first) != "arrived" || f.status(t, second) != "pending" {
		t.Fatalf("first %s, second %s, want one credit to close only the oldest forward",
			f.status(t, first), f.status(t, second))
	}
	if got := f.arriving(t); got != "5000000" {
		t.Fatalf("arriving = %s, want the second forward still in flight", got)
	}
}

type displayBalance struct{}

func (displayBalance) ForDisplay(context.Context, ids.UserID) (port.Balance, error) {
	return port.Balance{}, nil
}

type memberWallet struct{}

func (memberWallet) MemberWalletAddress(context.Context, ids.UserID) (chain.SolanaAddress, error) {
	return "wallet", nil
}

type failingArrivals struct{}

func (failingArrivals) ArrivingMicros(context.Context, ids.UserID) (money.Micros, error) {
	return money.Micros{}, errs.New(errs.CodeDBUnavailable, "test.ArrivingMicros")
}

func TestHTTPGetMyBalance_OmitsArrivingWhenItIsUnwiredOrUnreadable(t *testing.T) {
	t.Parallel()
	user := ids.UserIDFrom(ids.Real{}.NewV7())
	ctx := auth.WithActor(t.Context(), auth.Actor{Kind: auth.ActorUser, ID: user.String()})
	for name, arriving := range map[string]app.ArrivingDeposits{"unwired": nil, "unreadable": failingArrivals{}} {
		h := adapters.HTTP{Balances: displayBalance{}, Wallets: memberWallet{}, Arriving: arriving}
		got, err := h.GetMyBalance(ctx, api.GetMyBalanceRequestObject{})
		if err != nil {
			t.Fatalf("%s: GetMyBalance error = %v, want the balance without arriving_micros", name, err)
		}
		if value := got.(api.GetMyBalance200JSONResponse).ArrivingMicros; value != nil {
			t.Fatalf("%s: arriving_micros = %q, want absent", name, *value)
		}
	}
}

func TestEVMDepositArrivals_ReadAndDecodeFailures(t *testing.T) {
	t.Parallel()
	f := newArrivalFixture(t)
	closed := app.EVMDepositArrivals{Reads: closedPool(t, f.pool)}
	if _, err := closed.ArrivingMicros(t.Context(), f.user.ID); errs.CodeOf(err) != errs.CodeInternal {
		t.Fatalf("closed pool: ArrivingMicros error = %v, want internal", err)
	}
	f.forward(t, "beyond-uint64", creditMicros, time.Minute, "")
	if _, err := f.pool.Exec(t.Context(), `UPDATE evm_deposit_forwards SET amount_micros = 99999999999999999999
		WHERE privy_execution_id = 'beyond-uint64'`); err != nil {
		t.Fatal(err)
	}
	arrivals := app.EVMDepositArrivals{Reads: f.pool}
	if _, err := arrivals.ArrivingMicros(t.Context(), f.user.ID); errs.CodeOf(err) != errs.CodeDecodeFailed {
		t.Fatalf("sum beyond uint64: ArrivingMicros error = %v, want decode_failed", err)
	}
}
