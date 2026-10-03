package verify

import (
	"context"
	"errors"
	"fmt"
	"hash/fnv"
	"runtime"
	"slices"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/nats-io/nats.go/jetstream"
	"golang.org/x/sync/errgroup"

	"github.com/monaco/monaco/apps/backend/internal/platform/auth"
	"github.com/monaco/monaco/apps/backend/internal/platform/bus"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/config"
	"github.com/monaco/monaco/apps/backend/internal/platform/faultpoint"
	"github.com/monaco/monaco/apps/backend/internal/testkit/scenario"
	tools "github.com/monaco/monaco/apps/backend/internal/tools/flows"
)

const parallelFlows = 4

type Env struct {
	API        string
	Fakes      string
	PrivyAppID string
	TokenKey   string
	Pool       *pgxpool.Pool
	JS         jetstream.JetStream
	Events     string
	DeadLetter string
	Subject    func(subject string) string
	Consumers  []bus.Consumer
	Logs       *Logs
	Crash      func(ctx context.Context, point faultpoint.Name) error
	Arm        func(ctx context.Context) error
}

type Result struct {
	Unit      Unit
	Failure   string
	Over      *OverBudgetError
	Phases    map[Phase]time.Duration
	Exchanges []scenario.Exchange
	Users     []string
	Events    []string

	rows      []EventEvidence
	logLines  []string
	logFrom   int
	startedAt time.Time
	mu        sync.Mutex
}

func (r *Result) Pass() bool { return r.Failure == "" }

func (r *Result) fail(err error) {
	if r.Failure == "" {
		r.Failure = err.Error()
	}
	var over *OverBudgetError
	if r.Over == nil && errors.As(err, &over) {
		r.Over = over
	}
}

type driver struct {
	env      Env
	budget   Budget
	verifier *auth.DevVerifier
	clock    clock.Clock
}

func newDriver(env Env, budget Budget) (*driver, error) {
	verifier, err := auth.NewDevVerifier(
		config.Config{Env: config.EnvTest, Auth: config.Auth{DevTokenKey: env.TokenKey}}, clock.Real{})
	return &driver{env: env, budget: budget, verifier: verifier, clock: clock.Real{}}, err
}

func (d *driver) runAll(ctx context.Context, units []Unit, parallel int) []*Result {
	results := make([]*Result, len(units))
	var g errgroup.Group
	g.SetLimit(parallel)
	var pollers []int
	for i, u := range units {
		if kind, _ := u.Flow.TriggerKind(u.Command); kind == tools.TriggerPoller {
			pollers = append(pollers, i)
			continue
		}
		g.Go(func() error {
			results[i] = d.run(ctx, u)
			return nil
		})
	}
	_ = g.Wait()
	for _, i := range pollers {
		results[i] = d.run(ctx, units[i])
	}
	return results
}

func (d *driver) run(ctx context.Context, u Unit) *Result {
	res := &Result{Unit: u, Phases: map[Phase]time.Duration{}, startedAt: time.Now()}
	if err := d.env.Arm(ctx); err != nil {
		res.fail(err)
		return res
	}
	res.logFrom = d.env.Logs.mark()
	if err := d.script(ctx, u, res); err != nil {
		res.fail(err)
		return res
	}
	began := d.clock.Now()
	ctx, cancel := withDeadline(ctx, d.clock, d.budget.Converge,
		&OverBudgetError{Phase: PhaseConverge, Flow: u.Name(), Budget: d.budget.Converge})
	defer cancel()
	err := d.settle(ctx, res)
	res.Phases[PhaseConverge] = d.clock.Now().Sub(began)
	if err != nil {
		res.fail(err)
	}
	return res
}

