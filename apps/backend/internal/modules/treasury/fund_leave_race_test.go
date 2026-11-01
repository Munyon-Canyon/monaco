package treasury_test

import (
	"context"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/cabal"
	cabalport "github.com/monaco/monaco/apps/backend/internal/modules/cabal/port"
	fundingport "github.com/monaco/monaco/apps/backend/internal/modules/funding/port"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/adapters"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/app"
	"github.com/monaco/monaco/apps/backend/internal/platform/auth"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	dbsqlc "github.com/monaco/monaco/apps/backend/internal/platform/db/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

type lockedMember struct {
	*fundStubs
	real     cabalport.Queries
	locked   chan struct{}
	release  chan struct{}
	holdOnce sync.Once
}

func (l *lockedMember) LockMember(ctx context.Context, q dbsqlc.DBTX, id ids.CabalID, user ids.UserID) (bool, error) {
	ok, err := l.real.LockMember(ctx, q, id, user)
	l.holdOnce.Do(func() {
		close(l.locked)
		<-l.release
	})
	return ok, err
}

type fundLeaveRace struct {
	fixture
	cabal   testkit.SeededCabal
	user    ids.UserID
	fund    *app.FundCabalHandler
	leave   *cabal.LeaveCabalHandler
	barrier *lockedMember
}

func newFundLeaveRace(t *testing.T) *fundLeaveRace {
	t.Helper()
	f := newFixture(t)
	c := testkit.NewCabal(t, f.pool, testkit.WithMembers(2))
	usdc := chain.SolanaAddress(f.cfg.Solana.USDCMint)
	stubs := &fundStubs{
		member: true, lockMember: true, status: cabalport.StatusActive, onChain: money.MicrosFromUint64(100_000_000),
		reads: adapters.NewFundOutflows(f.pool), owner: adapters.NewQueries(f.pool, nil, nil, f.clock, usdc),
	}
	cabalMod := cabal.New(module.Deps{Config: f.cfg, Clock: f.clock, IDs: f.ids, Pool: f.pool, UoW: f.uow})
	b := &lockedMember{
		fundStubs: stubs, real: cabalMod.Queries(), locked: make(chan struct{}),
		release: make(chan struct{}),
	}
	fund := app.NewFundCabalHandler(app.FundCabalDeps{
		UoW: f.uow, IDs: f.ids, Clock: f.clock, Cabals: b, Wallets: stubs, Balances: stubs,
		Pauses: func(db.Tx) fundingport.Pauses { return stubs }, Pot: stubs, Transfers: stubs,
		Hints: &hints{}, USDC: chain.Mint{Address: usdc, Decimals: 6}, SendWindow: fundSendWindow,
	})
	return &fundLeaveRace{
		fixture: f, cabal: c, user: c.Members[1].ID, fund: fund, barrier: b,
		leave: cabalMod.LeaveCabalHandler(),
	}
}

func (r *fundLeaveRace) ctx() context.Context {
	return auth.WithActor(r.fixture.ctx(), auth.Actor{Kind: auth.ActorUser, ID: r.user.String()})
}

func (r *fundLeaveRace) members(t *testing.T) int {
	t.Helper()
	var n int
	if err := r.pool.QueryRow(t.Context(),
		`SELECT count(*) FROM cabal_members WHERE cabal_id = $1 AND user_id = $2`, r.cabal.ID.UUID(), r.user.UUID(),
	).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func (r *fundLeaveRace) funds(t *testing.T) int {
	t.Helper()
	var n int
	if err := r.pool.QueryRow(t.Context(),
		`SELECT count(*) FROM fund_transfers WHERE cabal_id = $1 AND user_id = $2`, r.cabal.ID.UUID(), r.user.UUID(),
	).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func (r *fundLeaveRace) waitForBlockedLeave(t *testing.T) {
	t.Helper()
	testkit.Eventually(t, func() bool {
		var n int
		if err := r.pool.QueryRow(t.Context(),
			`SELECT count(*) FROM pg_locks WHERE NOT granted AND locktype IN ('tuple', 'transactionid')
				AND pid IN (SELECT pid FROM pg_stat_activity WHERE datname = current_database())`,
		).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n > 0
	}, 10*time.Second)
}

func TestFundCabal_holdingTheMemberLockMakesALeaveWaitThenRefuse(t *testing.T) {
	t.Parallel()
	r := newFundLeaveRace(t)
	var fundErr, leaveErr error
	var wg sync.WaitGroup
	wg.Go(func() {
		_, fundErr = r.fund.Handle(r.ctx(), app.FundCabal{
			CabalID: r.cabal.ID, UserID: r.user, Amount: money.MicrosFromUint64(5_000_000),
		})
	})
	<-r.barrier.locked
	wg.Go(func() { leaveErr = r.leave.Handle(r.ctx(), cabal.LeaveCabal{ActorID: r.user, CabalID: r.cabal.ID}) })
	r.waitForBlockedLeave(t)
	if got := r.members(t); got != 1 {
		t.Fatalf("members = %d while Fund holds the lock, want 1", got)
	}
	close(r.barrier.release)
	wg.Wait()
	if fundErr != nil {
		t.Fatalf("FundCabal = %v, want success", fundErr)
	}
	if got := errs.CodeOf(leaveErr); got != errs.CodeLeaveHoldsShares {
		t.Fatalf("LeaveCabal code = %q, want %q", got, errs.CodeLeaveHoldsShares)
	}
	if m, f := r.members(t), r.funds(t); m != 1 || f != 1 {
		t.Fatalf("members = %d, funds = %d; want the member kept with the one fund", m, f)
	}
}

func TestFundCabal_racingALeaveNeverFundsANonMember(t *testing.T) {
	t.Parallel()
	for i := range 10 {
		t.Run(strconv.Itoa(i), func(t *testing.T) {
			t.Parallel()
			r := newFundLeaveRace(t)
			r.barrier.holdOnce.Do(func() {})
			var fundErr, leaveErr error
			var wg sync.WaitGroup
			wg.Go(func() {
				_, fundErr = r.fund.Handle(r.ctx(), app.FundCabal{
					CabalID: r.cabal.ID, UserID: r.user, Amount: money.MicrosFromUint64(5_000_000),
				})
			})
			wg.Go(func() {
				leaveErr = r.leave.Handle(r.ctx(), cabal.LeaveCabal{ActorID: r.user, CabalID: r.cabal.ID})
			})
			wg.Wait()
			if (fundErr == nil) == (leaveErr == nil) {
				t.Fatalf("fund = %v, leave = %v; want exactly one to succeed", fundErr, leaveErr)
			}
			if m, f := r.members(t), r.funds(t); m != f {
				t.Fatalf("members = %d, funds = %d; want a kept member with its fund or neither", m, f)
			}
		})
	}
}
