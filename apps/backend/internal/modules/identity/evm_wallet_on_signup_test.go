package identity_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity/adapters"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity/app"
	"github.com/monaco/monaco/apps/backend/internal/platform/bus"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/multichain"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
	"github.com/monaco/monaco/apps/backend/internal/testkit/privyfake"
)

type evmSignup struct {
	pool    *pgxpool.Pool
	uow     *db.UnitOfWork
	wallets *privyfake.EVMWallets
	handler adapters.EVMWalletOnSignup
	at      time.Time
}

func newEVMSignup(t *testing.T, evm multichain.EVMWallets) evmSignup {
	t.Helper()
	return newEVMSignupWith(t, evm, &privyfake.Forwarding{})
}

func newEVMSignupWith(t *testing.T, evm multichain.EVMWallets, forwarding multichain.DepositForwarding) evmSignup {
	t.Helper()
	pool := testkit.DB(t)
	clk := testkit.NewClock(time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC))
	uow := db.New(pool, testkit.NewIDs(5431), clk)
	wallets, _ := evm.(*privyfake.EVMWallets)
	return evmSignup{
		pool:    pool,
		uow:     uow,
		wallets: wallets,
		at:      clk.Now(),
		handler: adapters.EVMWalletOnSignup{
			UoW:      uow,
			Networks: []chain.Network{chain.NetworkBase},
			Wallets: app.NewEVMDepositWallets(app.EVMDepositWalletDeps{
				UoW: uow, Reads: pool, Store: adapters.EVMWallets{}, Wallets: evm, Forwarding: forwarding,
				Clock: clk,
			}),
		},
	}
}

type signup struct {
	delivery bus.Delivery
	event    events.UserCreated
}

func (s evmSignup) signUp(t *testing.T, user ids.UserID) signup {
	t.Helper()
	e := events.UserCreated{V: 1, UserID: user.UUID(), LoginProvider: "sms", CreatedAt: s.at}
	ctx := observability.WithActor(t.Context(), "system:test")
	if err := s.uow.Do(ctx, func(ctx context.Context, tx db.Tx) error { return tx.Events.Append(ctx, e) }); err != nil {
		t.Fatal(err)
	}
	var id uuid.UUID
	if err := s.pool.QueryRow(t.Context(), `SELECT id FROM events WHERE aggregate_id = $1`, user.UUID()).
		Scan(&id); err != nil {
		t.Fatal(err)
	}
	return signup{
		delivery: bus.Delivery{Handler: "identity.evm_wallet", EventID: ids.EventIDFrom(id), At: s.at},
		event:    e,
	}
}

func (s evmSignup) deliver(t *testing.T, up signup) error {
	t.Helper()
	return s.handler.Handle(t.Context(), up.delivery, up.event)
}

func (s evmSignup) recorded(t *testing.T, d bus.Delivery) string {
	t.Helper()
	var code string
	err := s.pool.QueryRow(t.Context(), `SELECT code FROM event_deliveries WHERE handler = $1 AND event_id = $2`,
		d.Handler, d.EventID.UUID()).Scan(&code)
	if errors.Is(err, pgx.ErrNoRows) {
		return ""
	}
	if err != nil {
		t.Fatal(err)
	}
	return code
}

func (s evmSignup) evmAddress(t *testing.T, user ids.UserID) string {
	t.Helper()
	var addr *string
	if err := s.pool.QueryRow(t.Context(), `SELECT evm_address FROM user_wallets WHERE user_id = $1`, user.UUID()).
		Scan(&addr); err != nil {
		t.Fatal(err)
	}
	if addr == nil {
		return ""
	}
	return *addr
}

func TestEVMWalletOnSignup_storesTheWalletOnceAcrossRedeliveries(t *testing.T) {
	t.Parallel()
	s := newEVMSignup(t, &privyfake.EVMWallets{})
	user := testkit.SeedUser(t, s.pool, testkit.UserOpts{WithWallet: true}).ID
	up := s.signUp(t, user)
	if err := s.deliver(t, up); err != nil || s.recorded(t, up.delivery) != "ok" {
		t.Fatalf("handle = %v, recorded %q; want ok", err, s.recorded(t, up.delivery))
	}
	first := s.evmAddress(t, user)
	if err := s.deliver(t, up); err != nil || s.evmAddress(t, user) != first || first == "" ||
		s.wallets.Creates() != 1 {
		t.Fatalf("redelivery = %v, address %q then %q, %d creates", err, first, s.evmAddress(t, user),
			s.wallets.Creates())
	}
}

