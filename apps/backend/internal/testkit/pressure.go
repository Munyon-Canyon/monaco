package testkit

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/sync/errgroup"

	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
)

const (
	PressureConns    = 4
	PressureCallers  = 6
	PressureDeadline = 30 * time.Second
	pressureProbeGap = 5 * time.Millisecond
	pressureProbeMax = 3 * time.Second
)

func TracedPool(tb testing.TB, pool *pgxpool.Pool) (*pgxpool.Pool, *db.NestedReadGuard) {
	tb.Helper()
	guard := &db.NestedReadGuard{}
	cfg := pool.Config()
	cfg.ConnConfig.Tracer = guard
	traced, err := pgxpool.NewWithConfig(tb.Context(), cfg)
	if err != nil {
		tb.Fatalf("testkit.TracedPool: %v", err)
	}
	tb.Cleanup(traced.Close)
	if got := traced.Config().MaxConns; got != PressureConns {
		tb.Fatalf("testkit.TracedPool: MaxConns = %d, want %d as Render runs it", got, PressureConns)
	}
	return traced, guard
}

type Gate struct {
	mu   sync.Mutex
	left int
	open chan struct{}
}

func NewGate(callers int) *Gate { return &Gate{left: callers, open: make(chan struct{})} }

func (g *Gate) Arrive() {
	if g == nil {
		return
	}
	g.mu.Lock()
	g.left--
	if g.left == 0 {
		close(g.open)
	}
	g.mu.Unlock()
	select {
	case <-g.open:
	case <-clock.Real{}.After(PressureDeadline):
	}
}

func Concurrently(tb testing.TB, jobs ...func() error) {
	tb.Helper()
	var mu sync.Mutex
	var failures []error
	var group errgroup.Group
	for i, job := range jobs {
		group.Go(func() error {
			if err := job(); err != nil {
				mu.Lock()
				failures = append(failures, fmt.Errorf("job %d: %w", i, err))
				mu.Unlock()
			}
			return nil
		})
	}
	_ = group.Wait()
	for _, err := range failures {
		tb.Error(err)
	}
}

type Probe struct {
	stop  context.CancelFunc
	group errgroup.Group
	reads atomic.Int64
	worst atomic.Int64
}

func StartProbe(ctx context.Context, pool *pgxpool.Pool) *Probe {
	ctx, stop := context.WithCancel(ctx)
	p := &Probe{stop: stop}
	p.group.Go(func() error {
		for ctx.Err() == nil {
			start := clock.Real{}.Now()
			var one int
			if err := pool.QueryRow(ctx, `SELECT 1`).Scan(&one); err != nil {
				return fmt.Errorf("probe read: %w", err)
			}
			p.reads.Add(1)
			took := clock.Real{}.Now().Sub(start).Nanoseconds()
			if took > p.worst.Load() {
				p.worst.Store(took)
			}
			select {
			case <-ctx.Done():
			case <-clock.Real{}.After(pressureProbeGap):
			}
		}
		return nil
	})
	return p
}

func (p *Probe) Check(tb testing.TB) {
	tb.Helper()
	p.stop()
	err := p.group.Wait()
	worst := time.Duration(p.worst.Load())
	if (err != nil && !errors.Is(err, context.Canceled)) || p.reads.Load() == 0 || worst > pressureProbeMax {
		tb.Errorf("unrelated reads = %d (error %v), slowest %s, want steady progress under %s",
			p.reads.Load(), err, worst, pressureProbeMax)
	}
}

func CheckNoNestedReads(tb testing.TB, guard *db.NestedReadGuard) {
	tb.Helper()
	if found := guard.Found(); len(found) != 0 {
		tb.Errorf("pool queries made while a transaction held a connection: %+v", found)
	}
}
