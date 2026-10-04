package scenario

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/nats-io/nats.go/jetstream"
	"go.opentelemetry.io/otel/metric/noop"
	tracenoop "go.opentelemetry.io/otel/trace/noop"

	openapi "github.com/monaco/monaco/apps/backend/api"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/platform/auth"
	"github.com/monaco/monaco/apps/backend/internal/platform/bus"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/config"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/faultpoint"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpx"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpx/api"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpx/ratelimit"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpx/sse"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
	"github.com/monaco/monaco/apps/backend/internal/platform/poller"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
	"github.com/monaco/monaco/apps/backend/internal/testkit/fakes"
)

type app struct {
	tb        *testing.T
	logger    *slog.Logger
	pool      *pgxpool.Pool
	db        *db.UnitOfWork
	bus       testkit.Bus
	ids       *testkit.IDs
	verifier  *auth.DevVerifier
	server    *httptest.Server
	relay     *bus.Relay
	held      atomic.Bool
	consumers []bus.Consumer
	pollers   []poller.Poller
	runner    *poller.Runner
	log       *lineLog

	note       *notifier
	committed  map[string]map[string]bool
	upstreams  *fakes.Server
	privyAppID string
}

func start(t *testing.T, o options) *app {
	t.Helper()
	pool := testkit.DB(t)
	a := &app{
		tb: t, bus: testkit.NATS(t), ids: testkit.NewIDs(1), note: newNotifier(),
		committed: map[string]map[string]bool{},
	}
	a.pool = pool
	a.upstreams, a.privyAppID = o.fakes, o.privyAppID
	a.db = db.New(pool, a.ids, clock.Real{})
	verifier, err := auth.NewDevVerifier(
		config.Config{Env: config.EnvTest, Auth: config.Auth{DevTokenKey: tokenKey}}, clock.Real{})
	must(t, err)
	a.verifier = verifier
	logs := o.logs
	if logs == nil {
		logs = io.Discard
	}
	a.log = &lineLog{note: a.note}
	a.logger = observability.NewLogger(config.Config{Env: config.EnvTest}, io.MultiWriter(logs, a.log))
	ctx, cancel := context.WithCancel(observability.WithLogger(context.WithoutCancel(t.Context()), a.logger))
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
	for _, m := range o.modules {
		reg.Add(m)
	}
	set := reg.Build(module.Deps{
		Config: testkit.Config(), Clock: clock.Real{}, IDs: a.ids, Pool: pool, UoW: a.db, Bus: a.bus.Conn, Hub: hub,
	})
	a.consumers = a.observe(set.Consumers())
	a.pollers = set.Pollers()
	a.runner, err = poller.NewRunner(pool, clock.Real{}, noop.NewMeterProvider().Meter("scenario"))
	must(t, err)
	registry, err := bus.NewRegistry(a.bus.Conn, a.db, clock.Real{}, a.consumers)
	must(t, err)
	stopConsumers, err := registry.Start(ctx)
	must(t, err)
	stops = append(stops, stopConsumers)
	a.relay = bus.NewRelay(a.bus.Conn, db.NewOutbox(pool, clock.Real{}), nil, clock.Real{})
	stops = append(stops, background(ctx, a.runRelay))
	a.server = httptest.NewServer(a.handler(t, pool, set.Mount, o.contract, o.wrap))
	stops = append(stops, a.server.Close)
	return a
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("scenario: start the app: %v", err)
	}
}

func (a *app) handler(
	t *testing.T, pool *pgxpool.Pool, mount func(api.Mount), c *httpx.Contract, wrap func(http.Handler) http.Handler,
) http.Handler {
	t.Helper()
	if c == nil {
		var err error
		c, err = LoadContract(openapi.Spec)
		must(t, err)
	}
	policies, err := ratelimit.FromDocument(c.Document())
	must(t, err)
	limiter, err := ratelimit.New(pool, clock.Real{}, noop.NewMeterProvider())
	must(t, err)
	h, err := httpx.HandlerFor(httpx.Deps{
		Logger:       a.logger,
		Tracer:       tracenoop.NewTracerProvider(),
		Clock:        clock.Real{},
		IDs:          a.ids,
		MaxBodyBytes: 1 << 20,
		Idempotency:  db.NewIdempotencyStore(pool, clock.Real{}),
		Verifier:     a.verifier,
		RateLimit:    ratelimit.Middleware(limiter, policies, httpx.ActorKey, false),
	}, mount, c)
	must(t, err)
	checked := testkit.HTTPAgainst(t, c.Document(), h)
	served := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/stream" {
			h.ServeHTTP(w, r)
			return
		}
		checked.ServeHTTP(w, r)
	})
	if wrap == nil {
		return served
	}
	return wrap(served)
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
	a.note.update(func() {
		if a.committed[handler] == nil {
			a.committed[handler] = map[string]bool{}
		}
		a.committed[handler][eventID] = true
	})
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

