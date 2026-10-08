package db

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/monaco/monaco/apps/backend/internal/platform/db/sqlc"
)

const (
	opReadOnly         = "db.ReadOnly"
	pgQueryCanceled    = "57014"
	statementTimeoutIn = "statement timeout"
)

func ReadOnly(
	ctx context.Context, pool *pgxpool.Pool, timeout time.Duration, fn func(ctx context.Context, q sqlc.DBTX) error,
) error {
	tx, err := pool.BeginTx(ctx, pgx.TxOptions{AccessMode: pgx.ReadOnly})
	if err != nil {
		return classify(err, opReadOnly)
	}
	err = readOnlyRun(ctx, tx, timeout, fn)
	return withRollback(err, rollback(ctx, tx))
}

func readOnlyRun(
	ctx context.Context, tx pgx.Tx, timeout time.Duration, fn func(ctx context.Context, q sqlc.DBTX) error,
) error {
	const setTimeout = `SELECT set_config('statement_timeout', $1, true)`
	if _, err := tx.Exec(ctx, setTimeout, strconv.FormatInt(timeout.Milliseconds(), 10)+"ms"); err != nil {
		return classify(err, opReadOnly)
	}
	return fn(ctx, tx)
}

func IsStatementTimeout(err error) bool {
	var pg *pgconn.PgError
	return errors.As(err, &pg) && pg.Code == pgQueryCanceled && strings.Contains(pg.Message, statementTimeoutIn)
}
