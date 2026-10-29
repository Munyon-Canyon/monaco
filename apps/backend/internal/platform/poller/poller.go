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

const (
	lockTimeout     = 5 * time.Second
	minBackoff      = 5 * time.Second
	defaultMaxWait  = 5 * time.Minute
	maxBackoffShift = 20
)

type outcome int

const (
	outcomeOK outcome = iota
	outcomeSkipped
	outcomeFailed
)

type Poller interface {
	Name() string
	Interval() time.Duration
	Tick(ctx context.Context) (Report, error)
}

type Budgeted interface {
	TickBudget() time.Duration
}

type Report struct {
	Scanned, Changed int
	Attrs            []slog.Attr
}

type Runner struct {
	pool   *pgxpool.Pool
	clock  clock.Clock
	errors metric.Int64Counter

	maxBackoff time.Duration

	mu      sync.Mutex
	last    map[string]time.Time
	success map[string]time.Time
	names   []string
}

func NewRunner(pool *pgxpool.Pool, c clock.Clock, meter metric.Meter) (*Runner, error) {
	counter, err := meter.Int64Counter("poller_errors_total",
		metric.WithDescription("Poller ticks that ended in an error, by poller and error code."))
	if err != nil {
		return nil, errs.Wrap(err, errs.CodeInternal, "poller.NewRunner")
	}
	r := &Runner{
		pool: pool, clock: c, errors: counter, maxBackoff: defaultMaxWait,
		last: map[string]time.Time{}, success: map[string]time.Time{},
	}
	if err := r.exportSuccess(meter); err != nil {
		return nil, err
	}
	return r, nil
}

func (r *Runner) exportSuccess(meter metric.Meter) error {
	const op = "poller.NewRunner"
	gauge, err := meter.Int64ObservableGauge("poller_last_success_timestamp_seconds",
		metric.WithUnit("s"),
		metric.WithDescription("Unix time of the last tick of a poller that ran to completion; 0 before the first."))
	if err != nil {
		return errs.Wrap(err, errs.CodeInternal, op)
	}
	_, err = meter.RegisterCallback(func(_ context.Context, o metric.Observer) error {
		r.mu.Lock()
		defer r.mu.Unlock()
		for _, name := range r.names {
			var at int64
			if last := r.success[name]; !last.IsZero() {
				at = last.Unix()
			}
			o.ObserveInt64(gauge, at, metric.WithAttributes(attribute.String("poller", name)))
		}
		return nil
	}, gauge)
	if err != nil {
		return errs.Wrap(err, errs.CodeInternal, op)
	}
	return nil
}

func (r *Runner) WithBackoffMax(d time.Duration) *Runner {
	r.maxBackoff = d
	return r
}

func (r *Runner) Run(ctx context.Context, pollers ...Poller) error {
	seen := map[string]bool{}
	for _, p := range pollers {
		if seen[p.Name()] {
			panic("poller.Runner.Run: duplicate poller " + p.Name())
		}
		seen[p.Name()] = true
	}
	r.register(pollers)
	locks := db.NewLocks(r.pool)
	released := make([]error, len(pollers))
	var wg sync.WaitGroup
	for i, p := range pollers {
		wg.Go(func() { released[i] = r.loop(ctx, p, locks) })
	}
	wg.Wait()
	return errors.Join(released...)
}

func (r *Runner) register(pollers []Poller) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, p := range pollers {
		r.names = append(r.names, p.Name())
	}
}

func (r *Runner) LastTick(name string) time.Time {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.last[name]
}

func (r *Runner) LastSuccess(name string) time.Time {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.success[name]
}

func (r *Runner) loop(ctx context.Context, p Poller, locks *db.Locks) error {
	ticker := r.clock.NewTicker(p.Interval())
	defer func() { ticker.Stop() }()
	failures := 0
	for {
		wait := ticker.C()
		backedOff := false
		if r.attempt(ctx, p, locks, r.backoff(p.Interval(), failures+1)) == outcomeFailed {
			failures++
			wait = r.clock.After(r.backoff(p.Interval(), failures))
			backedOff = true
		} else {
			failures = 0
		}
		select {
		case <-ctx.Done():
			return locks.Release(ctx, lockKey(p))
		case <-wait:
		}
		if backedOff {
			ticker.Stop()
			ticker = r.clock.NewTicker(p.Interval())
		}
	}
}

