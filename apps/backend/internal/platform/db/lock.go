package db

import (
	"context"
	"errors"
	"log/slog"
	"sync"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability/boundary"
)

const (
	tryAdvisoryLock = `SELECT pg_try_advisory_lock(hashtext($1))`
	advisoryUnlock  = `SELECT pg_advisory_unlock(hashtext($1))`
	holdOp          = "db.Locks.Hold"
)

type Locks struct {
	pool *pgxpool.Pool
	mu   sync.Mutex
	conn *pgxpool.Conn
	held map[string]bool
	lost map[string]error
}

func NewLocks(pool *pgxpool.Pool) *Locks {
	return &Locks{pool: pool, held: map[string]bool{}, lost: map[string]error{}}
}

func (l *Locks) Hold(ctx context.Context, key string) (bool, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return false, classify(err, holdOp)
	}
	if l.conn != nil {
		if err := l.conn.Ping(ctx); err != nil {
			l.drop(ctx, err)
		} else if l.held[key] {
			return true, nil
		}
	}
	held, err := l.take(ctx, key)
	lost, wasLost := l.lost[key]
	delete(l.lost, key)
	if err != nil {
		return false, errors.Join(err, lost)
	}
	if wasLost {
		boundary.Warn(ctx, observability.DBLockLost,
			slog.String("lock", key), slog.Bool("held", held), slog.Any("err", lost))
	}
	return held, nil
}

func (l *Locks) take(ctx context.Context, key string) (bool, error) {
	if l.conn == nil {
		conn, err := l.pool.Acquire(ctx)
		if err != nil {
			return false, classify(err, holdOp)
		}
		l.conn = conn
	}
	var held bool
	if err := l.conn.QueryRow(ctx, tryAdvisoryLock, key).Scan(&held); err != nil {
		l.drop(ctx, err)
		return false, classify(err, holdOp)
	}
	if held {
		l.held[key] = true
	}
	l.releaseIdle()
	return held, nil
}

func (l *Locks) Release(ctx context.Context, key string) error {
	const op = "db.Locks.Release"
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.lost, key)
	if !l.held[key] {
		return nil
	}
	delete(l.held, key)
	finishCtx, cancel := finishContext(ctx)
	defer cancel()
	if _, err := l.conn.Exec(finishCtx, advisoryUnlock, key); err != nil {
		l.drop(ctx, err)
		return classify(err, op)
	}
	l.releaseIdle()
	return nil
}

func (l *Locks) drop(ctx context.Context, cause error) {
	conn := l.conn.Hijack()
	l.conn = nil
	finishCtx, cancel := finishContext(ctx)
	defer cancel()
	cause = errors.Join(cause, conn.Close(finishCtx))
	for key := range l.held {
		l.lost[key] = cause
	}
	clear(l.held)
}

func (l *Locks) releaseIdle() {
	if l.conn != nil && len(l.held) == 0 {
		l.conn.Release()
		l.conn = nil
	}
}
