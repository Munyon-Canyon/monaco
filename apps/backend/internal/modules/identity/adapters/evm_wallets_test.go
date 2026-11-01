package adapters_test

import (
	"context"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity/adapters"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/multichain"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/config"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
	"github.com/monaco/monaco/apps/backend/internal/testkit/fakes"
	"github.com/monaco/monaco/apps/backend/internal/testkit/privyfake"
)

const evmMember = chain.SolanaAddress("Dht9c9YfstFWkNYXgqr8HZbhqVn563bCpNU6zL32Ftqf")

func seedMember(t *testing.T, q *sqlc.Queries, wallet bool) ids.UserID {
	t.Helper()
	user := ids.NewUserID(ids.Real{})
	now := clock.Real{}.Now().UTC()
	if _, err := q.CreateUser(t.Context(), sqlc.CreateUserParams{
		ID: user.UUID(), PrivyUserID: "did:privy:evm-" + user.String(), LoginProvider: "sms", Now: now,
	}); err != nil {
		t.Fatal(err)
	}
	if wallet {
		if _, err := q.AttachUserWallet(t.Context(), sqlc.AttachUserWalletParams{
			UserID: user.UUID(), PrivyWalletID: "wallet-" + user.String(), Address: string(evmMember), CreatedAt: now,
		}); err != nil {
			t.Fatal(err)
		}
	}
	return user
}

func TestEnsureEVMDepositWallet_twoConcurrentCallsStoreOneWalletAndAttachItToTheSameMember(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	q := sqlc.New(pool)
	user := seedMember(t, q, true)
	wallets, forwarding := &privyfake.EVMWallets{}, &privyfake.Forwarding{}
	ensure := app.NewEVMDepositWallets(app.EVMDepositWalletDeps{
		UoW: db.New(pool, ids.Real{}, clock.Real{}), Reads: pool, Store: adapters.EVMWallets{},
		Wallets: wallets, Forwarding: forwarding, Clock: clock.Real{},
	})
	got := make([]chain.EVMAddress, 2)
	failures := make([]error, 2)
	var wg sync.WaitGroup
	for i := range got {
		wg.Go(func() { got[i], failures[i] = ensure.EnsureEVMDepositWallet(t.Context(), user) })
	}
	wg.Wait()
	if failures[0] != nil || failures[1] != nil || got[0] == (chain.EVMAddress{}) || got[0] != got[1] {
		t.Fatalf("ensures = %s / %s, errors %v / %v", got[0], got[1], failures[0], failures[1])
	}
	row, err := q.FindUserEVMWallet(t.Context(), user.UUID())
	if err != nil || row.EvmAddress != got[0].String() || forwarding.Attaches() < 1 || forwarding.Attaches() > 2 ||
		forwarding.Destination(row.EvmPrivyWalletID) != evmMember {
		t.Fatalf("stored %+v, %v; %d attaches to %s", row, err, forwarding.Attaches(),
			forwarding.Destination(row.EvmPrivyWalletID))
	}
}

func newEnsure(
	pool *pgxpool.Pool, wallets multichain.EVMWallets, forwarding multichain.DepositForwarding,
) *app.EVMDepositWallets {
	return app.NewEVMDepositWallets(app.EVMDepositWalletDeps{
		UoW: db.New(pool, ids.Real{}, clock.Real{}), Reads: pool, Store: adapters.EVMWallets{},
		Wallets: wallets, Forwarding: forwarding, Clock: clock.Real{},
	})
}

func TestEnsureEVMDepositWallet_anAttachFailureStoresNothingAndALaterCallRecovers(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	q := sqlc.New(pool)
	user := seedMember(t, q, true)
	forwarding := &privyfake.Forwarding{}
	forwarding.Fail("Attach", errs.New(errs.CodePrivyUnavailable, "test"))
	ensure := newEnsure(pool, &privyfake.EVMWallets{}, forwarding)
	if _, err := ensure.EnsureEVMDepositWallet(t.Context(), user); errs.CodeOf(err) != errs.CodePrivyUnavailable {
		t.Fatalf("ensure with Privy down = %v, want privy_unavailable", err)
	}
	if _, found, err := (adapters.EVMWallets{}).StoredEVMWallet(t.Context(), pool, user); err != nil || found {
		t.Fatalf("stored after a failed attach = %v, %v; want nothing", found, err)
	}
	forwarding.Fail("Attach", nil)
	if got, err := ensure.EnsureEVMDepositWallet(t.Context(), user); err != nil || got == (chain.EVMAddress{}) {
		t.Fatalf("ensure after Privy returned = %s, %v; want the wallet", got, err)
	}
}

func TestEnsureEVMDepositWallet_aWalletStoredWhilePrivyWasBusyWins(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	q := sqlc.New(pool)
	user := seedMember(t, q, true)
	rival := multichain.EVMWallet{ID: "wallet-evm-rival", Address: chain.EVMAddress{9}}
	forwarding := &privyfake.Forwarding{OnAttach: func() {
		if err := (adapters.EVMWallets{}).SetEVMWallet(t.Context(), pool, user, rival, clock.Real{}.Now()); err != nil {
			t.Error(err)
		}
	}}
	got, err := newEnsure(pool, &privyfake.EVMWallets{}, forwarding).EnsureEVMDepositWallet(t.Context(), user)
	if err != nil || got != rival.Address {
		t.Fatalf("ensure = %s, %v; want the wallet stored meanwhile %s", got, err, rival.Address)
	}
}

