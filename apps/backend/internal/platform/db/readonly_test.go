package db_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/db/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

func TestReadOnly_ReadsAndRollsBack(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	var timeout string
	err := db.ReadOnly(t.Context(), pool, 5*time.Second, func(ctx context.Context, q sqlc.DBTX) error {
		return q.QueryRow(ctx, `SELECT current_setting('statement_timeout')`).Scan(&timeout)
	})
	if err != nil || timeout != "5s" {
		t.Fatalf("ReadOnly() = %v with statement_timeout %q, want nil and 5s", err, timeout)
	}
	var after string
	if err := pool.QueryRow(t.Context(), `SELECT current_setting('statement_timeout')`).Scan(&after); err != nil ||
		after == "5s" {
		t.Fatalf("statement_timeout after = %q, %v, want the setting to end with the transaction", after, err)
	}
}

func TestReadOnly_RefusesAWrite(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	err := db.ReadOnly(t.Context(), pool, time.Second, func(ctx context.Context, q sqlc.DBTX) error {
		_, err := q.Exec(ctx, `CREATE TABLE read_only_probe (id int)`)
		return err
	})
	var pg *pgconn.PgError
	if !errors.As(err, &pg) || pg.Code != "25006" {
		t.Fatalf("ReadOnly() error = %v, want SQLSTATE 25006", err)
	}
	if db.IsStatementTimeout(err) {
		t.Fatalf("IsStatementTimeout(%v) = true for a read-only violation", err)
	}
}

func TestReadOnly_CancelsASlowStatement(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	err := db.ReadOnly(t.Context(), pool, 50*time.Millisecond, func(ctx context.Context, q sqlc.DBTX) error {
		_, err := q.Exec(ctx, `SELECT pg_sleep(5)`)
		return err
	})
	if !db.IsStatementTimeout(err) {
		t.Fatalf("ReadOnly() error = %v, want a statement timeout", err)
	}
}

func TestIsStatementTimeout_IgnoresOtherCancellations(t *testing.T) {
	t.Parallel()
	timedOut := &pgconn.PgError{Code: "57014", Message: "canceling statement due to statement timeout"}
	for name, err := range map[string]error{
		"nil":             nil,
		"plain":           errors.New("boom"),
		"user cancel":     &pgconn.PgError{Code: "57014", Message: "canceling statement due to user request"},
		"other code":      &pgconn.PgError{Code: "22012", Message: "statement timeout"},
		"wrapped timeout": errs.Wrap(timedOut, errs.CodeInternal, "test"),
	} {
		if got, want := db.IsStatementTimeout(err), name == "wrapped timeout"; got != want {
			t.Errorf("IsStatementTimeout(%s) = %v, want %v", name, got, want)
		}
	}
}

func TestReadOnly_FailsToStartOnACancelledContext(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	err := db.ReadOnly(ctx, testkit.DB(t), time.Second, func(context.Context, sqlc.DBTX) error { return nil })
	if errs.CodeOf(err) != errs.CodeDBUnavailable {
		t.Fatalf("ReadOnly() error = %v, want db_unavailable", err)
	}
}

func TestReadOnly_RefusesATimeoutPostgresRejects(t *testing.T) {
	t.Parallel()
	called := false
	err := db.ReadOnly(t.Context(), testkit.DB(t), -time.Second, func(context.Context, sqlc.DBTX) error {
		called = true
		return nil
	})
	if err == nil || called {
		t.Fatalf("ReadOnly() = %v, called %v, want an error before the callback", err, called)
	}
}
