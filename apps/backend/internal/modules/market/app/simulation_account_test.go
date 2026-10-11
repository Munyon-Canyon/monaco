package app_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/market/app"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/multichain"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

const (
	simWallet   = "wallet-sim"
	simAddress  = "0x00000000000000000000000000000000000051a1"
	simDelegate = "0xd6cedde84be40893d153be9d467cd6ad37875b28"
)

var errSimFake = errs.New(errs.CodeInternal, "fake")

type simWallets struct {
	creates int
	sends   []multichain.EVMTransaction
	sponsor []bool
	created multichain.EVMWallet
	err     error
}

func (w *simWallets) CreateAppEVMWallet(_ context.Context, key string) (multichain.EVMWallet, error) {
	w.creates++
	if key != "market:simulation-account" {
		return multichain.EVMWallet{}, errSimFake
	}
	return w.created, w.err
}

func (*simWallets) SignEVMTransaction(context.Context, string, multichain.EVMTransaction, bool) ([]byte, error) {
	return nil, errSimFake
}

func (w *simWallets) SendEVMTransaction(
	_ context.Context, walletID string, tx multichain.EVMTransaction, sponsor bool,
) (string, error) {
	if walletID != simWallet {
		return "", errSimFake
	}
	w.sends, w.sponsor = append(w.sends, tx), append(w.sponsor, sponsor)
	return "0xsent", w.err
}

type simCode struct {
	code map[chain.Network]string
	err  error
}

func (c simCode) Code(_ context.Context, n chain.Network, _ chain.EVMAddress) (string, error) {
	return c.code[n], c.err
}

func simAccount(t *testing.T) app.SimulationAccount {
	t.Helper()
	a, err := chain.ParseEVMAddress(simAddress)
	if err != nil {
		t.Fatal(err)
	}
	return app.SimulationAccount{WalletID: simWallet, Address: a}
}

func simulationWallets(t *testing.T) *simWallets {
	t.Helper()
	a := simAccount(t)
	return &simWallets{created: multichain.EVMWallet{ID: a.WalletID, Address: a.Address}}
}

func TestSetUpSimulationAccount_createsOnceAndReusesTheStoredWallet(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	wallets := simulationWallets(t)
	now := testkit.NewClock(time.Date(2026, 10, 11, 12, 0, 0, 0, time.UTC)).Now()

	first, created, err := app.SetUpSimulationAccount(t.Context(), pool, wallets, now)
	if err != nil || !created || first != simAccount(t) || wallets.creates != 1 {
		t.Fatalf("first run = %+v created %t, %v, creates %d", first, created, err, wallets.creates)
	}
	second, created, err := app.SetUpSimulationAccount(t.Context(), pool, wallets, now)
	if err != nil || created || second != first || wallets.creates != 1 {
		t.Fatalf("second run = %+v created %t, %v, creates %d, want the stored wallet and no new create",
			second, created, err, wallets.creates)
	}
}

