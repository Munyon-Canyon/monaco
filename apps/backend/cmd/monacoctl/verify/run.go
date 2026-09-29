package verify

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/platform/bus"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/faultpoint"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/testkit/flows"
)

var (
	errArgs   = errors.New("bad arguments")
	errFailed = errors.New("verify failed")
)

const Usage = "usage: monacoctl verify [flow <id> | all] [--outcome X] [--crash-at P]"

type Target struct {
	Flow    string
	Outcome string
	CrashAt string
}

func ParseArgs(args []string) (Target, error) {
	var t Target
	for i := 0; i < len(args); i++ {
		next := func() (string, error) {
			if i+1 >= len(args) {
				return "", fmt.Errorf("%w: %s needs a value", errArgs, args[i])
			}
			i++
			return args[i], nil
		}
		var err error
		switch args[i] {
		case "all":
			t.Flow = ""
		case "flow":
			t.Flow, err = next()
		case "--outcome":
			t.Outcome, err = next()
		case "--crash-at":
			t.CrashAt, err = next()
		default:
			err = fmt.Errorf("%w: unknown argument %q", errArgs, args[i])
		}
		if err != nil {
			return Target{}, err
		}
	}
	return t.settle()
}

func (t Target) settle() (Target, error) {
	if point, ok := strings.CutPrefix(t.Outcome, "crash:"); ok && t.CrashAt == "" {
		t.CrashAt = point
	}
	if t.CrashAt != "" && !faultpoint.Known(t.CrashAt) {
		return Target{}, fmt.Errorf("%w: --crash-at %s is not a faultpoint", errArgs, t.CrashAt)
	}
	return t, nil
}

type Config struct {
	Dir      string
	Environ  []string
	Go       string
	Docker   Docker
	Atlas    string
	TempDir  string
	Budget   Budget
	Postgres PostgresFunc
	Modules  func(module.Deps) module.Set
	Scripts  map[string]flows.Script
	Ledger   []LedgerCheck
	Stdout   io.Writer
	Stderr   io.Writer
}

func Run(ctx context.Context, cfg Config, target Target) int {
	if err := run(ctx, cfg, target); err != nil {
		_, _ = fmt.Fprintf(cfg.Stderr, "monacoctl verify: %v\n", err)
		return 1
	}
	return 0
}

func run(ctx context.Context, cfg Config, target Target) (err error) {
	all, err := readFlows(cfg.Dir)
	if err != nil {
		return err
	}
	units, err := selectUnits(all, target, cfg.Scripts)
	if err != nil {
		return err
	}
	rep := newReport(ctx, target, units)
	defer func() {
		writeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), cfg.Budget.Teardown)
		defer cancel()
		err = errors.Join(err, rep.write(writeCtx, cfg.Dir, err))
	}()
	began := time.Now()
	bins, cover, cleanup, err := prepare(ctx, cfg, target)
	defer func() { err = errors.Join(err, cleanup()) }()
	rep.phases[PhaseBuild] = time.Since(began)
	if err != nil {
		return err
	}
	runCtx, cancel := context.WithTimeoutCause(ctx, cfg.Budget.Total,
		&OverBudgetError{Phase: PhaseTotal, Budget: cfg.Budget.Total})
	defer cancel()
	began = time.Now()
	stack, err := Up(runCtx, Options{
		Dir: cfg.Dir, Atlas: cfg.Atlas, Docker: cfg.Docker, Environ: cfg.Environ, Bins: bins,
		Budget: cfg.Budget, Postgres: cfg.Postgres, CoverDir: cover, Faultpoint: target.CrashAt,
	})
	rep.phases[PhaseStack] = time.Since(began)
	defer func() {
		began := time.Now()
		err = errors.Join(err, stack.Down(runCtx))
		rep.phases[PhaseTeardown] = time.Since(began)
		rep.crashes = stack.Crashes
	}()
	if err != nil {
		return err
	}
	_, _ = fmt.Fprintf(cfg.Stdout, "stack %s healthy: api %s, worker %s\n", stack.RunID, stack.API, stack.Worker)
	consumers := cfg.Modules(module.Deps{
		Clock: clock.Real{}, IDs: ids.Real{}, Pool: stack.Pool, Bus: stack.Bus,
		UoW: db.New(stack.Pool, ids.Real{}, clock.Real{}),
	}).Consumers()
	parallel := parallelFlows
	if target.CrashAt != "" {
		parallel = 1
	}
	return verifyUnits(runCtx, cfg, Env{
		API: stack.API, TokenKey: stack.TokenKey, Pool: stack.Pool, JS: stack.NATS.JS,
		Events: bus.StreamEvents, DeadLetter: bus.StreamDeadLetter, Consumers: consumers, Logs: stack.Logs,
		Crash: stack.crash, Arm: stack.arm,
	}, rep, parallel)
}

func prepare(ctx context.Context, cfg Config, target Target) (Binaries, string, func() error, error) {
	cover := filepath.Join(cfg.Dir, ".verify", "cover")
	out, err := os.MkdirTemp(cfg.TempDir, "monaco-verify-bin-")
	cleanup := func() error { return os.RemoveAll(out) }
	if err != nil {
		return Binaries{}, cover, cleanup, fmt.Errorf("binaries dir: %w", err)
	}
	bins, err := build(ctx, cfg.Go, cfg.Dir, out, target.CrashAt != "")
	if err != nil {
		return bins, cover, cleanup, err
	}
	if cfg.Postgres == nil {
		if err := cfg.Docker.ensureImage(ctx); err != nil {
			return bins, cover, cleanup, err
		}
	}
	if err := os.MkdirAll(cover, 0o750); err != nil {
		return bins, cover, cleanup, fmt.Errorf("coverage dir: %w", err)
	}
	return bins, cover, cleanup, nil
}

func verifyUnits(ctx context.Context, cfg Config, env Env, rep *report, parallel int) error {
	d, err := newDriver(env, cfg.Budget)
	if err != nil {
		return err
	}
	rep.results = d.runAll(ctx, rep.units, parallel)
	failed := 0
	var over error
	for _, r := range rep.results {
		phases := fmt.Sprintf("seed %s, flow %s, converge %s", r.Phases[PhaseSeed].Round(time.Millisecond),
			r.Phases[PhaseFlow].Round(time.Millisecond), r.Phases[PhaseConverge].Round(time.Millisecond))
		if r.Pass() {
			_, _ = fmt.Fprintf(cfg.Stdout, "PASS flow %s (%s)\n", r.Unit.Name(), phases)
			continue
		}
		failed++
		if r.Over != nil && over == nil {
			over = r.Over
		}
		_, _ = fmt.Fprintf(cfg.Stdout, "FAIL flow %s (%s): %s\n", r.Unit.Name(), phases, r.Failure)
	}
	global := d.global(ctx, cfg.Ledger, rep)
	for _, err := range global {
		rep.global = append(rep.global, err.Error())
		_, _ = fmt.Fprintf(cfg.Stdout, "FAIL %v\n", err)
	}
	if err := d.collect(ctx, rep); err != nil {
		return err
	}
	if failed+len(global) > 0 {
		return errors.Join(fmt.Errorf("%w: %d of %d flow outcomes failed, %d run invariants failed",
			errFailed, failed, len(rep.results), len(global)), over)
	}
	return nil
}
