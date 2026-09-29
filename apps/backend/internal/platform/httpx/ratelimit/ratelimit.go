package ratelimit

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/db/sqlc"
)

const (
	milli    = 1000
	maxRate  = 100_000
	maxBurst = 100_000
	maxFill  = 24 * time.Hour
)

type Policy struct {
	Rate  int64
	Per   time.Duration
	Burst int64
}

type Decision struct {
	Allowed    bool
	RetryAfter time.Duration
}

type Limiter struct {
	q     *sqlc.Queries
	clock clock.Clock
}

func New(db sqlc.DBTX, c clock.Clock) *Limiter {
	return &Limiter{q: sqlc.New(db), clock: c}
}

func (l *Limiter) Take(ctx context.Context, key string, p Policy, cost int64) (Decision, error) {
	const op = "ratelimit.Take"
	if !p.valid() || cost < 1 || cost > p.Burst {
		return Decision{}, errs.New(errs.CodeInvalidConfig, op, slog.String("key", key),
			slog.Int64("rate", p.Rate), slog.Duration("per", p.Per), slog.Int64("burst", p.Burst),
			slog.Int64("cost", cost))
	}
	now := l.clock.Now()
	_, err := l.q.TakeRateLimitTokens(ctx, sqlc.TakeRateLimitTokensParams{
		Key: key, BurstMilli: p.Burst * milli, CostMilli: cost * milli, Now: now,
		RateMilli: p.Rate * milli, PerMicros: p.Per.Microseconds(),
	})
	if err == nil {
		return Decision{Allowed: true}, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return Decision{}, errs.Wrap(err, errs.CodeDBUnavailable, op, slog.String("key", key))
	}
	bucket, err := l.q.GetRateLimitBucket(ctx, key)
	if err != nil {
		return Decision{}, errs.Wrap(err, errs.CodeDBUnavailable, op, slog.String("key", key))
	}
	ready := bucket.UpdatedAt.Add(p.refillTime(cost*milli - bucket.TokensMilli))
	return Decision{RetryAfter: max(ready.Sub(now), 0)}, nil
}

func (p Policy) valid() bool {
	return p.Rate >= 1 && p.Rate <= maxRate && p.Burst >= 1 && p.Burst <= maxBurst &&
		p.Per >= time.Microsecond && p.Per <= maxFill && p.refillTime(p.Burst*milli) <= maxFill
}

func (p Policy) refillTime(tokensMilli int64) time.Duration {
	rateMilli := p.Rate * milli
	micros := (tokensMilli*p.Per.Microseconds() + rateMilli - 1) / rateMilli
	return time.Duration(micros) * time.Microsecond
}
