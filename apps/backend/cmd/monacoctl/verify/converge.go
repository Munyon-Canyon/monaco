package verify

import (
	"cmp"
	"context"
	"fmt"
	"maps"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"
)

const probeTimeout = 5 * time.Second

type watched struct {
	durable, handler, typ string
}

func (d *driver) watchedBy(u Unit) []watched {
	var out []watched
	for _, c := range d.env.Consumers {
		for _, h := range c.Handlers {
			if slices.Contains(u.Flow.Consumers, h.Name) || slices.Contains(u.Flow.Consumers, c.Durable) {
				out = append(out, watched{durable: c.Durable, handler: h.Name, typ: string(h.Type())})
			}
		}
	}
	return out
}

func (d *driver) flowEvents(ctx context.Context, users, polled []string) ([]string, error) {
	rows, err := d.env.Pool.Query(ctx, `SELECT id::text FROM events
		WHERE actor_id = ANY($1) OR (type = ANY($2) AND actor_id LIKE 'poller.%') ORDER BY id`, users, polled)
	var ids []string
	if err == nil {
		ids, err = pgx.CollectRows(rows, pgx.RowTo[string])
	}
	if err != nil {
		return nil, fmt.Errorf("read the flow's events: %w", err)
	}
	return ids, nil
}

func (d *driver) converge(ctx context.Context, u Unit, eventIDs []string) error {
	tick := time.NewTicker(d.pollInterval())
	defer tick.Stop()
	for {
		stuck, err := d.probe(ctx, u, eventIDs)
		if err != nil || stuck == "" {
			return err
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("%s: %w", stuck, context.Cause(ctx))
		case <-tick.C:
		}
	}
}

func (d *driver) pollInterval() time.Duration { return cmp.Or(d.env.PollEvery, pollEvery) }

func (d *driver) probe(ctx context.Context, u Unit, eventIDs []string) (string, error) {
	ctx, cancel := detached(ctx)
	defer cancel()
	return d.stuck(ctx, d.watchedBy(u), eventIDs)
}

func detached(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.WithoutCancel(ctx), probeTimeout)
}

type delivered struct {
	ID, Type  string
	Published bool
	Handlers  []string
}

func (d *driver) deliveries(ctx context.Context, eventIDs []string) ([]delivered, error) {
	rows, err := d.env.Pool.Query(ctx, `SELECT e.id::text, e.type, e.published_at IS NOT NULL,
		coalesce(array_agg(d.handler ORDER BY d.handler) FILTER (WHERE d.handler IS NOT NULL), '{}')
		FROM events e LEFT JOIN event_deliveries d ON d.event_id = e.id
		WHERE e.id::text = ANY($1) GROUP BY e.id, e.type, e.published_at ORDER BY e.id`, eventIDs)
	var out []delivered
	if err == nil {
		out, err = pgx.CollectRows(rows, pgx.RowToStructByPos[delivered])
	}
	if err != nil {
		return nil, fmt.Errorf("read deliveries: %w", err)
	}
	return out, nil
}

func (d *driver) stuck(ctx context.Context, watch []watched, eventIDs []string) (string, error) {
	events, err := d.deliveries(ctx, eventIDs)
	if err != nil {
		return "", err
	}
	durables := map[string]bool{}
	for _, e := range events {
		for _, w := range watch {
			if w.typ != e.Type {
				continue
			}
			if !slices.Contains(e.Handlers, w.handler) {
				return fmt.Sprintf("consumer %s handler %s has not handled event %s", w.durable, w.handler, e.ID), nil
			}
			durables[w.durable] = true
		}
	}
	return d.pending(ctx, slices.Sorted(maps.Keys(durables)))
}

func (d *driver) pending(ctx context.Context, durables []string) (string, error) {
	for _, durable := range durables {
		c, err := d.env.JS.Consumer(ctx, d.env.Events, durable)
		if err != nil {
			return "", fmt.Errorf("consumer %s: %w", durable, err)
		}
		if info := c.CachedInfo(); info.NumPending > 0 || info.NumAckPending > 0 {
			return fmt.Sprintf("consumer %s has %d pending and %d unacked messages",
				durable, info.NumPending, info.NumAckPending), nil
		}
	}
	return "", nil
}
