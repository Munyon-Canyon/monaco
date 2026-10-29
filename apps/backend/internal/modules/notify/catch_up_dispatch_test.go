package notify_test

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/cabal"
	"github.com/monaco/monaco/apps/backend/internal/modules/notify/adapters"
	"github.com/monaco/monaco/apps/backend/internal/modules/notify/app"
	"github.com/monaco/monaco/apps/backend/internal/platform/bus"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
	"github.com/monaco/monaco/apps/backend/internal/testkit/chaos"
)

type memberCabals struct {
	app.Cabals
	members []ids.UserID
}

func (m memberCabals) Members(context.Context, ids.CabalID) ([]cabal.MemberView, error) {
	out := make([]cabal.MemberView, len(m.members))
	for i, u := range m.members {
		out[i] = cabal.MemberView{UserID: u}
	}
	return out, nil
}

func (w *catchUpWorld) members() memberCabals { return memberCabals{members: w.cabalMembers} }

func (w *catchUpWorld) dispatch(t *testing.T, clears *app.CatchUpClears, evs ...events.Event) {
	t.Helper()
	consumer := bus.Consumer{Durable: adapters.CatchUpDurable, Handlers: adapters.CatchUpHandlers(clears)}
	conn := testkit.NATS(t).Conn
	reg, err := bus.NewRegistry(conn, w.r.uow, w.r.clock, []bus.Consumer{consumer})
	if err != nil {
		t.Fatal(err)
	}
	ctx := observability.WithActor(t.Context(), "system:catch-up-test")
	for _, ev := range evs {
		appendEvent := func(ctx context.Context, tx db.Tx) error { return tx.Events.Append(ctx, ev) }
		if err := w.r.uow.Do(ctx, appendEvent); err != nil {
			t.Fatal(err)
		}
	}
	rows, err := w.r.pool.Query(ctx, `SELECT id, type, payload FROM events ORDER BY id`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var msgs []*chaos.Msg
	for rows.Next() {
		var (
			id      uuid.UUID
			typ     string
			payload []byte
		)
		if err := rows.Scan(&id, &typ, &payload); err != nil {
			t.Fatal(err)
		}
		msgs = append(msgs, chaos.NewMsg(conn, events.Type(typ), ids.EventIDFrom(id), payload))
	}
	rows.Close()
	chaos.Baseline(ctx, t, reg, consumer, msgs)
}
