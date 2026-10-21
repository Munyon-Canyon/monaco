package db_test

import (
	"context"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/monaco/monaco/apps/backend/internal/platform/db"
)

func tracedHarness(t *testing.T) (*harness, *pgxpool.Pool, *db.NestedReadGuard) {
	t.Helper()
	h := newHarness(t)
	guard := &db.NestedReadGuard{}
	cfg := h.pool.Config()
	cfg.ConnConfig.Tracer = guard
	traced, err := pgxpool.NewWithConfig(t.Context(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(traced.Close)
	h.uow = db.New(traced, h.ids, h.clock)
	return h, traced, guard
}

func TestNestedReadGuard_flagsAPoolQueryMadeWhileATransactionIsOpen(t *testing.T) {
	t.Parallel()
	h, pool, guard := tracedHarness(t)
	err := h.uow.Do(h.ctx(t, "user:u1"), func(ctx context.Context, tx db.Tx) error {
		if err := h.insertThing(ctx, tx, 1); err != nil {
			return err
		}
		_, err := pool.Exec(ctx, `SELECT 42`)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	found := guard.Found()
	if len(found) != 1 || !strings.Contains(found[0].SQL, "SELECT 42") {
		t.Fatalf("Found() = %+v, want the one pool query made inside the transaction", found)
	}
}

func TestNestedReadGuard_flagsASecondTransactionOpenedInsideTheFirst(t *testing.T) {
	t.Parallel()
	h, _, guard := tracedHarness(t)
	err := h.uow.Do(h.ctx(t, "user:u1"), func(ctx context.Context, _ db.Tx) error {
		return h.uow.Do(ctx, func(ctx context.Context, tx db.Tx) error { return h.insertThing(ctx, tx, 2) })
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(guard.Found()) == 0 {
		t.Fatal("Found() is empty after a transaction opened inside a transaction")
	}
}

func TestNestedReadGuard_ignoresTransactionQueriesAndQueriesOutsideOne(t *testing.T) {
	t.Parallel()
	h, pool, guard := tracedHarness(t)
	ctx := h.ctx(t, "user:u1")
	if _, err := pool.Exec(ctx, `SELECT 1`); err != nil {
		t.Fatal(err)
	}
	err := h.uow.Do(ctx, func(ctx context.Context, tx db.Tx) error {
		if err := h.insertThing(ctx, tx, 3); err != nil {
			return err
		}
		_, err := tx.Queries().Exec(ctx, `SELECT 2`)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `SELECT 3`); err != nil {
		t.Fatal(err)
	}
	if found := guard.Found(); len(found) != 0 {
		t.Fatalf("Found() = %+v, want none", found)
	}
}
