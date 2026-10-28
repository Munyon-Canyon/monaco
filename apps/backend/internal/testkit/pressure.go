package testkit

import (
	"context"
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
	pressureMargin   = 5 * time.Second
)

func PressureContext(tb testing.TB) (context.Context, context.CancelFunc) {
	tb.Helper()
	deadline, ok := tb.(interface{ Deadline() (time.Time, bool) })
	if !ok {
		return context.WithTimeout(tb.Context(), PressureDeadline)
	}
	if at, set := deadline.Deadline(); set {
		return context.WithDeadline(tb.Context(), at.Add(-pressureMargin))
	}
	return context.WithTimeout(tb.Context(), PressureDeadline)
}

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
	pool  *pgxpool.Pool
	reads atomic.Int64
}

func StartProbe(ctx context.Context, pool *pgxpool.Pool) *Probe {
	return startProbe(ctx, pool, func(ctx context.Context) error {
		var one int
		return pool.QueryRow(ctx, `SELECT 1`).Scan(&one)
	})
}

func startProbe(ctx context.Context, pool *pgxpool.Pool, read func(context.Context) error) *Probe {
	ctx, stop := context.WithCancel(ctx)
	p := &Probe{stop: stop, pool: pool}
	first := make(chan struct{})
	p.group.Go(func() error {
		defer func() {
			select {
			case <-first:
			default:
				close(first)
			}
		}()
		return p.loop(ctx, read, first)
	})
	<-first
	return p
}

func (p *Probe) loop(ctx context.Context, read func(context.Context) error, first chan struct{}) error {
	for ctx.Err() == nil {
		if err := read(ctx); err != nil {
			return probeReadError(ctx, err)
		}
		if p.reads.Add(1) == 1 {
			close(first)
		}
		select {
		case <-ctx.Done():
		case <-clock.Real{}.After(pressureProbeGap):
		}
	}
	return nil
}

func probeReadError(ctx context.Context, err error) error {
	if ctx.Err() == nil {
		return fmt.Errorf("probe read: %w", err)
	}
	return nil
}

func (p *Probe) Check(tb testing.TB) {
	tb.Helper()
	p.stop()
	err := p.group.Wait()
	if err != nil || p.reads.Load() == 0 {
		stat := p.pool.Stat()
		tb.Errorf(
			"unrelated reads = %d (error %v), want one while the callers ran; "+
				"pool acquired=%d idle=%d total=%d max=%d empty-acquires=%d canceled-acquires=%d",
			p.reads.Load(), err, stat.AcquiredConns(), stat.IdleConns(), stat.TotalConns(), stat.MaxConns(),
			stat.EmptyAcquireCount(), stat.CanceledAcquireCount())
	}
}

func CheckNoNestedReads(tb testing.TB, guard *db.NestedReadGuard) {
	tb.Helper()
	if found := guard.Found(); len(found) != 0 {
		tb.Errorf("pool queries made while a transaction held a connection: %+v", found)
	}
}
