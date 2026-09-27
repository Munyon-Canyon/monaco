package db_test

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
)

const advance = `UPDATE things SET status = 'submitted' WHERE id = $1 AND status = 'created'`

func TestGuardedUpdate_reportsWhetherThisCallerWonTheTransition(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	ctx := h.ctx(t, "user:u1")
	err := h.uow.Do(ctx, func(ctx context.Context, tx db.Tx) error { return h.insertThing(ctx, tx, 1) })
	if err != nil {
		t.Fatal(err)
	}
	var wins []bool
	for range 2 {
		err := h.uow.Do(ctx, func(ctx context.Context, tx db.Tx) error {
			ok, err := db.GuardedUpdate(ctx, tx.Queries(), advance, 1)
			wins = append(wins, ok)
			return err
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	if len(wins) != 2 || !wins[0] || wins[1] {
		t.Fatalf("wins = %v, want the first caller to win and the second to lose", wins)
	}
	if h.count(t, "things WHERE status = 'submitted'") != 1 {
		t.Fatal("the winning transition did not commit")
	}
}

func TestGuardedUpdate_refusesAGuardThatMatchesSeveralRows(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	ctx := h.ctx(t, "user:u1")
	err := h.uow.Do(ctx, func(ctx context.Context, tx db.Tx) error {
		if err := h.insertThing(ctx, tx, 1); err != nil {
			return err
		}
		return h.insertThing(ctx, tx, 2)
	})
	if err != nil {
		t.Fatal(err)
	}
	err = h.uow.Do(ctx, func(ctx context.Context, tx db.Tx) error {
		_, err := db.GuardedUpdate(ctx, tx.Queries(), `UPDATE things SET status = 'submitted' WHERE status = 'created'`)
		return err
	})
	attrs := codedError(t, err, errs.CodeInternal)
	if attr(attrs, "rows") != "2" {
		t.Fatalf("err = %v, want rows=2", err)
	}
	if h.count(t, "things WHERE status = 'submitted'") != 0 {
		t.Fatal("the multi-row update was committed")
	}
}

func TestGuardedUpdate_wrapsAFailedStatement(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	_, err := db.GuardedUpdate(h.ctx(t, "user:u1"), h.pool, `UPDATE nowhere SET x = 1 WHERE y = 2`)
	var pg *pgconn.PgError
	if errs.CodeOf(err) != errs.CodeInternal || !errors.As(err, &pg) || pg.Code != "42P01" {
		t.Fatalf("err = %v, want internal wrapping undefined_table", err)
	}
}

func TestCountingTracer_countsEveryQueryAConnectionRuns(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	tracer := &db.CountingTracer{}
	cfg := h.pool.Config()
	cfg.ConnConfig.Tracer = tracer
	traced, err := pgxpool.NewWithConfig(t.Context(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer traced.Close()
	uow := db.New(traced, h.ids, h.clock)
	ctx := h.ctx(t, "user:u1")
	err = uow.Do(ctx, func(ctx context.Context, tx db.Tx) error {
		if err := h.insertThing(ctx, tx, 1); err != nil {
			return err
		}
		return tx.Events.Append(ctx, pinged(h.ids))
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := tracer.Queries(); got != 4 {
		t.Fatalf("Queries() = %d after one transaction, want 4 (begin, insert, append, commit)", got)
	}
	tracer.Reset()
	if _, err := traced.Exec(ctx, `SELECT 1`); err != nil {
		t.Fatal(err)
	}
	if got := tracer.Queries(); got != 1 {
		t.Fatalf("Queries() = %d after Reset and one query, want 1", got)
	}
}