func TestSetUpSimulationAccount_reportsCreateAndStoreFailures(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	wallets := simulationWallets(t)
	wallets.err = errs.New(errs.CodePrivyUnavailable, "fake")
	if _, _, err := app.SetUpSimulationAccount(t.Context(), pool, wallets, time.Time{}); !errors.Is(err, wallets.err) {
		t.Fatalf("create failure = %v", err)
	}
	wallets.err, wallets.created.ID = nil, ""
	if _, _, err := app.SetUpSimulationAccount(t.Context(), pool, wallets, time.Time{}); err == nil {
		t.Fatal("store failure returned no error")
	}
	if _, ok, err := app.LoadSimulationAccount(t.Context(), pool); ok || err != nil {
		t.Fatalf("after failures = %t, %v, want nothing stored", ok, err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, _, err := app.SetUpSimulationAccount(ctx, pool, wallets, time.Time{}); err == nil {
		t.Fatal("read failure returned no error")
	}
}

func TestSimulationAccounts_readTheStoredAddressAndKeepIt(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	accounts := app.NewSimulationAccounts(pool)
	if _, ok, err := accounts.SimulationAccount(t.Context()); ok || err != nil {
		t.Fatalf("empty = %t, %v, want no account", ok, err)
	}
	if _, _, err := app.SetUpSimulationAccount(t.Context(), pool, simulationWallets(t), time.Time{}); err != nil {
		t.Fatal(err)
	}
	if got, ok, err := accounts.SimulationAccount(t.Context()); !ok || err != nil || got != simAccount(t).Address {
		t.Fatalf("stored = %s, %t, %v, want %s", got, ok, err, simAddress)
	}
	if _, err := pool.Exec(t.Context(), `DELETE FROM evm_simulation_account`); err != nil {
		t.Fatal(err)
	}
	if got, ok, err := accounts.SimulationAccount(t.Context()); !ok || err != nil || got != simAccount(t).Address {
		t.Fatalf("after delete = %s, %t, %v, want the kept address", got, ok, err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, _, err := app.NewSimulationAccounts(pool).SimulationAccount(ctx); err == nil {
		t.Fatal("read failure returned no error")
	}
}

func simulationCode() simCode {
	return simCode{code: map[chain.Network]string{
		chain.NetworkBase:      "0xef0100" + simDelegate[2:],
		chain.NetworkRobinhood: "0x",
	}}
}

func TestDelegateSimulationAccount_leavesADelegatedChainAlone(t *testing.T) {
	t.Parallel()
	wallets := simulationWallets(t)
	base, err := app.DelegateSimulationAccount(t.Context(), wallets, simulationCode(), simAccount(t), chain.NetworkBase)
	if err != nil || !base.Delegated() || base.Delegate.String() != simDelegate || base.Sent != "" ||
		len(wallets.sends) != 0 {
		t.Fatalf("delegated chain = %+v, %v, sends %d, want no send", base, err, len(wallets.sends))
	}
}

func TestDelegateSimulationAccount_sendsASponsoredSelfTransferWhenUndelegated(t *testing.T) {
	t.Parallel()
	a := simAccount(t)
	wallets := simulationWallets(t)
	rh, err := app.DelegateSimulationAccount(t.Context(), wallets, simulationCode(), a, chain.NetworkRobinhood)
	if err != nil || rh.Delegated() || rh.Code != "0x" || rh.Sent != "0xsent" || len(wallets.sends) != 1 {
		t.Fatalf("undelegated chain = %+v, %v, sends %d", rh, err, len(wallets.sends))
	}
	tx := wallets.sends[0]
	if !wallets.sponsor[0] || tx.Network != chain.NetworkRobinhood || tx.To != a.Address || tx.Value.Sign() != 0 ||
		len(tx.Data) != 0 || tx.IdempotencyKey != "market:simulation-account:delegate:robinhood:"+simWallet {
		t.Fatalf("send = %+v sponsor %t, want a sponsored zero-value self-transfer", tx, wallets.sponsor[0])
	}
}

func TestDelegateSimulationAccount_reportsCodeAndSendFailures(t *testing.T) {
	t.Parallel()
	a := simAccount(t)
	wallets := simulationWallets(t)
	rpcDown := errs.New(errs.CodeRPCUnavailable, "fake")
	if _, err := app.DelegateSimulationAccount(t.Context(), wallets, simCode{err: rpcDown}, a,
		chain.NetworkBase); !errors.Is(err, rpcDown) || len(wallets.sends) != 0 {
		t.Fatalf("code failure = %v, sends %d", err, len(wallets.sends))
	}
	wallets.err = errs.New(errs.CodePrivyUnavailable, "fake")
	code := simCode{code: map[chain.Network]string{chain.NetworkBase: "0x"}}
	if _, err := app.DelegateSimulationAccount(t.Context(), wallets, code, a,
		chain.NetworkBase); errs.CodeOf(err) != errs.CodePrivyUnavailable {
		t.Fatalf("send failure = %v", err)
	}
}
