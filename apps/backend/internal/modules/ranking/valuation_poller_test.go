package ranking_test

import (
	"context"
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/ranking/adapters"
	"github.com/monaco/monaco/apps/backend/internal/modules/ranking/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/ranking/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/ranking/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
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
