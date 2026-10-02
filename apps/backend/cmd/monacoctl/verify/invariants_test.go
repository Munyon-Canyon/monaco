package verify

import (
	"context"
	"net/http"
	"slices"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
	"github.com/monaco/monaco/apps/backend/internal/testkit/scenario"
	tools "github.com/monaco/monaco/apps/backend/internal/tools/flows"
)

const (
	healthzRequest = `{"msg":"http.request","method":"GET","route":"/healthz","status":200,"duration_ms":0}`
	healthzMissing = "flow 90 ok invariant: no http.request log line with map[method:GET route:/healthz]"
)

func healthzSettle(pool *pgxpool.Pool, lines ...string) (*driver, *Result) {
	d := &driver{clock: clock.Real{}, env: Env{Pool: pool, Logs: &Logs{}}}
	res := &Result{
		Unit:      Unit{Flow: tools.Flow{ID: "90", Trigger: "GET /healthz"}, Outcome: tools.OutcomeOK},
		Exchanges: []scenario.Exchange{{Method: http.MethodGet, Path: "/healthz", Status: http.StatusOK}},
		logFrom:   d.env.Logs.mark(),
	}
	for _, line := range lines {
		d.env.Logs.add(procAPI, line)
	}
	return d, res
}

func TestSettle_passesOnARequiredLineThatLandsAfterTheLogCheckStarts(t *testing.T) {
	t.Parallel()
	d, res := healthzSettle(testkit.DB(t))
	ctx, cancel := context.WithTimeout(t.Context(), DefaultBudget().Converge)
	defer cancel()
	time.AfterFunc(20*time.Millisecond, func() { d.env.Logs.add(procAPI, healthzRequest) })
	if err := d.settle(ctx, res); err != nil || ctx.Err() != nil {
		t.Fatalf("settle = %v, deadline passed %v; want a pass once the line lands 20ms in", err, ctx.Err() != nil)
	}
	if want := []string{procAPI + ": " + healthzRequest}; !slices.Equal(res.logLines, want) {
		t.Fatalf("evidence log lines = %q, want %q", res.logLines, want)
	}
}

func TestSettle_failsOnARequiredLineThatNeverLandsOnceTheDeadlinePasses(t *testing.T) {
	t.Parallel()
	d, res := healthzSettle(testkit.DB(t))
	ctx, cancel := context.WithTimeout(t.Context(), 100*time.Millisecond)
	defer cancel()
	if err := d.settle(ctx, res); err == nil || err.Error() != healthzMissing {
		t.Fatalf("settle = %v, want %q", err, healthzMissing)
	}
	if ctx.Err() == nil {
		t.Fatal("settle failed on the missing line before the converge deadline passed")
	}
}

const (
	consumerAck = `{"msg":"bus.dispatched","handler":"fixture.ok","subject":"events.system.pinged",` +
		`"outcome":"ack","code":"ok"}`
	consumerAckMissing = "flow 96 ok invariant: no bus.dispatched for system.pinged with outcome ack " +
		"after the script started"
)

func consumerSettle(pool *pgxpool.Pool) (*driver, *Result) {
	d := &driver{clock: clock.Real{}, env: Env{Pool: pool, Logs: &Logs{}, Subject: func(s string) string { return s }}}
	res := &Result{
		Unit: Unit{
			Flow:    tools.Flow{ID: "96", Trigger: "consumer:system.pinged"},
			Outcome: tools.OutcomeOK,
		},
		logFrom: d.env.Logs.mark(),
	}
	return d, res
}

func TestSettle_passesOnAConsumerDispatchThatLandsAfterTheLogCheckStarts(t *testing.T) {
	t.Parallel()
	d, res := consumerSettle(testkit.DB(t))
	ctx, cancel := context.WithTimeout(t.Context(), DefaultBudget().Converge)
	defer cancel()
	time.AfterFunc(20*time.Millisecond, func() { d.env.Logs.add(procWorker, consumerAck) })
	if err := d.settle(ctx, res); err != nil || ctx.Err() != nil {
		t.Fatalf("settle = %v, deadline passed %v; want a pass once the dispatch lands 20ms in", err, ctx.Err() != nil)
	}
	if want := []string{procWorker + ": " + consumerAck}; !slices.Equal(res.logLines, want) {
		t.Fatalf("evidence log lines = %q, want %q", res.logLines, want)
	}
}

func TestSettle_failsOnAConsumerDispatchThatNeverLandsOnceTheDeadlinePasses(t *testing.T) {
	t.Parallel()
	d, res := consumerSettle(testkit.DB(t))
	ctx, cancel := context.WithTimeout(t.Context(), 100*time.Millisecond)
	defer cancel()
	if err := d.settle(ctx, res); err == nil || err.Error() != consumerAckMissing {
		t.Fatalf("settle = %v, want %q", err, consumerAckMissing)
	}
	if ctx.Err() == nil {
		t.Fatal("settle failed on the missing dispatch before the converge deadline passed")
	}
}

func TestSettle_pollsTheLogOnceWhenTheDeadlineHasAlreadyPassed(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	for _, tc := range []struct {
		name  string
		lines []string
		want  string
	}{
		{"line there", []string{healthzRequest}, ""},
		{"line missing", nil, healthzMissing},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			d, res := healthzSettle(pool, tc.lines...)
			ctx, cancel := context.WithTimeout(t.Context(), -time.Second)
			defer cancel()
			got := ""
			if err := d.settle(ctx, res); err != nil {
				got = err.Error()
			}
			if got != tc.want {
				t.Fatalf("settle past its deadline = %q, want %q", got, tc.want)
			}
		})
	}
}
