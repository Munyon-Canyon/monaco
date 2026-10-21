package funding_test

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/monaco/monaco/apps/backend/internal/modules/cabal"
	"github.com/monaco/monaco/apps/backend/internal/modules/funding"
	"github.com/monaco/monaco/apps/backend/internal/modules/funding/adapters"
	"github.com/monaco/monaco/apps/backend/internal/modules/funding/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/funding/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/funding/port"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/relayer"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/solana"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	dbsqlc "github.com/monaco/monaco/apps/backend/internal/platform/db/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

type gatedChain struct{ gate *testkit.Gate }

func (c gatedChain) TokenBalancesAt(
	context.Context, chain.SolanaAddress, chain.Mint, string,
) (solana.TokenBalances, error) {
	c.gate.Arrive()
	held := money.NewBaseUnits(50_000_000, 6)
	return solana.TokenBalances{Total: held, Spendable: held, Unspendable: money.NewBaseUnits(0, 6)}, nil
}

func (gatedChain) SignatureStatuses(context.Context, []chain.Signature) ([]solana.Status, error) {
	return nil, nil
}

type distinctTransfers struct{ n atomic.Int64 }

func (t *distinctTransfers) Build(context.Context, relayer.TransferSpec) (relayer.SignedTx, error) {
	sig := chain.Signature(fmt.Sprintf("pressure-%d-%s", t.n.Add(1), uuid.NewString()))
	return relayer.SignedTx{Bytes: []byte(sig), Signature: sig, LastValidBlockHeight: 900}, nil
}

func (*distinctTransfers) Broadcast(context.Context, relayer.SignedTx) error { return nil }

type quietHints struct{}

func (quietHints) PublishHint(context.Context, string, []byte) {}

type withdrawPressure struct {
	pool    *pgxpool.Pool
	guard   *db.NestedReadGuard
	handler *app.WithdrawHandler
	holders []testkit.SeededUser
}

func newWithdrawPressure(t *testing.T) withdrawPressure {
	t.Helper()
	pool, guard := testkit.TracedPool(t, testkit.DB(t))
	clk := testkit.NewClock(clock.Real{}.Now().UTC().Truncate(time.Microsecond))
	usdc := chain.Mint{Address: testkit.USDCMint, Decimals: 6}
	deps := module.Deps{Pool: pool}
	wallet := chain.Wallet{ID: "member-wallet", Address: "9xQeWvG816bUx9EPjHmaT23yvVMvM9fQj4a8PHF4H6P"}
	gate := testkit.NewGate(testkit.PressureCallers)
	balances := adapters.NewBalances(
		signingWalletAddress{wallet.Address},
		func() adapters.ChainReads { return gatedChain{gate: gate} },
		adapters.Outflows{
			Funds:       treasury.New(deps).FundOutflows(),
			Withdrawals: app.WithdrawalOutflows{Reads: pool},
			FundsOn:     func(q dbsqlc.DBTX) app.Outflows { return treasury.New(deps).FundOutflowsOn(q) },
			WithdrawalsOn: func(q dbsqlc.DBTX) app.Outflows {
				return app.WithdrawalOutflows{Reads: q}
			},
		},
		clk, usdc)
	handler := app.NewWithdrawHandler(app.WithdrawDeps{
		UoW: db.New(pool, testkit.NewIDs(5), clk), Balances: balances, Wallets: signingWallet{wallet: wallet},
		Treasuries: cabal.New(deps).Queries(),
		Transfers:  func() (app.Transfers, error) { return &distinctTransfers{}, nil },
		Hints:      quietHints{}, Clock: clk, USDC: usdc, TopUp: &topUpFake{},
	})
	return withdrawPressure{
		pool: pool, guard: guard, handler: handler,
		holders: []testkit.SeededUser{
			testkit.SeedUser(t, pool, testkit.UserOpts{WithWallet: true}),
			testkit.SeedUser(t, pool, testkit.UserOpts{WithWallet: true}),
		},
	}
}

func (p withdrawPressure) withdraw(ctx context.Context, holder testkit.SeededUser) func() error {
	return func() error {
		_, err := p.handler.Handle(observability.WithActor(ctx, "user:"+holder.ID.String()), app.Withdraw{
			ID: ids.Real{}.NewV7(), UserID: holder.ID,
			Request: domain.WithdrawalRequest{Amount: money.MicrosFromUint64(withdrawMicros), To: withdrawTo},
		})
		return err
	}
}

func TestWithdrawalsNeverNestPoolReadsAtFourConnections(t *testing.T) {
	t.Parallel()
	p := newWithdrawPressure(t)
	ctx, cancel := context.WithTimeout(t.Context(), testkit.PressureDeadline)
	defer cancel()
	probe := testkit.StartProbe(ctx, p.pool)
	jobs := make([]func() error, 0, testkit.PressureCallers)
	for i := range testkit.PressureCallers {
		jobs = append(jobs, p.withdraw(ctx, p.holders[i%len(p.holders)]))
	}
	testkit.Concurrently(t, jobs...)
	probe.Check(t)
	if ctx.Err() != nil {
		t.Fatalf("the run did not finish inside %s: %v", testkit.PressureDeadline, ctx.Err())
	}
	var submitted int
	err := p.pool.QueryRow(t.Context(), `SELECT count(*) FROM withdrawals WHERE status = 'submitted'`).Scan(&submitted)
	if err != nil || submitted != testkit.PressureCallers {
		t.Errorf("submitted withdrawals = %d, %v, want %d", submitted, err, testkit.PressureCallers)
	}
	testkit.CheckNoNestedReads(t, p.guard)
}

func TestWiredBalancesReadTheOutflowsThroughTheTransaction(t *testing.T) {
	t.Parallel()
	pool, guard := testkit.TracedPool(t, testkit.DB(t))
	clk := testkit.NewClock(clock.Real{}.Now().UTC())
	uow := db.New(pool, testkit.NewIDs(5), clk)
	balances := funding.New(module.Deps{Pool: pool, Clock: clk, Config: testkit.Config()}).Balances()
	user := testkit.SeedUser(t, pool, testkit.UserOpts{WithWallet: true})
	five := money.MicrosFromUint64(5)
	obs := port.Observation{User: user.ID, OnChainMicros: five, SpendableMicros: five, At: clk.Now()}
	var got port.Balance
	err := uow.Do(t.Context(), func(ctx context.Context, tx db.Tx) error {
		var err error
		got, err = balances.AvailableIn(ctx, tx.Queries(), obs)
		return err
	})
	if err != nil || got.AvailableMicros.Uint64() != 5 {
		t.Fatalf("AvailableIn = %+v, %v, want 5 available", got, err)
	}
	testkit.CheckNoNestedReads(t, guard)
}
