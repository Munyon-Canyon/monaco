package poller

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"runtime/debug"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/platform/auth"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/faultpoint"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability/boundary"
)

const lockTimeout = 5 * time.Second

type Poller interface {
	Name() string
	Interval() time.Duration
	Tick(ctx context.Context) (Report, error)
}

type Report struct {
	Scanned, Changed int
	Attrs            []slog.Attr
}

type Runner struct {
	pool   *pgxpool.Pool
	clock  clock.Clock
	errors metric.Int64Counter

	mu   sync.Mutex
	last map[string]time.Time
}

func NewRunner(pool *pgxpool.Pool, c clock.Clock, meter metric.Meter) (*Runner, error) {
	counter, err := meter.Int64Counter("poller_errors_total",
		metric.WithDescription("Poller ticks that ended in an error, by poller and error code."))
	if err != nil {
		return nil, errs.Wrap(err, errs.CodeInternal, "poller.NewRunner")
	}
	return &Runner{pool: pool, clock: c, errors: counter, last: map[string]time.Time{}}, nil
}

func (r *Runner) Run(ctx context.Context, pollers ...Poller) error {
	seen := map[string]bool{}
	for _, p := range pollers {
		if seen[p.Name()] {
			panic("poller.Runner.Run: duplicate poller " + p.Name())
		}
		seen[p.Name()] = true
	}
	if maxConns := int(r.pool.Config().MaxConns); maxConns <= len(pollers) {
		panic(fmt.Sprintf("poller.Runner.Run: pool MaxConns %d must exceed the %d pollers that each hold a connection",
			maxConns, len(pollers)))
	}
	released := make([]error, len(pollers))
	var wg sync.WaitGroup
	for i, p := range pollers {
		wg.Go(func() { released[i] = r.loop(ctx, p) })
	}
	wg.Wait()
	return errors.Join(released...)
}

func (r *Runner) LastTick(name string) time.Time {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.last[name]
}

func (r *Runner) loop(ctx context.Context, p Poller) error {
	lock := db.NewLock(r.pool, "poller:"+p.Name())
	ticker := r.clock.NewTicker(p.Interval())
	defer ticker.Stop()
	for {
		r.attempt(ctx, p, lock)
		select {
		case <-ctx.Done():
			return lock.Release(ctx)
		case <-ticker.C():
		}
	}
}

func (r *Runner) attempt(ctx context.Context, p Poller, lock *db.Lock) {
	name := p.Name()
	ctx = faultpoint.WithFlow(ctx, flow(name))
	defer recoverAttempt(ctx, name)
	r.mark(name, r.clock.Now())
	lockCtx, cancelLock := context.WithTimeout(ctx, lockTimeout)
	held, err := lock.Hold(lockCtx)
	cancelLock()
	if err != nil {
		r.fail(ctx, name, err)
		return
	}
	if !held {
		observability.Debug(ctx, observability.PollerSkipped, slog.String("poller", name))
		return
	}
	start := r.clock.Now()
	actor := auth.Actor{Kind: auth.ActorSystem, ID: "poller." + name}
	tickCtx, cancelTick := context.WithTimeout(auth.WithActor(ctx, actor), p.Interval())
	report, err := tick(tickCtx, p)
	cancelTick()
	if err != nil {
		r.fail(ctx, name, deadlineAsTimeout(err))
		return
	}
	observability.Info(ctx, observability.PollerTick, slog.String("poller", name),
		slog.Int("scanned", report.Scanned), slog.Int("changed", report.Changed),
		slog.Int64("duration_ms", r.clock.Now().Sub(start).Milliseconds()), slog.GroupAttrs("detail", report.Attrs...))
}

func deadlineAsTimeout(err error) error {
	if errs.KindOf(errs.CodeOf(err)) == errs.KindInternal && errors.Is(err, context.DeadlineExceeded) {
		return errs.Wrap(err, errs.CodeUpstreamTimeout, "poller.tick")
	}
	return err
}

func flow(poller string) string {
	switch poller {
	case "funding.deposits":
		return "05"
	case "funding.withdrawals":
		return "15"
	case "funding.treasury-reconcile", "funding.bounce-sweeper":
		return "08"
	case "market.prices":
		return "18"
	case "identity.nudges":
		return "28"
	default:
		return ""
	}
}

func recoverAttempt(ctx context.Context, name string) {
	v := recover()
	if v == nil {
		return
	}
	if !faultpoint.IsCrash(v) {
		panic(v)
	}
	observability.Info(ctx, observability.PollerCrashed,
		slog.String("poller", name), slog.String("code", string(errs.CodeFaultpoint)))
	panic(v)
}

func tick(ctx context.Context, p Poller) (report Report, err error) {
	defer func() {
		if v := recover(); v != nil {
			if faultpoint.IsCrash(v) {
				panic(v)
			}
			err = errs.New(errs.CodePanic, "poller.tick", slog.String("panic", fmt.Sprint(v)),
				slog.String("stack", string(debug.Stack())))
		}
	}()
	return p.Tick(ctx)
}

func (r *Runner) fail(ctx context.Context, name string, err error) {
	if ctx.Err() != nil {
		return
	}
	code := errs.CodeOf(err)
	r.errors.Add(
		ctx,
		1,
		metric.WithAttributes(attribute.String("poller", name), attribute.String("code", string(code))),
	)
	detail := errs.Detail(err)
	boundary.Error(ctx, observability.PollerFailed, slog.String("poller", name), slog.String("code", string(code)),
		slog.Any("err", err), slog.Bool("alert", errs.Alert(code)), slog.GroupAttrs("detail", detail...))
}

func (r *Runner) mark(name string, at time.Time) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.last[name] = at
}
