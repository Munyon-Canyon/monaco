package testkit

import (
	"bufio"
	"bytes"
	"context"
	"embed"
	"encoding/json"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/platform/bus"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
)

//go:embed scenarios/*.jsonl
var scenarios embed.FS

type Seeded struct {
	ID    uuid.UUID
	Actor string
	Event events.Event
}

type seedLine struct {
	ID        uuid.UUID       `json:"id"`
	Type      events.Type     `json:"type"`
	Actor     string          `json:"actor"`
	CreatedAt time.Time       `json:"created_at"`
	Payload   json.RawMessage `json:"payload"`
}

type SeedT interface {
	Helper()
	Fatalf(format string, args ...any)
	Context() context.Context
}

func Seed(t SeedT, pool *pgxpool.Pool, name string, consumers ...bus.Consumer) []Seeded {
	t.Helper()
	raw, err := scenarios.ReadFile("scenarios/" + name + ".jsonl")
	if err != nil {
		t.Fatalf("testkit.Seed: %v", err)
	}
	uow := db.New(pool, NewIDs(1), clock.Real{})
	var seeded []Seeded
	lines := bufio.NewScanner(bytes.NewReader(raw))
	for n := 1; lines.Scan(); n++ {
		line, ev := parseSeedLine(t, name, n, lines.Bytes())
		ctx := observability.WithEventID(observability.WithActor(t.Context(), line.Actor), ids.EventIDFrom(line.ID))
		actorType, actorID, _ := strings.Cut(line.Actor, ":")
		if _, err := pool.Exec(ctx, `INSERT INTO events
			(id, aggregate_type, aggregate_id, type, payload, actor_type, actor_id, created_at, published_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $8)`,
			line.ID, ev.AggregateType(), ev.AggregateID(), line.Type, line.Payload, actorType, actorID, line.CreatedAt,
		); err != nil {
			t.Fatalf("testkit.Seed: %s line %d: %v", name, n, err)
		}
		for _, h := range handlersFor(line.Type, consumers) {
			if err := uow.Do(ctx, func(ctx context.Context, tx db.Tx) error {
				if _, err := tx.Queries().Exec(ctx, `INSERT INTO event_deliveries (handler, event_id, code, handled_at)
					VALUES ($1, $2, 'ok', $3)`, h.Name, line.ID, line.CreatedAt); err != nil {
					return err
				}
				return h.Apply(ctx, tx, ev)
			}); err != nil {
				t.Fatalf("testkit.Seed: %s line %d: %s: %v", name, n, h.Name, err)
			}
		}
		seeded = append(seeded, Seeded{ID: line.ID, Actor: line.Actor, Event: ev})
	}
	return seeded
}

func parseSeedLine(t SeedT, name string, n int, raw []byte) (seedLine, events.Event) {
	t.Helper()
	var line seedLine
	var head struct {
		V int `json:"v"`
	}
	err := json.Unmarshal(raw, &line)
	if err == nil {
		err = json.Unmarshal(line.Payload, &head)
	}
	var ev events.Event
	if err == nil {
		ev, err = events.Decode(line.Type, head.V, line.Payload)
	}
	if err != nil {
		t.Fatalf("testkit.Seed: %s line %d: %v", name, n, err)
	}
	return line, ev
}

func handlersFor(typ events.Type, consumers []bus.Consumer) []bus.HandlerSpec {
	var out []bus.HandlerSpec
	for _, c := range consumers {
		for _, h := range c.Handlers {
			if h.Type() == typ {
				out = append(out, h)
			}
		}
	}
	return out
}
