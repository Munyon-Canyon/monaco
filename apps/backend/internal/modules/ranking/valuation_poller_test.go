package ranking_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/ranking/adapters"
	"github.com/monaco/monaco/apps/backend/internal/modules/ranking/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/ranking/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/ranking/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
	"github.com/monaco/monaco/apps/backend/internal/platform/poller"
)

type emptyRuns struct{}

func (emptyRuns) Run(_ context.Context, at time.Time) (app.Valuation, error) {
	return app.Valuation{AsOf: at, PricesAsOf: at, Entries: []app.Entry{}}, nil
}

type pollerRig struct {
	d      deliverer
	poller adapters.ValuationPoller
}

func newPollerRig(t *testing.T) pollerRig {
	t.Helper()
	d := newDeliverer(t)
	return pollerRig{d: d, poller: adapters.ValuationPoller{
		Reads: sqlc.New(d.pool), Runner: emptyRuns{}, Writer: app.NewSnapshotWriter(d.uow, d.gen), Clock: d.clock,
	}}
}

func (r pollerRig) tick(t *testing.T) {
	t.Helper()
	ctx := observability.WithActor(t.Context(), "system:poller.ranking.valuation")
	if _, err := r.poller.Tick(ctx); err != nil {
		t.Fatal(err)
	}
}

func (r pollerRig) fund(t *testing.T) {
	t.Helper()
	ev := events.Funded{V: 1}
	if _, err := r.d.deliverAs(t.Context(), t, r.d.event(t, ev), "ranking.triggers.funded", ev); err != nil {
		t.Fatal(err)
	}
}

func (r pollerRig) wantRuns(t *testing.T, want int) {
	t.Helper()
	if got := count(t, r.d.pool, `SELECT count(*) FROM leaderboard_runs`); got != want {
		t.Fatalf("runs = %d, want %d", got, want)
	}
}

func TestValuationPoller_aFundedEventGivesARunWithinASecond(t *testing.T) {
	t.Parallel()
	r := newPollerRig(t)
	r.tick(t)
	r.wantRuns(t, 1)
	r.d.clock.Advance(10 * time.Second)
	r.tick(t)
	r.wantRuns(t, 1)
	r.fund(t)
	r.d.clock.Advance(999 * time.Millisecond)
	r.tick(t)
	r.wantRuns(t, 1)
	r.d.clock.Advance(time.Millisecond)
	r.tick(t)
	r.wantRuns(t, 2)
	if got := count(t, r.d.pool, `SELECT count(*) FROM ranking_triggers`); got != 0 {
		t.Fatalf("triggers left = %d, want the run to have consumed them", got)
	}
}

func TestValuationPoller_fiveEventsInHalfASecondGiveOneRun(t *testing.T) {
	t.Parallel()
	r := newPollerRig(t)
	r.tick(t)
	r.d.clock.Advance(time.Minute)
	for range 5 {
		r.fund(t)
		r.d.clock.Advance(100 * time.Millisecond)
		r.tick(t)
	}
	r.wantRuns(t, 1)
	r.d.clock.Advance(time.Second)
	r.tick(t)
	r.wantRuns(t, 2)
	r.d.clock.Advance(time.Second)
	r.tick(t)
	r.wantRuns(t, 2)
}

func TestValuationPoller_runsEveryTwoMinutesWithoutEvents(t *testing.T) {
	t.Parallel()
	r := newPollerRig(t)
	r.tick(t)
	r.d.clock.Advance(domain.RunEvery)
	r.tick(t)
	r.wantRuns(t, 2)
}

type failingRuns struct{ err error }

func (f failingRuns) Run(context.Context, time.Time) (app.Valuation, error) {
	return app.Valuation{}, f.err
}

type failingWriter struct{ err error }

func (f failingWriter) Write(context.Context, app.Valuation, time.Time, time.Time) (uuid.UUID, error) {
	return uuid.Nil, f.err
}

func (r pollerRig) failingTick(t *testing.T, runs adapters.ValuationRunner, writer adapters.ValuationWriter) error {
	t.Helper()
	p := r.poller
	p.Runner, p.Writer = runs, writer
	ctx := observability.WithActor(t.Context(), "system:poller.ranking.valuation")
	_, err := p.Tick(ctx)
	return err
}

func TestValuationPoller_aFailingTickAlertsOnlyWhenTheLastRunIsOverdue(t *testing.T) {
	t.Parallel()
	cause := errs.New(errs.CodeDBUnavailable, "test")
	for name, tc := range map[string]struct {
		age  time.Duration
		want errs.Code
	}{
		"seven minutes": {7 * time.Minute, errs.CodeRankingRunsStalled},
		"over six":      {3*domain.RunEvery + time.Second, errs.CodeRankingRunsStalled},
		"exactly three": {3 * domain.RunEvery, errs.CodeDBUnavailable},
		"one minute":    {time.Minute, errs.CodeDBUnavailable},
	} {
		for stage, fail := range map[string]func(pollerRig) error{
			"run":   func(r pollerRig) error { return r.failingTick(t, failingRuns{cause}, nil) },
			"write": func(r pollerRig) error { return r.failingTick(t, emptyRuns{}, failingWriter{cause}) },
		} {
			t.Run(name+" "+stage, func(t *testing.T) {
				t.Parallel()
				r := newPollerRig(t)
				r.tick(t)
				r.fund(t)
				r.d.clock.Advance(tc.age)
				err := fail(r)
				if got := errs.CodeOf(err); got != tc.want || !errors.Is(err, cause) {
					t.Fatalf("Tick() = %v with code %s, want %s wrapping the cause", err, got, tc.want)
				}
			})
		}
	}
}

func TestValuationPoller_aFailingFirstRunKeepsItsCode(t *testing.T) {
	t.Parallel()
	r := newPollerRig(t)
	cause := errs.New(errs.CodeDBUnavailable, "test")
	if err := r.failingTick(t, failingRuns{cause}, nil); errs.CodeOf(err) != errs.CodeDBUnavailable {
		t.Fatalf("Tick() = %v, want the original code with no run on record", err)
	}
}

func TestValuationPoller_hasANinetySecondBudgetUnderTheRunCadence(t *testing.T) {
	t.Parallel()
	var p poller.Poller = adapters.ValuationPoller{}
	budgeted, ok := p.(poller.Budgeted)
	if !ok || budgeted.TickBudget() != 90*time.Second || budgeted.TickBudget() >= domain.RunEvery {
		t.Fatalf("TickBudget() = %v, %v, want 90s under %v", budgeted, ok, domain.RunEvery)
	}
}
