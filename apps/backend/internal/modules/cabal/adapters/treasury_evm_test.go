package adapters_test

import (
	"context"
	"sync"
	"testing"

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
