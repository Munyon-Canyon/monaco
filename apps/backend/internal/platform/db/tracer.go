package db

import (
	"context"
	"sync/atomic"

	"github.com/jackc/pgx/v5"
)

type CountingTracer struct {
	queries atomic.Int64
}

var _ pgx.QueryTracer = (*CountingTracer)(nil)

func (t *CountingTracer) TraceQueryStart(ctx context.Context, _ *pgx.Conn, _ pgx.TraceQueryStartData) context.Context {
	t.queries.Add(1)
	return ctx
}

func (t *CountingTracer) TraceQueryEnd(context.Context, *pgx.Conn, pgx.TraceQueryEndData) {}

func (t *CountingTracer) Queries() int64 { return t.queries.Load() }

func (t *CountingTracer) Reset() { t.queries.Store(0) }
