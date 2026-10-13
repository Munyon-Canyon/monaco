package adapters_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	cabalmodule "github.com/monaco/monaco/apps/backend/internal/modules/cabal"
	"github.com/monaco/monaco/apps/backend/internal/modules/cabal/adapters"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/multichain"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
	"github.com/monaco/monaco/apps/backend/internal/testkit/privyfake"
)

func TestEnsureTreasuryEVMWallet_concurrentCallsStoreOneWallet(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	cabal := testkit.NewCabal(t, pool)
	wallets := &privyfake.EVMWallets{}
	ensure := adapters.TreasuryEVMWallets{DB: pool, Wallets: wallets, Clock: clock.Real{}}
	got := make([]multichain.EVMWallet, 3)
	failures := make([]error, 3)
	var wg sync.WaitGroup
	for i := range got {
		wg.Go(func() { got[i], failures[i] = ensure.EnsureTreasuryEVMWallet(t.Context(), cabal.ID) })
	}
	wg.Wait()
	for i := range got {
		if failures[i] != nil || got[i] != got[0] || got[i].ID == "" {
			t.Fatalf("ensure %d = %+v, %v; want %+v", i, got[i], failures[i], got[0])
		}
	}
	creates := wallets.Creates()
	if again, err := ensure.EnsureTreasuryEVMWallet(t.Context(), cabal.ID); err != nil || again != got[0] ||
		wallets.Creates() != creates {
		t.Fatalf("a stored wallet = %+v, %v after %d creates; want no new create", again, err, wallets.Creates())
	}
}

type lockstepWallets struct {
	*privyfake.EVMWallets
	arrived *sync.WaitGroup
}

func (l lockstepWallets) CreateAppEVMWallet(ctx context.Context, key string) (multichain.EVMWallet, error) {
	l.arrived.Done()
	l.arrived.Wait()
	return l.EVMWallets.CreateAppEVMWallet(ctx, key)
}

func TestEnsureTreasuryEVMWallet_twoEnsuresInsertingAtOnceBothReadTheStoredWallet(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	clk := testkit.NewClock(time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC))
	for round := range 30 {
		cabal := testkit.NewCabal(t, pool)
		var arrived sync.WaitGroup
		arrived.Add(2)
		ensure := adapters.TreasuryEVMWallets{
			DB: pool, Wallets: lockstepWallets{EVMWallets: &privyfake.EVMWallets{}, arrived: &arrived}, Clock: clk,
		}
		got := make([]multichain.EVMWallet, 2)
		failures := make([]error, 2)
		var wg sync.WaitGroup
		for i := range got {
			wg.Go(func() { got[i], failures[i] = ensure.EnsureTreasuryEVMWallet(t.Context(), cabal.ID) })
		}
		wg.Wait()
		if failures[0] != nil || failures[1] != nil || got[0] != got[1] || got[0].ID == "" {
			t.Fatalf("round %d: ensures = %+v %v, %+v %v; want one stored wallet", round, got[0], failures[0],
				got[1], failures[1])
		}
	}
}

func TestEnsureTreasuryEVMWallet_aWalletStoredForAnotherCabalIsInternal(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	owner, cabal := testkit.NewCabal(t, pool), testkit.NewCabal(t, pool)
	wallets := &privyfake.EVMWallets{}
	taken, err := wallets.CreateAppEVMWallet(t.Context(), "treasury-evm:"+cabal.ID.String())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(
		t.Context(),
		`INSERT INTO treasury_evm_wallets (cabal_id, privy_wallet_id, address, created_at)
		VALUES ($1, $2, $3, now())`,
		owner.ID.UUID(),
		taken.ID,
		taken.Address.String(),
	); err != nil {
		t.Fatal(err)
	}
	ensure := adapters.TreasuryEVMWallets{DB: pool, Wallets: wallets, Clock: clock.Real{}}
	if got, err := ensure.EnsureTreasuryEVMWallet(t.Context(), cabal.ID); errs.CodeOf(err) != errs.CodeInternal {
		t.Fatalf("ensure = %+v, %v; want internal", got, err)
	}
}

func TestEnsureTreasuryEVMWallet_reportsFailures(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	wallets := &privyfake.EVMWallets{}
	ensure := adapters.TreasuryEVMWallets{DB: pool, Wallets: wallets, Clock: clock.Real{}}
	missing := ids.CabalIDFrom(ids.NewUserID(ids.Real{}).UUID())
	if _, err := ensure.EnsureTreasuryEVMWallet(t.Context(), missing); errs.CodeOf(err) != errs.CodeInternal {
		t.Fatalf("unknown cabal: err = %v, want internal", err)
	}
	wallets.Fail("CreateAppEVMWallet", errs.New(errs.CodePrivyUnavailable, "test"))
	cabal := testkit.NewCabal(t, pool)
	if _, err := ensure.EnsureTreasuryEVMWallet(t.Context(), cabal.ID); errs.CodeOf(err) != errs.CodePrivyUnavailable {
		t.Fatalf("create failing: err = %v", err)
	}
}

func TestEnsureTreasuryEVMWallet_aBrokenStoreIsInternal(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	cabal := testkit.NewCabal(t, pool)
	ensure := adapters.TreasuryEVMWallets{DB: pool, Wallets: &privyfake.EVMWallets{}, Clock: clock.Real{}}
	gone, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := ensure.EnsureTreasuryEVMWallet(gone, cabal.ID); errs.CodeOf(err) != errs.CodeInternal {
		t.Fatalf("unreachable store: err = %v, want internal", err)
	}
	if _, err := pool.Exec(
		t.Context(),
		"ALTER TABLE treasury_evm_wallets DROP CONSTRAINT treasury_evm_wallets_address_check",
	); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(
		t.Context(),
		"INSERT INTO treasury_evm_wallets (cabal_id, privy_wallet_id, address, created_at) VALUES ($1, 'w', '0xbad', now())",
		cabal.ID.UUID(),
	); err != nil {
		t.Fatal(err)
	}
	if _, err := ensure.EnsureTreasuryEVMWallet(t.Context(), cabal.ID); errs.CodeOf(err) != errs.CodeInternal {
		t.Fatalf("corrupt address: err = %v, want internal", err)
	}
}

func TestModuleTreasuryEVMWallets_usesThePendingStubWithoutPrivy(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	cabal := testkit.NewCabal(t, pool)
	m := cabalmodule.New(module.Deps{
		Clock: clock.Real{}, IDs: ids.Real{}, Pool: pool, UoW: db.New(pool, ids.Real{}, clock.Real{}),
	})
	if _, err := m.TreasuryEVMWallets().
		EnsureTreasuryEVMWallet(t.Context(), cabal.ID); errs.CodeOf(
		err,
	) != errs.CodeNotImplemented {
		t.Fatalf("err = %v, want not_implemented", err)
	}
}
