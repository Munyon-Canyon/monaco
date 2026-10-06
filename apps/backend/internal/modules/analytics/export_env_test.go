package analytics_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
	"github.com/monaco/monaco/apps/backend/internal/testkit/chaos"
)

type governanceFake struct {
	testkit.Faults
	mu        sync.Mutex
	proposers map[ids.ProposalID]ids.UserID
}

func (g *governanceFake) Proposer(_ context.Context, id ids.ProposalID) (ids.UserID, error) {
	if err := g.Check("Proposer"); err != nil {
		return ids.UserID{}, err
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	proposer, ok := g.proposers[id]
	if !ok {
		return ids.UserID{}, errs.New(errs.CodeProposalNotFound, "governanceFake.Proposer")
	}
	return proposer, nil
}

func (g *governanceFake) opened(proposal ids.ProposalID, by ids.UserID) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.proposers == nil {
		g.proposers = map[ids.ProposalID]ids.UserID{}
	}
	g.proposers[proposal] = by
}

func (e *env) appendEvent(t *testing.T, actor string, ev events.Event) appended {
	t.Helper()
	ctx := observability.WithActor(t.Context(), actor)
	err := e.uow.Do(ctx, func(ctx context.Context, tx db.Tx) error { return tx.Events.Append(ctx, ev) })
	if err != nil {
		t.Fatal(err)
	}
	var a appended
	err = e.pool.QueryRow(t.Context(), `SELECT id, payload, created_at FROM events WHERE type = $1 AND aggregate_id = $2`,
		string(ev.Type()), ev.AggregateID()).
		Scan(&a.id, &a.payload, &a.created)
	if err != nil {
		t.Fatal(err)
	}
	e.clock.Advance(time.Second)
	return a
}

func (e *env) messageOf(typ events.Type, a appended) *msg {
	return &msg{Msg: chaos.NewMsg(e.bus.Conn, typ, ids.EventIDFrom(a.id), a.payload)}
}

func (e *env) deliveriesOf(t *testing.T, handler string) int {
	t.Helper()
	var n int
	if err := e.pool.QueryRow(t.Context(), `SELECT count(*) FROM event_deliveries WHERE handler = $1`, handler).
		Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}