func TestEVMWalletOnSignup_aPrivyOutageNaksAndStoresNothing(t *testing.T) {
	t.Parallel()
	wallets := &privyfake.EVMWallets{}
	wallets.Fail("CreateAppEVMWallet", errs.New(errs.CodePrivyUnavailable, "test"))
	s := newEVMSignup(t, wallets)
	user := testkit.SeedUser(t, s.pool, testkit.UserOpts{WithWallet: true}).ID
	up := s.signUp(t, user)
	err := s.deliver(t, up)
	if errs.VerdictFor(errs.CodeOf(err)) != errs.VerdictNak || s.recorded(t, up.delivery) != "" ||
		s.evmAddress(t, user) != "" {
		t.Fatalf("handle = %v, recorded %q, address %q; want a nak and nothing stored", err, s.recorded(t, up.delivery),
			s.evmAddress(t, user))
	}
}

func TestEVMWalletOnSignup_acksWhatItCannotEnsure(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct {
		wallets multichain.EVMWallets
		opts    testkit.UserOpts
		want    errs.Code
	}{
		"a deleted user":     {&privyfake.EVMWallets{}, testkit.UserOpts{WithWallet: true, AccountStatus: "deleted"}, errs.CodeUserNotFound},
		"no member wallet":   {&privyfake.EVMWallets{}, testkit.UserOpts{}, errs.CodeUserNotFound},
		"privy is not wired": {multichain.Pending{Ticket: "#5431"}, testkit.UserOpts{WithWallet: true}, errs.CodeNotImplemented},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			s := newEVMSignup(t, tc.wallets)
			up := s.signUp(t, testkit.SeedUser(t, s.pool, tc.opts).ID)
			if err := s.deliver(t, up); err != nil || s.recorded(t, up.delivery) != string(tc.want) {
				t.Fatalf("handle = %v, recorded %q; want %s", err, s.recorded(t, up.delivery), tc.want)
			}
		})
	}
}

func TestEVMWalletOnSignup_defersTheWalletWhenNoEVMNetworkIsOffered(t *testing.T) {
	t.Parallel()
	s := newEVMSignup(t, &privyfake.EVMWallets{})
	s.handler.Networks = nil
	user := testkit.SeedUser(t, s.pool, testkit.UserOpts{WithWallet: true}).ID
	up := s.signUp(t, user)
	if err := s.deliver(t, up); err != nil || s.recorded(t, up.delivery) != "deferred" ||
		s.evmAddress(t, user) != "" || s.wallets.Creates() != 0 {
		t.Fatalf("handle = %v, recorded %q, address %q, %d creates; want deferred with no Privy call",
			err, s.recorded(t, up.delivery), s.evmAddress(t, user), s.wallets.Creates())
	}
}

func TestEnsureEVMDepositWallet_holdsNoConnectionAcrossThePrivyCalls(t *testing.T) {
	t.Parallel()
	var s evmSignup
	var held []int32
	forwarding := &privyfake.Forwarding{OnAttach: func() { held = append(held, s.pool.Stat().AcquiredConns()) }}
	s = newEVMSignupWith(t, &privyfake.EVMWallets{}, forwarding)
	user := testkit.SeedUser(t, s.pool, testkit.UserOpts{WithWallet: true}).ID
	up := s.signUp(t, user)
	if err := s.deliver(t, up); err != nil || s.evmAddress(t, user) == "" {
		t.Fatalf("handle = %v, address %q; want a stored wallet", err, s.evmAddress(t, user))
	}
	if len(held) != 1 || held[0] != 0 {
		t.Fatalf("connections held while attaching = %v, want one attach with none held", held)
	}
}
