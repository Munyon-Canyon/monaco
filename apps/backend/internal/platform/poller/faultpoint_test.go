//go:build faultpoints

package poller

import (
	"bytes"
	"context"
	"log/slog"
	"testing"
	"time"

	"go.opentelemetry.io/otel/metric/noop"

	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/faultpoint"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

type crashingPoller struct{}

func (crashingPoller) Name() string { return "fixture.crash" }

func (crashingPoller) Interval() time.Duration { return time.Minute }

func (crashingPoller) Tick(context.Context) (Report, error) {
	panic(faultpoint.Crash{Name: faultpoint.BeforeCommit})
}

func TestAttempt_logsAndRepanicsAFaultpoint(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	runner, err := NewRunner(pool, clock.Real{}, noop.NewMeterProvider().Meter("poller"))
	if err != nil {
		t.Fatal(err)
	}
	lock := db.NewLock(pool, "poller:faultpoint")
	t.Cleanup(func() { _ = lock.Release(t.Context()) })
	var logs bytes.Buffer
	ctx := observability.WithLogger(t.Context(), slog.New(slog.NewJSONHandler(&logs, nil)))
	defer func() {
		if got := recover(); got != (faultpoint.Crash{Name: faultpoint.BeforeCommit}) {
			t.Fatalf("recovered %v, want before-commit faultpoint", got)
		}
		if got := logs.String(); !bytes.Contains([]byte(got), []byte(`"msg":"poller.tick.crashed"`)) ||
			!bytes.Contains([]byte(got), []byte(`"code":"faultpoint"`)) {
			t.Fatalf("logs = %s", got)
		}
	}()
	runner.attempt(ctx, crashingPoller{}, lock)
}

func TestTick_repanicsAFaultpoint(t *testing.T) {
	t.Parallel()
	defer func() {
		if got := recover(); got != (faultpoint.Crash{Name: faultpoint.BeforeCommit}) {
			t.Fatalf("recovered %v, want before-commit faultpoint", got)
		}
	}()
	_, _ = tick(t.Context(), crashingPoller{})
}

func TestRecoverAttempt_repanicsAnUnexpectedPanic(t *testing.T) {
	t.Parallel()
	defer func() {
		if got := recover(); got != "unexpected" {
			t.Fatalf("recovered %v, want unexpected", got)
		}
	}()
	func() {
		defer recoverAttempt(t.Context(), "fixture.panic")
		panic("unexpected")
	}()
}

func TestFlow_associatesOnlyFlowPollers(t *testing.T) {
	t.Parallel()
	for name, want := range map[string]string{
		"funding.deposits": "05", "market.prices": "18", "identity.nudges": "28", "market.catalog": "",
	} {
		if got := flow(name); got != want {
			t.Errorf("flow(%q) = %q, want %q", name, got, want)
		}
	}
}
