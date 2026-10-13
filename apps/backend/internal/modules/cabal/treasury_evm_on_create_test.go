package cabal_test

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
	"github.com/monaco/monaco/apps/backend/internal/modules/cabal/adapters"
	"github.com/monaco/monaco/apps/backend/internal/platform/bus"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/multichain"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
	"github.com/monaco/monaco/apps/backend/internal/testkit/privyfake"
)

type treasuryEVMOnCreate struct {
	pool    *pgxpool.Pool
	uow     *db.UnitOfWork
	handler adapters.TreasuryEVMWalletOnCreate
	at      time.Time
}

type cabalCreated struct {
	delivery bus.Delivery
	event    events.CabalCreated
}

func newTreasuryEVMOnCreate(t *testing.T, evm multichain.EVMWallets) treasuryEVMOnCreate {
	t.Helper()
	pool := testkit.DB(t)
	clk := testkit.NewClock(time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC))
	uow := db.New(pool, testkit.NewIDs(5431), clk)
	return treasuryEVMOnCreate{
		pool: pool, uow: uow, at: clk.Now(),
		handler: adapters.TreasuryEVMWalletOnCreate{
			UoW: uow, Wallets: adapters.TreasuryEVMWallets{DB: pool, Wallets: evm, Clock: clk},
		},
	}
}

func (s treasuryEVMOnCreate) create(t *testing.T) cabalCreated {
	t.Helper()
	c := testkit.NewCabal(t, s.pool)
	e := events.CabalCreated{
		V: 1, CabalID: c.ID.UUID(), CreatorID: c.Creator.ID.UUID(), Name: "evm", TreasuryAddress: c.TreasuryAddress,
	}
	ctx := observability.WithActor(t.Context(), "system:test")
	if err := s.uow.Do(ctx, func(ctx context.Context, tx db.Tx) error { return tx.Events.Append(ctx, e) }); err != nil {
		t.Fatal(err)
	}
	var id uuid.UUID
	if err := s.pool.QueryRow(t.Context(), `SELECT id FROM events WHERE aggregate_id = $1`, c.ID.UUID()).
		Scan(&id); err != nil {
		t.Fatal(err)
	}
	return cabalCreated{
		delivery: bus.Delivery{Handler: "cabal.evm_wallet", EventID: ids.EventIDFrom(id), At: s.at},
		event:    e,
	}
}

func (s treasuryEVMOnCreate) deliver(t *testing.T, c cabalCreated) error {
	t.Helper()
	return s.handler.Handle(t.Context(), c.delivery, c.event)
}

func (s treasuryEVMOnCreate) recorded(t *testing.T, c cabalCreated) string {
	t.Helper()
	var code string
	err := s.pool.QueryRow(t.Context(), `SELECT code FROM event_deliveries WHERE handler = $1 AND event_id = $2`,
		c.delivery.Handler, c.delivery.EventID.UUID()).Scan(&code)
	if errors.Is(err, pgx.ErrNoRows) {
		return ""
	}
	if err != nil {
		t.Fatal(err)
	}
	return code
}

func (s treasuryEVMOnCreate) stored(t *testing.T, c cabalCreated) []string {
	t.Helper()
	rows, err := s.pool.Query(t.Context(), `SELECT address FROM treasury_evm_wallets WHERE cabal_id = $1`,
		c.event.CabalID)
	if err != nil {
		t.Fatal(err)
	}
	got, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		t.Fatal(err)
	}
	return got
}

func TestTreasuryEVMWalletOnCreate_storesOneWalletAcrossRedeliveries(t *testing.T) {
	t.Parallel()
	wallets := &privyfake.EVMWallets{}
	s := newTreasuryEVMOnCreate(t, wallets)
	c := s.create(t)
	for range 2 {
		if err := s.deliver(t, c); err != nil {
			t.Fatal(err)
		}
	}
	if got := s.stored(t, c); len(got) != 1 || s.recorded(t, c) != "ok" || wallets.Creates() != 1 {
		t.Fatalf("stored %v, recorded %q after %d creates; want one wallet, ok and one create", got,
			s.recorded(t, c), wallets.Creates())
	}
}

func TestTreasuryEVMWalletOnCreate_aPrivyOutageNaksAndStoresNothing(t *testing.T) {
	t.Parallel()
	wallets := &privyfake.EVMWallets{}
	wallets.Fail("CreateAppEVMWallet", errs.New(errs.CodePrivyUnavailable, "test"))
	s := newTreasuryEVMOnCreate(t, wallets)
	c := s.create(t)
	err := s.deliver(t, c)
	if errs.VerdictFor(errs.CodeOf(err)) != errs.VerdictNak || s.recorded(t, c) != "" || len(s.stored(t, c)) != 0 {
		t.Fatalf("handle = %v, recorded %q, stored %v; want a nak and nothing stored", err, s.recorded(t, c),
			s.stored(t, c))
	}
}

func TestTreasuryEVMWalletOnCreate_acksWhenPrivyIsNotWired(t *testing.T) {
	t.Parallel()
	s := newTreasuryEVMOnCreate(t, multichain.Pending{Ticket: "#5431"})
	c := s.create(t)
	if err := s.deliver(t, c); err != nil || s.recorded(t, c) != string(errs.CodeNotImplemented) {
		t.Fatalf("handle = %v, recorded %q; want not_implemented", err, s.recorded(t, c))
	}
}
