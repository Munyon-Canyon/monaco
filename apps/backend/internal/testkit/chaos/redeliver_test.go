//go:build faultpoints

package chaos_test

import (
	"context"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/platform/bus"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
	"github.com/monaco/monaco/apps/backend/internal/testkit/chaos"
)

func TestDispatch_aDroppedAckRedeliversIntoTheDedupeRowSoTheHandlerRunsOnce(t *testing.T) {
	t.Parallel()
	pool, clk := testkit.DB(t), testkit.NewClock(time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC))
	conn := testkit.NATS(t).Conn
	uow := db.New(pool, testkit.NewIDs(1), clk)
	ran := map[string]int{}
	count := func(ctx context.Context, _ db.Tx, _ events.SystemPinged) error {
		ran[observability.EventIDFrom(ctx)]++
		return nil
	}
	c := bus.Consumer{Durable: "once", Handlers: []bus.HandlerSpec{bus.Handle("once.count", count)}}
	reg, err := bus.NewRegistry(conn, uow, clk, []bus.Consumer{c})
	if err != nil {
		t.Fatal(err)
	}
	ctx := observability.WithActor(t.Context(), "system:test")
	msgs := appendPings(ctx, t, uow, pool, conn, 12)

	res := chaos.Dispatch(ctx, t, 9, reg, c, msgs)
	trace := strings.Join(res.Trace, "\n")
	dropped := 0
	for _, m := range msgs {
		id := m.ID()
		dropped += countLines(res.Trace, id, "ack dropped, redelivered")
		want := 1 + crashesBeforeFirstCommit(res.Trace, id)
		if ran[id] != want || res.Verdicts[id].Outcome != bus.OutcomeAck {
			t.Fatalf(
				"%s: handler ran %d times, want %d (one plus the crashes before its first commit); verdict %+v\n%s",
				id,
				ran[id],
				want,
				res.Verdicts[id],
				trace,
			)
		}
	}
	var deliveries int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM event_deliveries`).Scan(&deliveries); err != nil {
		t.Fatal(err)
	}
	if dropped == 0 || deliveries != len(msgs) {
		t.Fatalf("%d dropped acks, %d event_deliveries rows; want some drops and one row per event\n%s",
			dropped, deliveries, trace)
	}
}

func appendPings(
	ctx context.Context, t *testing.T, uow *db.UnitOfWork, pool *pgxpool.Pool, conn *bus.Conn, n int,
) []*chaos.Msg {
	t.Helper()
	for i := range n {
		ev := events.SystemPinged{V: 1, PingID: uuid.NewSHA1(uuid.Nil, []byte(strconv.Itoa(i)))}
		if err := uow.Do(
			ctx,
			func(ctx context.Context, tx db.Tx) error { return tx.Events.Append(ctx, ev) },
		); err != nil {
			t.Fatal(err)
		}
	}
	var msgs []*chaos.Msg
	rows, err := pool.Query(ctx, `SELECT id, payload FROM events ORDER BY id`)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var (
			id      uuid.UUID
			payload []byte
		)
		if err := rows.Scan(&id, &payload); err != nil {
			t.Fatal(err)
		}
		msgs = append(msgs, chaos.NewMsg(conn, events.TypeSystemPinged, ids.EventIDFrom(id), payload))
	}
	rows.Close()
	return msgs
}

func crashesBeforeFirstCommit(trace []string, id string) int {
	n := 0
	for _, line := range trace {
		if !strings.Contains(line, " "+id+" #") {
			continue
		}
		if !strings.Contains(line, "crashed before commit") {
			return n
		}
		n++
	}
	return n
}

func countLines(trace []string, id, note string) int {
	n := 0
	for _, line := range trace {
		if strings.Contains(line, " "+id+" #") && strings.Contains(line, note) {
			n++
		}
	}
	return n
}
