package db

import (
	"context"
	"errors"
	"log/slog"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/platform/config"
	"github.com/monaco/monaco/apps/backend/migrations"
)

const migrateHint = "run: just migrate db"

const latestRevision = `SELECT version FROM atlas_schema_revisions.atlas_schema_revisions ORDER BY version DESC LIMIT 1`

func Open(ctx context.Context, cfg config.DB) (*pgxpool.Pool, error) {
	const op = "db.Open"
	poolCfg, err := pgxpool.ParseConfig(cfg.URL)
	if err != nil {
		return nil, errs.Wrap(err, errs.CodeInternal, op)
	}
	poolCfg.MaxConns = cfg.MaxConns
	pool, err := pgxpool.NewWithConfig(ctx, poolCfg)
	if err != nil {
		return nil, errs.Wrap(err, errs.CodeInternal, op)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, errs.Wrap(err, errs.CodeDBUnavailable, op)
	}
	if err := checkRevision(ctx, pool); err != nil {
		pool.Close()
		return nil, err
	}
	return pool, nil
}

func checkRevision(ctx context.Context, pool *pgxpool.Pool) error {
	const op = "db.Open"
	want := migrations.Latest()
	var have string
	err := pool.QueryRow(ctx, latestRevision).Scan(&have)
	var pg *pgconn.PgError
	unmigrated := errors.As(err, &pg) && pg.Code == pgUndefinedTable
	if err != nil && !unmigrated && !errors.Is(err, pgx.ErrNoRows) {
		return classify(err, op)
	}
	if unmigrated || have < want {
		return errs.Wrap(err, errs.CodeDBSchemaBehind, op,
			slog.String("have", have), slog.String("want", want), slog.String("hint", migrateHint))
	}
	return nil
}
