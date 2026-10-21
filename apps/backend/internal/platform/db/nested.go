package db

import (
	"context"
	"sync"

	"github.com/jackc/pgx/v5"
)

type heldConnKey struct{}

func withHeldConn(ctx context.Context, conn *pgx.Conn) context.Context {
	return context.WithValue(ctx, heldConnKey{}, conn)
}

type NestedRead struct {
	SQL string
}

type NestedReadGuard struct {
	mu    sync.Mutex
	found []NestedRead
}

var _ pgx.QueryTracer = (*NestedReadGuard)(nil)

func (g *NestedReadGuard) TraceQueryStart(
	ctx context.Context,
	conn *pgx.Conn,
	data pgx.TraceQueryStartData,
) context.Context {
	if held, ok := ctx.Value(heldConnKey{}).(*pgx.Conn); ok && held != conn {
		g.mu.Lock()
		g.found = append(g.found, NestedRead{SQL: data.SQL})
		g.mu.Unlock()
	}
	return ctx
}

func (g *NestedReadGuard) TraceQueryEnd(context.Context, *pgx.Conn, pgx.TraceQueryEndData) {}

func (g *NestedReadGuard) Found() []NestedRead {
	g.mu.Lock()
	defer g.mu.Unlock()
	return append([]NestedRead(nil), g.found...)
}
