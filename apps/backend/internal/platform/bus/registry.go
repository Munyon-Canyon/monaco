package bus

import (
	"context"
	"fmt"

	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
)

type Handler[E events.Event] func(ctx context.Context, tx db.Tx, e E) error

type HandlerSpec struct {
	Name string
	typ  events.Type
	run  func(ctx context.Context, tx db.Tx, e events.Event) error
}

func Handle[E events.Event](name string, fn Handler[E]) HandlerSpec {
	var zero E
	return HandlerSpec{
		Name: name,
		typ:  zero.Type(),
		run: func(ctx context.Context, tx db.Tx, e events.Event) error {
			return fn(ctx, tx, e.(E))
		},
	}
}

type Consumer struct {
	Durable  string
	Handlers []HandlerSpec
}

type Registry struct {
	conn      *Conn
	uow       *db.UnitOfWork
	clock     clock.Clock
	consumers map[string]Consumer
}

func NewRegistry(conn *Conn, uow *db.UnitOfWork, clk clock.Clock, consumers []Consumer) *Registry {
	r := &Registry{conn: conn, uow: uow, clock: clk, consumers: make(map[string]Consumer, len(consumers))}
	handlers := map[string]struct{}{}
	for _, c := range consumers {
		if _, dup := r.consumers[c.Durable]; dup {
			panic(fmt.Sprintf("bus: consumer %s registered twice", c.Durable))
		}
		for _, h := range c.Handlers {
			if _, dup := handlers[h.Name]; dup {
				panic(fmt.Sprintf("bus: handler %s registered twice", h.Name))
			}
			handlers[h.Name] = struct{}{}
		}
		r.consumers[c.Durable] = c
	}
	return r
}
