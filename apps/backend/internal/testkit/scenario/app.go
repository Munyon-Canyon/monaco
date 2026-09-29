package scenario

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"go.opentelemetry.io/otel/metric/noop"
	tracenoop "go.opentelemetry.io/otel/trace/noop"
	"golang.org/x/sync/errgroup"

	openapi "github.com/monaco/monaco/apps/backend/api"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/platform/auth"
	"github.com/monaco/monaco/apps/backend/internal/platform/bus"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/config"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpx"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpx/sse"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

const convergeWithin = 10 * time.Second

type app struct {
	pool      *pgxpool.Pool
	db        *db.UnitOfWork
	bus       testkit.Bus
	ids       *testkit.IDs
	verifier  *auth.DevVerifier
	server    *httptest.Server
	relay     *bus.Relay
	held      atomic.Bool
	consumers []bus.Consumer

	mu        sync.Mutex
	changed   chan struct{}
	committed map[string]map[string]bool
}

func start(t *testing.T, mods []func(module.Deps) module.Module) *app {
	t.Helper()
	pool := testkit.DB(t)
	a := &app{
		bus: testkit.NATS(t), ids: testkit.NewIDs(1), changed: make(chan struct{}),
		committed: map[string]map[string]bool{},
	}
	a.pool = pool
	a.db = db.New(pool, a.ids, clock.Real{})
	verifier, err := auth.NewDevVerifier(
		config.Config{Env: config.EnvTest, Auth: config.Auth{DevTokenKey: "scenario"}}, clock.Real{})
	must(t, err)
	a.verifier = verifier
	ctx, cancel := context.WithCancel(context.WithoutCancel(t.Context()))
	stops := make([]func(), 0, 4)
	t.Cleanup(func() {
		for i := len(stops) - 1; i >= 0; i-- {
			stops[i]()
		}
		cancel()
	})
	hub, err := sse.NewHub(sse.NoMemberships{}, noop.NewMeterProvider())
	must(t, err)
	stops = append(stops, background(ctx, hub.Run))
	must(t, a.bus.Conn.SubscribeHints(ctx, hub.Deliver))
	var reg module.Registry
	for _, m := range mods {
		reg.Add(m)
	}
	set := reg.Build(module.Deps{
		Clock: clock.Real{}, IDs: a.ids, Pool: pool, UoW: a.db, Bus: a.bus.Conn, Hub: hub,
	})
	a.consumers = a.observe(set.Consumers())
	registry, err := bus.NewRegistry(a.bus.Conn, a.db, clock.Real{}, a.consumers)
	must(t, err)
	stopConsumers, err := registry.Start(ctx)
	must(t, err)
	stops = append(stops, stopConsumers)
	a.relay = bus.NewRelay(a.bus.Conn, db.NewOutbox(pool, clock.Real{}), nil, clock.Real{})
	stops = append(stops, background(ctx, a.runRelay))
	a.server = httptest.NewServer(a.handler(t, pool, set.Routes()))
	stops = append(stops, a.server.Close)
	return a
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("scenario: start the app: %v", err)
	}
}

func background(ctx context.Context, run func(context.Context)) func() {
	ctx, cancel := context.WithCancel(ctx)
	var g errgroup.Group
	g.Go(func() error {
		run(ctx)
		return nil
	})
	return func() {
		cancel()
		_ = g.Wait()
	}
}

func (a *app) handler(t *testing.T, pool *pgxpool.Pool, routes httpx.Routes) http.Handler {
	t.Helper()
	h, err := httpx.Handler(httpx.Deps{
		Logger:       observability.NewLogger(config.Config{Env: config.EnvTest}, io.Discard),
		Tracer:       tracenoop.NewTracerProvider(),
		Clock:        clock.Real{},
		IDs:          a.ids,
		MaxBodyBytes: 1 << 20,
		Idempotency:  db.NewIdempotencyStore(pool, clock.Real{}),
		Verifier:     a.verifier,
	}, routes, openapi.Spec)
	must(t, err)
	checked := testkit.HTTP(t, h)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/stream" {
			h.ServeHTTP(w, r)
			return
		}
		checked.ServeHTTP(w, r)
	})
}

func (a *app) runRelay(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-a.db.Signal():
		}
		if !a.held.Load() {
			a.relay.Once(ctx)
		}
	}
}

func (a *app) observe(consumers []bus.Consumer) []bus.Consumer {
	out := make([]bus.Consumer, len(consumers))
	for i, c := range consumers {
		c.Handlers = append([]bus.HandlerSpec(nil), c.Handlers...)
		for j, h := range c.Handlers {
			c.Handlers[j] = h.OnCommit(func(ctx context.Context, _ events.Event) {
				a.record(h.Name, observability.EventIDFrom(ctx))
			})
		}
		out[i] = c
	}
	return out
}

func (a *app) record(handler, eventID string) {
	a.update(func() {
		if a.committed[handler] == nil {
			a.committed[handler] = map[string]bool{}
		}
		a.committed[handler][eventID] = true
	})
}

func (a *app) update(change func()) {
	a.mu.Lock()
	defer a.mu.Unlock()
	change()
	close(a.changed)
	a.changed = make(chan struct{})
}

func (a *app) handlersOf(typ events.Type) []string {
	var names []string
	for _, c := range a.consumers {
		for _, h := range c.Handlers {
			if h.Type() == typ {
				names = append(names, h.Name)
			}
		}
	}
	return names
}

func (a *app) handledAll(handlers, eventIDs []string) bool {
	for _, h := range handlers {
		for _, id := range eventIDs {
			if !a.committed[h][id] {
				return false
			}
		}
	}
	return true
}

func (a *app) await(t *testing.T, what string, done func() bool) {
	t.Helper()
	deadline := time.NewTimer(convergeWithin)
	defer deadline.Stop()
	for {
		a.mu.Lock()
		ok, changed := done(), a.changed
		a.mu.Unlock()
		if ok {
			return
		}
		select {
		case <-changed:
		case <-deadline.C:
			t.Fatalf("scenario: %s did not happen within %s", what, convergeWithin)
		}
	}
}