func (a *app) backend() *backend {
	return &backend{
		baseURL:       a.server.URL,
		client:        a.server.Client(),
		note:          a.note,
		pool:          a.pool,
		mint:          a.mint,
		privyToken:    a.privyToken,
		script:        a.scriptFakes,
		newUserID:     a.newUserID,
		enter:         func(Stage) {},
		exchanged:     func(Exchange) {},
		events:        a.events,
		eventPayloads: a.eventPayloads,
		awaitHandled:  a.awaitHandled,
		published:     a.published,
		hold:          a.hold,
		crashAt:       a.crashAt,
		seed:          a.seed,
		lines:         a.log.since,
		tick:          a.tickOnce,
	}
}

func (a *app) tickOnce(t T, name string) func() {
	t.Helper()
	i := slices.IndexFunc(a.pollers, func(p poller.Poller) bool { return p.Name() == name })
	if i < 0 {
		t.Fatalf("scenario: no module registers poller %s", name)
	}
	return background(observability.WithLogger(t.Context(), a.logger), func(ctx context.Context) {
		_ = a.runner.Run(ctx, a.pollers[i])
	})
}

func (a *app) mint(id ids.UserID) string {
	return a.verifier.Mint(id.String(), time.Now().Add(time.Hour))
}

func (a *app) privyToken(sub string) string {
	return fakes.PrivyAccessToken(a.privyAppID, sub, time.Now(), time.Hour)
}

func (a *app) scriptFakes(ctx context.Context, t T, step fakes.Step) {
	t.Helper()
	raw, err := json.Marshal(step)
	if err != nil {
		t.Fatalf("scenario: script %+v: %v", step, err)
	}
	rec := httptest.NewRecorder()
	a.upstreams.ServeHTTP(
		rec,
		httptest.NewRequestWithContext(ctx, http.MethodPost, "/_script", bytes.NewReader(raw)),
	)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("scenario: script %s answered %d %s", raw, rec.Code, rec.Body)
	}
}

func (a *app) newUserID() (ids.UserID, error) { return ids.ParseUserID(a.ids.NewV7().String()) }

func (a *app) events(t T, typ events.Type, _ []string) []string {
	t.Helper()
	rows, err := a.pool.Query(t.Context(), `SELECT id::text FROM events WHERE type = $1`, string(typ))
	return scanIDs(t, typ, rows, err)
}

func (a *app) eventPayloads(t T, typ events.Type) [][]byte {
	t.Helper()
	rows, err := a.pool.Query(t.Context(), `SELECT payload FROM events WHERE type = $1 ORDER BY id`, string(typ))
	if err != nil {
		t.Fatalf("scenario: read %s event payloads: %v", typ, err)
	}
	defer rows.Close()
	var payloads [][]byte
	for rows.Next() {
		var payload []byte
		if err := rows.Scan(&payload); err != nil {
			t.Fatalf("scenario: scan %s event payload: %v", typ, err)
		}
		payloads = append(payloads, payload)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("scenario: read %s event payloads: %v", typ, err)
	}
	return payloads
}

func scanIDs(t T, typ events.Type, rows pgx.Rows, err error) []string {
	t.Helper()
	var got []string
	for err == nil && rows.Next() {
		var id string
		err = rows.Scan(&id)
		got = append(got, id)
	}
	if err == nil {
		rows.Close()
		err = rows.Err()
	}
	if err != nil {
		t.Fatalf("scenario: read %s events: %v", typ, err)
	}
	return got
}

func (a *app) awaitHandled(t T, typ events.Type, eventIDs []string) {
	t.Helper()
	handlers := a.handlersOf(typ)
	a.note.await(t, "every handler of "+string(typ)+" committing "+strings.Join(eventIDs, ", "), func() bool {
		return a.handledAll(handlers, eventIDs)
	})
}

func (a *app) published(t T, typ events.Type, _ []string) uint64 {
	t.Helper()
	stream, err := a.bus.JS.Stream(t.Context(), a.bus.Events)
	var info *jetstream.StreamInfo
	if err == nil {
		info, err = stream.Info(t.Context(), jetstream.WithSubjectFilter(a.bus.Conn.Subject(typ.Subject())))
	}
	if err != nil {
		t.Fatalf("scenario: read the events stream: %v", err)
	}
	return info.State.Subjects[a.bus.Conn.Subject(typ.Subject())]
}

func (a *app) hold() { a.held.Store(true) }

func (a *app) crashAt(_ T, point faultpoint.Name) {
	testkit.CrashAt(a.tb, point, func(ctx context.Context) error {
		a.relay.Once(ctx)
		return nil
	})
	a.held.Store(false)
}

func (a *app) seed(t T, name string) []testkit.Seeded {
	t.Helper()
	return testkit.Seed(t, a.pool, name, a.consumers...)
}

type Served struct {
	URL, TokenKey, Events, DeadLetter string
	Subject                           func(subject string) string
	Pool                              *pgxpool.Pool
	JS                                jetstream.JetStream
	Consumers                         []bus.Consumer
}

func Serve(t *testing.T, opts ...Option) Served {
	t.Helper()
	var o options
	for _, opt := range opts {
		opt(&o)
	}
	a := start(t, o)
	return Served{
		URL: a.server.URL, TokenKey: tokenKey, Events: a.bus.Events, DeadLetter: a.bus.DeadLetter,
		Subject: a.bus.Conn.Subject, Pool: a.pool, JS: a.bus.JS, Consumers: a.consumers,
	}
}
