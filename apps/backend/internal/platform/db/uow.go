package db

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/db/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/faultpoint"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability/boundary"
)

const (
	opDo          = "db.UnitOfWork.Do"
	maxAttempts   = 4
	retryBase     = 10 * time.Millisecond
	finishTimeout = 10 * time.Second
)

type UnitOfWork struct {
	pool   *pgxpool.Pool
	ids    ids.Generator
	clock  clock.Clock
	signal chan struct{}
}

type Tx struct {
	tx     pgx.Tx
	Events *Events
	after  *[]func(context.Context)
}

func (t Tx) Queries() sqlc.DBTX { return t.tx }

func (t Tx) AfterCommit(fn func(ctx context.Context)) { *t.after = append(*t.after, fn) }

func New(pool *pgxpool.Pool, g ids.Generator, c clock.Clock) *UnitOfWork {
	return &UnitOfWork{pool: pool, ids: g, clock: c, signal: make(chan struct{}, 1)}
}

func (u *UnitOfWork) Signal() <-chan struct{} { return u.signal }

func (u *UnitOfWork) Do(ctx context.Context, fn func(ctx context.Context, tx Tx) error) error {
	for attempt := 1; ; attempt++ {
		committed, err, rbErr := u.run(ctx, fn, attempt)
		if err == nil {
			select {
			case u.signal <- struct{}{}:
			default:
			}
			observability.Info(ctx, observability.TxCommitted,
				slog.Any("event_ids", committed.appended), slog.Int("attempt", attempt))
			for _, after := range committed.after {
				after(ctx)
			}
			return nil
		}
		err = classify(err, opDo)
		code := string(errs.CodeOf(err))
		observability.Info(ctx, observability.TxRolledBack,
			slog.String("code", code), slog.Int("attempt", attempt), slog.Any("rollback_error", rbErr))
		if !transient(err) || attempt == maxAttempts {
			return withRollback(err, rbErr)
		}
		delay := backoff(attempt)
		boundary.Warn(ctx, observability.TxRetry,
			slog.String("code", code), slog.Int("attempt", attempt), slog.Duration("delay", delay))
		select {
		case <-u.clock.After(delay):
		case <-ctx.Done():
			return errs.Wrap(context.Cause(ctx), errs.CodeDBUnavailable, opDo)
		}
	}
}

func backoff(attempt int) time.Duration {
	return retryBase<<(attempt-1) + rand.N(retryBase)
}

type committed struct {
	appended []uuid.UUID
	after    []func(context.Context)
}

func (u *UnitOfWork) run(
	ctx context.Context, fn func(ctx context.Context, tx Tx) error, attempt int,
) (committed, error, error) {
	pgtx, err := u.pool.Begin(ctx)
	if err != nil {
		return committed{}, classify(err, opDo), nil
	}
	var after []func(context.Context)
	tx := Tx{tx: pgtx, Events: &Events{q: sqlc.New(pgtx), ids: u.ids, clock: u.clock}, after: &after}
	defer func() {
		r := recover()
		if r == nil {
			return
		}
		if faultpoint.IsCrash(r) {
			observability.Info(ctx, observability.TxCrashed,
				slog.String("code", string(errs.CodeFaultpoint)), slog.Int("attempt", attempt))
		} else {
			observability.Info(ctx, observability.TxRolledBack,
				slog.String("code", string(errs.CodePanic)), slog.Int("attempt", attempt))
		}
		rollbackAndRepanic(ctx, pgtx, r)
	}()
	if err := fn(ctx, tx); err != nil {
		return committed{}, classify(err, opDo), rollback(ctx, pgtx)
	}
	if err := ctx.Err(); err != nil {
		return committed{}, classify(err, opDo), rollback(ctx, pgtx)
	}
	faultpoint.Hit(ctx, faultpoint.BeforeCommit)
	finishCtx, cancel := finishContext(ctx)
	defer cancel()
	if err := pgtx.Commit(finishCtx); err != nil {
		return committed{}, classify(err, opDo), rollback(ctx, pgtx)
	}
	return committed{appended: tx.Events.appended, after: after}, nil, nil
}

func finishContext(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.WithoutCancel(ctx), finishTimeout)
}

func rollback(ctx context.Context, tx pgx.Tx) error {
	finishCtx, cancel := finishContext(ctx)
	defer cancel()
	err := tx.Rollback(finishCtx)
	if err == nil || errors.Is(err, pgx.ErrTxClosed) {
		return nil
	}
	return classify(err, "db.UnitOfWork.rollback")
}

func rollbackAndRepanic(ctx context.Context, tx pgx.Tx, r any) {
	if err := rollback(ctx, tx); err != nil && !faultpoint.IsCrash(r) {
		panic(fmt.Sprintf("%v (rollback: %v)", r, err))
	}
	panic(r)
}

func withRollback(err, rbErr error) error {
	if rbErr == nil {
		return err
	}
	return errors.Join(err, rbErr)
}
