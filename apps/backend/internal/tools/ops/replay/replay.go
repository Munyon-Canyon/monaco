package replay

import (
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"slices"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/platform/bus"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
)

type RefusedError string

func (e RefusedError) Error() string { return string(e) }

type Clock struct {
	clock.Real
	now time.Time
}

func (c *Clock) Now() time.Time { return c.now }

type Options struct {
	Source, Target *pgxpool.Pool
	UoW            *db.UnitOfWork
	Clock          *Clock
	Handlers       []bus.HandlerSpec
	To             uuid.UUID
	Verify         bool
	Checks         []LedgerCheck
}

type Report struct {
	Events     int
	Applied    int
	Duplicates int
	Diffs      []string
}

type row struct {
	ID            uuid.UUID  `db:"id"`
	AggregateType string     `db:"aggregate_type"`
	AggregateID   uuid.UUID  `db:"aggregate_id"`
	Type          string     `db:"type"`
	Payload       []byte     `db:"payload"`
	ActorType     string     `db:"actor_type"`
	ActorID       string     `db:"actor_id"`
	TraceParent   *string    `db:"trace_parent"`
	CreatedAt     time.Time  `db:"created_at"`
	PublishedAt   *time.Time `db:"published_at"`
}

const eventColumns = `id, aggregate_type, aggregate_id, type, payload, actor_type, actor_id, trace_parent,
	created_at, published_at`

func Handlers(consumers []bus.Consumer) []bus.HandlerSpec {
	var out []bus.HandlerSpec
	for _, c := range consumers {
		out = append(out, c.Handlers...)
	}
	return out
}

func Run(ctx context.Context, o Options) (Report, error) {
	if err := refuseUsed(ctx, o.Target); err != nil {
		return Report{}, err
	}
	to := &o.To
	if o.To == uuid.Nil {
		to = nil
	}
	rows, err := load(ctx, o.Source, `WHERE $1::uuid IS NULL OR id <= $1`, to)
	if err != nil {
		return Report{}, err
	}
	handled, err := handledAt(ctx, o.Source)
	if err != nil {
		return Report{}, err
	}
	skip := map[string]bool{}
	for _, c := range o.Checks {
		for _, h := range c.Handlers {
			skip[h] = true
		}
	}
	rep := Report{Events: len(rows)}
	for _, r := range rows {
		if err := apply(ctx, o, r, handled, skip, &rep); err != nil {
			return rep, err
		}
	}
	if o.Verify {
		rep.Diffs, err = verify(ctx, o)
	}
	return rep, err
}

func refuseUsed(ctx context.Context, target *pgxpool.Pool) error {
	const op = "replay.Run"
	var existing int
	if err := target.QueryRow(ctx, `SELECT count(*) FROM events`).Scan(&existing); err != nil {
		return errs.Wrap(err, errs.CodeInternal, op)
	}
	if existing > 0 {
		return errs.Wrap(RefusedError(fmt.Sprintf(
			"target holds %d events; replay writes only into a fresh database", existing)), errs.CodeInvalidInput, op)
	}
	return nil
}

func apply(
	ctx context.Context, o Options, r row, handled map[delivery]time.Time, skip map[string]bool, rep *Report,
) error {
	if _, err := o.Target.Exec(ctx, `INSERT INTO events (`+eventColumns+`)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`,
		r.ID, r.AggregateType, r.AggregateID, r.Type, r.Payload, r.ActorType, r.ActorID, r.TraceParent,
		r.CreatedAt, r.PublishedAt); err != nil {
		return errs.Wrap(err, errs.CodeInternal, "replay.Run", slog.String("event_id", r.ID.String()))
	}
	for _, h := range o.Handlers {
		if h.Type() != events.Type(r.Type) || skip[h.Name] {
			continue
		}
		o.Clock.now = r.CreatedAt
		if at, ok := handled[delivery{h.Name, r.ID}]; ok {
			o.Clock.now = at
		}
		if err := deliver(ctx, o.UoW, o.Clock, h, r, rep); err != nil {
			return err
		}
	}
	return nil
}

func load(ctx context.Context, pool *pgxpool.Pool, where string, args ...any) ([]row, error) {
	rows, _ := pool.Query(ctx, `SELECT `+eventColumns+` FROM events `+where+` ORDER BY id`, args...)
	out, err := pgx.CollectRows(rows, pgx.RowToStructByName[row])
	if err != nil {
		return nil, errs.Wrap(err, errs.CodeInternal, "replay.load")
	}
	return out, nil
}

type delivery struct {
	handler string
	event   uuid.UUID
}

func handledAt(ctx context.Context, pool *pgxpool.Pool) (map[delivery]time.Time, error) {
	rows, _ := pool.Query(ctx, `SELECT handler, event_id, handled_at FROM event_deliveries`)
	out := map[delivery]time.Time{}
	var (
		d  delivery
		at time.Time
	)
	_, err := pgx.ForEachRow(rows, []any{&d.handler, &d.event, &at}, func() error {
		out[d] = at
		return nil
	})
	if err != nil {
		return nil, errs.Wrap(err, errs.CodeInternal, "replay.handledAt")
	}
	return out, nil
}

func deliver(ctx context.Context, uow *db.UnitOfWork, clk clock.Clock, h bus.HandlerSpec, r row, rep *Report) error {
	var head struct {
		V int `json:"v"`
	}
	_ = json.Unmarshal(r.Payload, &head)
	ev, err := events.Decode(events.Type(r.Type), head.V, r.Payload)
	if err != nil {
		return err
	}
	id := ids.EventIDFrom(r.ID)
	ctx = observability.WithEventID(observability.WithActor(ctx, r.ActorType+":"+r.ActorID), id)
	duplicate, err := bus.Deliver(ctx, uow, clk, h, id, ev)
	switch {
	case err != nil:
		return errs.Wrap(err, errs.CodeOf(err), "replay.deliver",
			slog.String("handler", h.Name), slog.String("event_id", r.ID.String()))
	case duplicate:
		rep.Duplicates++
	default:
		rep.Applied++
	}
	return nil
}

func sortedRows(ctx context.Context, pool *pgxpool.Pool, table string) ([]string, error) {
	rows, _ := pool.Query(ctx, `SELECT to_jsonb(t)::text FROM `+pgx.Identifier{table}.Sanitize()+` t`)
	out, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		return nil, errs.Wrap(err, errs.CodeInternal, "replay.sortedRows", slog.String("table", table))
	}
	slices.Sort(out)
	return out, nil
}

func diffRows(table string, source, rebuilt []string) []string {
	var diffs []string
	for len(source) > 0 || len(rebuilt) > 0 {
		c := 1
		switch {
		case len(source) == 0:
		case len(rebuilt) == 0:
			c = -1
		default:
			c = cmp.Compare(source[0], rebuilt[0])
		}
		switch {
		case c < 0:
			diffs = append(diffs, table+": only in source: "+source[0])
			source = source[1:]
		case c > 0:
			diffs = append(diffs, table+": only in replay: "+rebuilt[0])
			rebuilt = rebuilt[1:]
		default:
			source, rebuilt = source[1:], rebuilt[1:]
		}
	}
	return diffs
}
