//go:build faultpoints

package testkit_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/platform/faultpoint"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

type stop struct{}

type fatalTB struct {
	testing.TB
	fatal string
}

func (f *fatalTB) Helper() {}

func (f *fatalTB) Fatalf(format string, args ...any) {
	f.fatal = fmt.Sprintf(format, args...)
	panic(stop{})
}

func crashAtFatal(t *testing.T, run func(ctx context.Context) error) (fatal string) {
	t.Helper()
	tb := &fatalTB{TB: t}
	defer func() {
		if p := recover(); p != nil && p != (stop{}) {
			panic(p)
		}
		fatal = tb.fatal
	}()
	testkit.CrashAt(tb, faultpoint.BeforeCommit, run)
	return tb.fatal
}

func TestCrashAt_runsArmedThenPlainAndPassesWhenTheRestartSucceeds(t *testing.T) {
	t.Parallel()
	var runs []string
	fatal := crashAtFatal(t, func(ctx context.Context) error {
		runs = append(runs, "run")
		faultpoint.Hit(ctx, faultpoint.BeforeCommit)
		runs[len(runs)-1] += " completed"
		return nil
	})
	if fatal != "" || strings.Join(runs, ", ") != "run, run completed" {
		t.Fatalf("fatal %q, runs %v; want a crashed run then a completed one", fatal, runs)
	}
}

func TestCrashAt_failsARunThatNeverReachesThePoint(t *testing.T) {
	t.Parallel()
	fatal := crashAtFatal(t, func(context.Context) error { return errors.New("gave up early") })
	if fatal != "testkit.CrashAt: the run returned (err gave up early) without reaching before-commit" {
		t.Fatalf("fatal = %q", fatal)
	}
}

func TestCrashAt_failsWhenTheRestartedRunFails(t *testing.T) {
	t.Parallel()
	fatal := crashAtFatal(t, func(ctx context.Context) error {
		faultpoint.Hit(ctx, faultpoint.BeforeCommit)
		return errors.New("still broken")
	})
	if fatal != "testkit.CrashAt: the restarted run after before-commit: still broken" {
		t.Fatalf("fatal = %q", fatal)
	}
}

func TestCrashAt_repanicsAnythingButItsCrash(t *testing.T) {
	t.Parallel()
	for _, p := range []any{"boom", faultpoint.Crash{Name: faultpoint.AfterPublish}} {
		got := func() (got any) {
			defer func() { got = recover() }()
			crashAtFatal(t, func(context.Context) error { panic(p) })
			return nil
		}()
		if got != p {
			t.Fatalf("recovered %v, want %v re-panicked", got, p)
		}
	}
}
