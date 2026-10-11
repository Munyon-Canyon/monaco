package trading_test

import (
	"net/http/httptest"
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/cabal"
	"github.com/monaco/monaco/apps/backend/internal/modules/trading"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/config"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	platformids "github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
	"github.com/monaco/monaco/apps/backend/internal/testkit/fakes"
)

func TestModule_wiresTheCabalsTreasuryEVMWalletOnePerCabal(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	srv := httptest.NewServer(fakes.New())
	t.Cleanup(srv.Close)
	cfg := testkit.Config()
	cfg.Privy = config.Privy{
		AppID: "app", AppSecret: "privy-app-5ecret", BaseURL: srv.URL + "/privy",
		VerificationKey:         fakes.PrivyVerificationKey(),
		AuthorizationPrivateKey: fakes.PrivyAuthorizationKeyConfig(),
		AuthorizationKeyID:      fakes.PrivyAuthorizationKeyID,
	}
	cfg.Timeouts.Privy = time.Second
	ids, clk := testkit.NewIDs(9), testkit.NewClock(clock.Real{}.Now())
	d := module.Deps{Config: cfg, Pool: pool, IDs: ids, Clock: clk, UoW: db.New(pool, ids, clk)}
	tr := trading.New(d)
	module.NewSet(cabal.New(d), tr)
	seeded := testkit.NewCabal(t, pool)

	first, err := tr.RouteTreasuryEVM().EnsureTreasuryEVMWallet(t.Context(), seeded.ID)
	if err != nil {
		t.Fatal(err)
	}
	second, err := tr.RouteTreasuryEVM().EnsureTreasuryEVMWallet(t.Context(), seeded.ID)
	if err != nil || second != first || first.ID == "" {
		t.Fatalf("second Ensure = %+v, %v, want the first wallet %+v", second, err, first)
	}
	var wallets int
	if err := pool.QueryRow(t.Context(), `SELECT count(*) FROM treasury_evm_wallets WHERE cabal_id = $1`,
		seeded.ID.UUID()).Scan(&wallets); err != nil || wallets != 1 {
		t.Fatalf("%d treasury EVM wallets, %v, want one", wallets, err)
	}
}

func TestModule_keepsTheTreasuryEVMWalletUnwiredWithoutTheCabalModule(t *testing.T) {
	t.Parallel()
	tr := trading.New(module.Deps{Config: config.Config{}})
	module.NewSet(tr)

	cabalID := platformids.CabalIDFrom(testkit.NewIDs(1).NewV7())
	if _, err := tr.RouteTreasuryEVM().
		EnsureTreasuryEVMWallet(t.Context(), cabalID); errs.CodeOf(
		err,
	) != errs.CodeNotImplemented {
		t.Fatalf("Ensure err = %v, want not_implemented", err)
	}
}
