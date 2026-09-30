package poller_test

import (
	"context"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/platform/auth"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/poller"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

type ctxPoller struct {
	fakePoller
	run func(ctx context.Context) error
}

func (p *ctxPoller) Tick(ctx context.Context) (poller.Report, error) {
	return poller.Report{}, p.run(ctx)
}

type actorPoller struct {
	fakePoller
	seen chan auth.Actor
}

func newActorPoller(name string) *actorPoller {
	return &actorPoller{fakePoller: fakePoller{name: name}, seen: make(chan auth.Actor, 1)}
}

func (p *actorPoller) Tick(ctx context.Context) (poller.Report, error) {
	actor, _ := auth.ActorFrom(ctx)
	p.seen <- actor
	return poller.Report{}, nil
}

func (p *actorPoller) actor(t *testing.T) auth.Actor {
	t.Helper()
	select {
	case actor := <-p.seen:
		return actor
	default:
		t.Fatalf("%s recorded no actor", p.name)
		return auth.Actor{}
	}
}

func TestRunner_eventAppendedInATickCommitsAsTheSystemActorNamedAfterThePoller(t *testing.T) {
	t.Parallel()
	pool, clk := testkit.DB(t), testkit.NewClock(epoch())
	uow, h := db.New(pool, testkit.NewIDs(1), clk), newHarness(t, pool, clk)
	ping := events.SystemPinged{V: 1, PingID: testkit.NewIDs(2).NewV7(), Note: "hi"}
	p := &ctxPoller{fakePoller: fakePoller{name: "test.appends"}, run: func(ctx context.Context) error {
		return uow.Do(ctx, func(ctx context.Context, tx db.Tx) error { return tx.Events.Append(ctx, ping) })
	}}
	out, _ := h.start(t, p)
	out.expect(t, "tx.committed")
	out.expect(t, "poller.tick")
	row := pool.QueryRow(t.Context(), `SELECT actor_type, actor_id FROM events WHERE aggregate_id = $1`, ping.PingID)
	var actorType, actorID string
	if err := row.Scan(&actorType, &actorID); err != nil {
		t.Fatal(err)
	}
	if actorType != "system" || actorID != "poller.test.appends" {
		t.Fatalf("events row actor = %s:%s, want system:poller.test.appends", actorType, actorID)
	}
}

func TestRunner_tickSeesTheSystemActorNamedAfterItsPoller(t *testing.T) {
	t.Parallel()
	h := newHarness(t, testkit.DB(t), testkit.NewClock(epoch()))
	p := newActorPoller("test.actor")
	out, _ := h.start(t, p)
	out.expect(t, "poller.tick")
	if got := p.actor(t); got.Kind != auth.ActorSystem || got.ID != "poller.test.actor" {
		t.Fatalf("auth.ActorFrom in Tick = %+v, want kind system and id poller.test.actor", got)
	}
}

func TestRunner_eachPollerInOneRunSeesItsOwnActor(t *testing.T) {
	t.Parallel()
	h := newHarness(t, testkit.DB(t), testkit.NewClock(epoch()))
	first, second := newActorPoller("test.first"), newActorPoller("test.second")
	out, _ := h.start(t, first, second)
	out.expect(t, "poller.tick")
	out.expect(t, "poller.tick")
	for _, p := range []*actorPoller{first, second} {
		if got := p.actor(t); got.Kind != auth.ActorSystem || got.ID != "poller."+p.name {
			t.Errorf("%s saw actor %+v, want kind system and its own name", p.name, got)
		}
	}
}