func TestEnsureEVMDepositWallet_aUserRemovedWhilePrivyWasBusyStoresNothing(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	q := sqlc.New(pool)
	user := seedMember(t, q, true)
	forwarding := &privyfake.Forwarding{OnAttach: func() {
		if _, err := pool.Exec(
			t.Context(),
			`WITH w AS (DELETE FROM user_wallets WHERE user_id = $1) DELETE FROM users WHERE id = $1`,
			user.UUID(),
		); err != nil {
			t.Error(err)
		}
	}}
	_, err := newEnsure(pool, &privyfake.EVMWallets{}, forwarding).EnsureEVMDepositWallet(t.Context(), user)
	if errs.CodeOf(err) != errs.CodeUserNotFound {
		t.Fatalf("ensure for a user removed meanwhile = %v, want user_not_found", err)
	}
}

func TestEVMWallets_namesMissingUsersAndReportsDatabaseFailures(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	q := sqlc.New(pool)
	store := adapters.EVMWallets{}
	unknown := ids.NewUserID(ids.Real{})
	wantCode := func(name string, err error, code errs.Code) {
		t.Helper()
		if errs.CodeOf(err) != code {
			t.Fatalf("%s: err = %v, want %s", name, err, code)
		}
	}
	wantCode("lock unknown", store.LockUser(t.Context(), pool, unknown), errs.CodeUserNotFound)
	_, err := store.MemberAddress(t.Context(), pool, seedMember(t, q, false))
	wantCode("no member wallet", err, errs.CodeUserNotFound)
	if _, found, err := store.StoredEVMWallet(t.Context(), pool, unknown); err != nil || found {
		t.Fatalf("stored for unknown = %v, %v", found, err)
	}
	gone, cancel := context.WithCancel(t.Context())
	cancel()
	wantCode("lock", store.LockUser(gone, pool, unknown), errs.CodeInternal)
	_, err = store.MemberAddress(gone, pool, unknown)
	wantCode("member", err, errs.CodeInternal)
	_, _, err = store.StoredEVMWallet(gone, pool, unknown)
	wantCode("stored", err, errs.CodeInternal)
	err = store.SetEVMWallet(gone, pool, unknown, multichain.EVMWallet{ID: "w"}, clock.Real{}.Now())
	wantCode("set", err, errs.CodeInternal)
}

func TestEVMWallets_aCorruptStoredAddressIsInternal(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	q := sqlc.New(pool)
	user := seedMember(t, q, true)
	if _, err := pool.Exec(
		t.Context(),
		"ALTER TABLE user_wallets DROP CONSTRAINT user_wallets_evm_address_check",
	); err != nil {
		t.Fatal(err)
	}
	if _, err := q.SetUserEVMWallet(t.Context(), sqlc.SetUserEVMWalletParams{
		EvmPrivyWalletID: pgtype.Text{
			String: "w",
			Valid:  true,
		},
		EvmAddress:   pgtype.Text{String: "0xnot-hex", Valid: true},
		EvmCreatedAt: pgtype.Timestamptz{Time: clock.Real{}.Now(), Valid: true},
		UserID:       user.UUID(),
	}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := (adapters.EVMWallets{}).StoredEVMWallet(
		t.Context(),
		pool,
		user,
	); errs.CodeOf(
		err,
	) != errs.CodeInternal {
		t.Fatalf("err = %v, want internal", err)
	}
}

func TestModuleEVMDepositWallets_failsSafeWithoutPrivyAndEnsuresThroughPrivy(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	q := sqlc.New(pool)
	user := seedMember(t, q, true)
	deps := func(cfg config.Config) module.Deps {
		return module.Deps{
			Config: cfg, Clock: clock.Real{}, IDs: ids.Real{}, Pool: pool, UoW: db.New(pool, ids.Real{}, clock.Real{}),
		}
	}
	_, err := identity.New(deps(config.Config{})).EVMDepositWallets().EnsureEVMDepositWallet(t.Context(), user)
	if errs.CodeOf(err) != errs.CodeNotImplemented {
		t.Fatalf("without privy: err = %v, want not_implemented", err)
	}
	if _, err := q.FindUserEVMWallet(t.Context(), user.UUID()); err == nil {
		t.Fatal("a wallet was stored without privy")
	}
	srv := httptest.NewServer(fakes.New())
	t.Cleanup(srv.Close)
	privyConfig := config.Config{Privy: config.Privy{
		AppID: "app", AppSecret: "secret", BaseURL: srv.URL + "/privy",
		VerificationKey:         fakes.PrivyVerificationKey(),
		AuthorizationPrivateKey: fakes.PrivyAuthorizationKeyConfig(),
		AuthorizationKeyID:      fakes.PrivyAuthorizationKeyID,
	}, Timeouts: config.Timeouts{Privy: 5 * time.Second}}
	got, err := identity.New(deps(privyConfig)).EVMDepositWallets().EnsureEVMDepositWallet(t.Context(), user)
	if err != nil {
		t.Fatalf("through privy: %v", err)
	}
	stored, err := q.FindUserEVMWallet(t.Context(), user.UUID())
	if err != nil || stored.EvmAddress != got.String() {
		t.Fatalf("stored wallet = %+v, %v, want the ensured address %s", stored, err, got)
	}
}