func (r *Runner) backoff(interval time.Duration, failures int) time.Duration {
	d := max(interval, minBackoff)
	for range min(failures-1, maxBackoffShift) {
		d = min(d*2, r.maxBackoff)
	}
	return min(d, r.maxBackoff)
}

func lockKey(p Poller) string { return "poller:" + p.Name() }

func (r *Runner) attempt(ctx context.Context, p Poller, locks *db.Locks, retryIn time.Duration) outcome {
	name := p.Name()
	ctx = faultpoint.WithFlow(ctx, flow(name))
	defer recoverAttempt(ctx, name)
	lockCtx, cancelLock := context.WithTimeout(ctx, lockTimeout)
	held, err := locks.Hold(lockCtx, lockKey(p))
	cancelLock()
	if err != nil {
		r.fail(ctx, name, err, retryIn)
		return outcomeFailed
	}
	if !held {
		r.markTick(name, r.clock.Now())
		observability.Debug(ctx, observability.PollerSkipped, slog.String("poller", name))
		return outcomeSkipped
	}
	start := r.clock.Now()
	actor := auth.Actor{Kind: auth.ActorSystem, ID: "poller." + name}
	tickCtx, cancelTick := context.WithTimeout(auth.WithActor(ctx, actor), TickBudget(p))
	report, err := tick(tickCtx, p)
	cancelTick()
	if err != nil {
		r.fail(ctx, name, deadlineAsTimeout(err), retryIn)
		return outcomeFailed
	}
	end := r.clock.Now()
	r.markSuccess(name, end)
	observability.Info(ctx, observability.PollerTick, slog.String("poller", name),
		slog.Int("scanned", report.Scanned), slog.Int("changed", report.Changed),
		slog.Int64("duration_ms", end.Sub(start).Milliseconds()), slog.GroupAttrs("detail", report.Attrs...))
	return outcomeOK
}

func TickBudget(p Poller) time.Duration {
	if b, ok := p.(Budgeted); ok {
		return b.TickBudget()
	}
	return p.Interval()
}

func deadlineAsTimeout(err error) error {
	code := errs.CodeOf(err)
	if errs.KindOf(code) == errs.KindInternal && code != errs.CodeRankingRunsStalled &&
		errors.Is(err, context.DeadlineExceeded) {
		return errs.Wrap(err, errs.CodeUpstreamTimeout, "poller.tick")
	}
	return err
}

func IsMoney(name string) bool {
	switch name {
	case "funding.deposit_watch", "funding.withdrawals", "funding.treasury-reconcile",
		"funding.bounce-sweeper", "funding.evm_deposit_forward", "treasury.cashout-sweeper",
		"treasury.fund-transfers", "trading.evm_trades", "ranking.valuation":
		return true
	default:
		return false
	}
}

func flow(poller string) string {
	return pollerFlows()[poller]
}

func pollerFlows() map[string]string {
	return map[string]string{
		"funding.deposit_watch":      "05",
		"funding.withdrawals":        "15",
		"funding.treasury-reconcile": "08",
		"funding.bounce-sweeper":     "08",
		"market.prices":              "18",
		"identity.nudges":            "28",
		"ranking.valuation":          "19",
		"ranking.calls_resolver":     "19a",
		"ranking.call_board":         "19b",
		"notify.vote_reminders":      "33",
		"ranking.weekly_ledger":      "37",
		"governance.close_reminders": "10b",
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

func (r *Runner) fail(ctx context.Context, name string, err error, retryIn time.Duration) {
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
	boundary.Error(
		ctx,
		observability.PollerFailed,
		slog.String("poller", name),
		slog.String("code", string(code)),
		slog.Any("err", err),
		slog.Bool("alert", errs.Alert(code)),
		slog.Duration("retry_in", retryIn),
		slog.GroupAttrs("detail", detail...),
	)
}

func (r *Runner) markTick(name string, at time.Time) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.last[name] = at
}

func (r *Runner) markSuccess(name string, at time.Time) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.last[name] = at
	r.success[name] = at
}
