package testkit

import (
	"context"
	"fmt"
	"hash/fnv"
	"maps"
	"math/rand/v2"
	"slices"
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
	"github.com/monaco/monaco/apps/backend/internal/testkit/chaos"
)

const suiteEvents = 12

type Harness struct {
	Pool  *pgxpool.Pool
	IDs   *IDs
	Clock *Clock
}

type consumerSuite struct {
	h        Harness
	conn     *bus.Conn
	uow      *db.UnitOfWork
	reg      *bus.Registry
	consumer bus.Consumer
	gen      func(rng *rand.Rand, i int) events.Event
}

func ConsumerSuite(
	t *testing.T, consumer func(Harness) bus.Consumer, gen func(rng *rand.Rand, i int) events.Event,
) {
	t.Helper()
	s := &consumerSuite{
		h:   Harness{Pool: DB(t), IDs: NewIDs(1), Clock: NewClock(time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC))},
		gen: gen,
	}
	s.conn = NATS(t).Conn
	s.uow = db.New(s.h.Pool, s.h.IDs, s.h.Clock)
	s.consumer = consumer(s.h)
	s.consumer.Handlers = slices.Clone(s.consumer.Handlers)
	for i, spec := range s.consumer.Handlers {
		s.consumer.Handlers[i] = spec.Before(func(ctx context.Context, _ events.Event) {
			s.h.IDs.Reseed(handlerSeed(spec.Name, observability.EventIDFrom(ctx)))
		})
	}
	reg, err := bus.NewRegistry(s.conn, s.uow, s.h.Clock, []bus.Consumer{s.consumer})
	if err != nil {
		t.Fatalf("testkit.ConsumerSuite: %v", err)
	}
	s.reg = reg

	ctx := s.ctx(t)
	base := chaos.Baseline(ctx, t, s.reg, s.consumer, s.appendEvents(t))
	want := s.snapshot(t, base)
	for _, seed := range chaos.Seeds() {
		t.Run(fmt.Sprintf("seed=%d", seed), func(t *testing.T) {
			res := chaos.Dispatch(s.ctx(t), t, seed, s.reg, s.consumer, s.appendEvents(t))
			if diff := diffSnapshots(want, s.snapshot(t, res)); diff != "" {
				t.Fatalf(
					"chaos seed %d diverged from the single-delivery run\n%s\nfates:\n%s\nrerun with -chaos.seed=%d",
					seed,
					diff,
					strings.Join(res.Trace, "\n"),
					seed,
				)
			}
		})
	}
}

func (s *consumerSuite) ctx(t *testing.T) context.Context {
	t.Helper()
	return observability.WithActor(t.Context(), "system:consumer-suite")
}

func handlerSeed(handler, eventID string) uint64 {
	h := fnv.New64a()
	_, _ = h.Write([]byte(handler + "/" + eventID))
	return h.Sum64()
}

func (s *consumerSuite) appendEvents(t *testing.T) []*chaos.Msg {
	t.Helper()
	Reset(t, s.h.Pool)
	s.h.IDs.Reseed(1)
	rng := rand.New(rand.NewPCG(1, 0))
	ctx := s.ctx(t)
	for i := range suiteEvents {
		ev := s.gen(rng, i)
		if err := s.uow.Do(
			ctx,
			func(ctx context.Context, tx db.Tx) error { return tx.Events.Append(ctx, ev) },
		); err != nil {
			t.Fatalf("testkit.ConsumerSuite: append event %d: %v", i, err)
		}
	}
	rows, err := s.h.Pool.Query(ctx, `SELECT id, type, payload FROM events ORDER BY id`)
	if err != nil {
		t.Fatalf("testkit.ConsumerSuite: list events: %v", err)
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
			t.Fatalf("testkit.ConsumerSuite: scan event: %v", err)
		}
		msgs = append(msgs, chaos.NewMsg(s.conn, events.Type(typ), ids.EventIDFrom(id), payload))
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("testkit.ConsumerSuite: list events: %v", err)
	}
	return msgs
}

func (s *consumerSuite) snapshot(t *testing.T, res chaos.Result) map[string][]string {
	t.Helper()
	ctx := t.Context()
	var tables []string
	rows, err := s.h.Pool.Query(ctx, `
		SELECT format('%I.%I', schemaname, tablename) FROM pg_tables
		WHERE schemaname NOT IN ('pg_catalog', 'information_schema', $1)`, atlasSchema)
	if err == nil {
		for rows.Next() {
			var name string
			if err = rows.Scan(&name); err != nil {
				break
			}
			tables = append(tables, name)
		}
		rows.Close()
		err = rows.Err()
	}
	if err != nil {
		t.Fatalf("testkit.ConsumerSuite: list tables: %v", err)
	}
	out := map[string][]string{"terms": res.Terms()}
	for _, table := range tables {
		var got []string
		err := s.h.Pool.QueryRow(ctx, `
			SELECT coalesce(array_agg(j ORDER BY j), '{}')
			FROM (SELECT (to_jsonb(t) - '{created_at,updated_at,handled_at,published_at,trace_parent}'::text[])::text AS j
				FROM `+table+` t) rows`).Scan(&got)
		if err != nil {
			t.Fatalf("testkit.ConsumerSuite: snapshot %s: %v", table, err)
		}
		out[table] = got
	}
	return out
}

func diffSnapshots(want, got map[string][]string) string {
	var b strings.Builder
	tables := maps.Clone(want)
	maps.Copy(tables, got)
	keys := slices.Sorted(maps.Keys(tables))
	for _, k := range keys {
		missing, extra := multisetDiff(want[k], got[k])
		for _, row := range missing {
			fmt.Fprintf(&b, "- %s %s\n", k, row)
		}
		for _, row := range extra {
			fmt.Fprintf(&b, "+ %s %s\n", k, row)
		}
	}
	return b.String()
}

func multisetDiff(want, got []string) (missing, extra []string) {
	count := map[string]int{}
	for _, row := range got {
		count[row]++
	}
	for _, row := range want {
		if count[row] > 0 {
			count[row]--
			continue
		}
		missing = append(missing, row)
	}
	for _, row := range got {
		if count[row] > 0 {
			count[row]--
			extra = append(extra, row)
		}
	}
	return missing, extra
}