func (d *driver) script(ctx context.Context, u Unit, res *Result) error {
	ctx, cancel := context.WithCancelCause(ctx)
	defer cancel(nil)
	t := &flowT{ctx: func() context.Context { return ctx }}
	stages := make(chan scenario.Stage, 16)
	done := make(chan struct{})
	remote := scenario.Remote{
		URL: d.env.API, FakesURL: d.env.Fakes, PrivyAppID: d.env.PrivyAppID, ClientIP: clientIP(u), Pool: d.env.Pool,
		Consumers: d.env.Consumers,
		Mint: func(id string) string {
			res.mu.Lock()
			res.Users = append(res.Users, id)
			res.mu.Unlock()
			return d.verifier.Mint(id, time.Now().Add(time.Hour))
		},
		Converge: func(ctx context.Context, ids []string) error { return d.converge(ctx, u, ids) },
		Crash:    d.env.Crash,
		Enter: func(s scenario.Stage) {
			select {
			case stages <- s:
			case <-ctx.Done():
			}
		},
		Exchanged: func(e scenario.Exchange) {
			res.mu.Lock()
			res.Exchanges = append(res.Exchanges, e)
			res.mu.Unlock()
		},
		Logs: d.env.Logs.since,
	}
	go func() {
		defer close(done)
		defer t.runCleanups()
		u.Script(scenario.Against(t, remote))
	}()
	phase, began := PhaseSeed, d.clock.Now()
	timer := d.clock.NewTicker(d.budget.Seed)
	defer func() { timer.Stop() }()
	stopped := func(err error) error {
		<-done
		res.Phases[phase] += d.clock.Now().Sub(began)
		if failed := t.err(); failed != nil {
			res.fail(failed)
		}
		return err
	}
	for {
		select {
		case <-done:
			return stopped(cancelCause(ctx, u.Name(), t.err()))
		case s := <-stages:
			if s != scenario.StageGiven && phase == PhaseSeed {
				res.Phases[phase] = d.clock.Now().Sub(began)
				phase, began = PhaseFlow, d.clock.Now()
				timer.Stop()
				timer = d.clock.NewTicker(d.budget.Flow)
			}
		case <-timer.C():
			over := &OverBudgetError{Phase: phase, Flow: u.Name(), Budget: d.budget.of(phase)}
			cancel(over)
			return stopped(over)
		case <-ctx.Done():
			return stopped(fmt.Errorf("flow %s: %w", u.Name(), context.Cause(ctx)))
		}
	}
}

func cancelCause(ctx context.Context, flow string, failure error) error {
	if failure != nil && ctx.Err() != nil {
		return fmt.Errorf("flow %s: %w", flow, context.Cause(ctx))
	}
	return failure
}

func withDeadline(
	ctx context.Context, clk clock.Clock, budget time.Duration, cause error,
) (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancelCause(ctx)
	timer := clk.NewTicker(budget)
	go func() {
		defer timer.Stop()
		select {
		case <-timer.C():
			cancel(cause)
		case <-ctx.Done():
		}
	}()
	return ctx, func() { cancel(context.Canceled) }
}

func (b Budget) of(p Phase) time.Duration {
	if p == PhaseSeed {
		return b.Seed
	}
	return b.Flow
}

type flowT struct {
	ctx      func() context.Context
	mu       sync.Mutex
	failure  string
	cleanups []func()
}

func (t *flowT) Helper() {}

func (t *flowT) Context() context.Context { return t.ctx() }

func (t *flowT) Cleanup(f func()) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.cleanups = append(t.cleanups, f)
}

func (t *flowT) Fatal(args ...any) { t.fail(fmt.Sprint(args...)) }

func (t *flowT) Fatalf(format string, args ...any) { t.fail(fmt.Sprintf(format, args...)) }

func (t *flowT) fail(msg string) {
	t.mu.Lock()
	if t.failure == "" {
		t.failure = msg
	}
	t.mu.Unlock()
	runtime.Goexit()
}

func (t *flowT) err() error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.failure == "" {
		return nil
	}
	return FailureError(t.failure)
}

type FailureError string

func (e FailureError) Error() string { return string(e) }

func (t *flowT) runCleanups() {
	t.mu.Lock()
	cleanups := slices.Clone(t.cleanups)
	t.mu.Unlock()
	for _, f := range slices.Backward(cleanups) {
		f()
	}
}

func clientIP(u Unit) string {
	h := fnv.New32a()
	_, _ = h.Write([]byte(u.Name()))
	sum := h.Sum(nil)
	return fmt.Sprintf("10.%d.%d.%d", sum[1], sum[2], sum[3])
}
