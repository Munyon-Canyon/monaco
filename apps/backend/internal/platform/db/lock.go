package db

import (
	"context"
	"errors"
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability/boundary"
)

const tryAdvisoryLock = `SELECT pg_try_advisory_lock(hashtext($1))`

type Lock struct {
	pool *pgxpool.Pool
	key  string
	conn *pgxpool.Conn
}

func NewLock(pool *pgxpool.Pool, key string) *Lock {
	return &Lock{pool: pool, key: key}
}

func (l *Lock) Hold(ctx context.Context) (bool, error) {
	var lost error
	if l.conn != nil {
		err := l.conn.Ping(ctx)
		if err == nil {
			return true, nil
		}
		lost = errors.Join(err, l.Release(ctx))
	}
	held, err := l.take(ctx)
	if err != nil {
		return false, errors.Join(err, lost)
	}
	if lost != nil {
		boundary.Warn(ctx, observability.DBLockLost,
			slog.String("lock", l.key), slog.Bool("held", held), slog.Any("err", lost))
	}
	return held, nil
}

func (l *Lock) take(ctx context.Context) (bool, error) {
	const op = "db.Lock.Hold"
	conn, err := l.pool.Acquire(ctx)
	if err != nil {
		return false, classify(err, op)
	}
	var held bool
	if err := conn.QueryRow(ctx, tryAdvisoryLock, l.key).Scan(&held); err != nil {
		conn.Release()
		return false, classify(err, op)
	}
	if !held {
		conn.Release()
		return false, nil
	}
	l.conn = conn
	return true, nil
}

func (l *Lock) Release(ctx context.Context) error {
	if l.conn == nil {
		return nil
	}
	conn := l.conn.Hijack()
	l.conn = nil
	finishCtx, cancel := finishContext(ctx)
	defer cancel()
	_, unlockErr := conn.Exec(finishCtx, `SELECT pg_advisory_unlock_all()`)
	if err := errors.Join(unlockErr, conn.Close(finishCtx)); err != nil {
		return classify(err, "db.Lock.Release")
	}
	return nil
}
