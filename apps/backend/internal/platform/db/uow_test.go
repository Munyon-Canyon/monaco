package db_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.opentelemetry.io/otel/trace"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/config"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

const (
	traceIDHex = "4bf92f3577b34da6a3ce929d0e0e4736"
	spanIDHex  = "00f067aa0ba902b7"
)

type harness struct {
	pool  *pgxpool.Pool
	uow   *db.UnitOfWork
	ids   *testkit.IDs
	clock *testkit.Clock
	logs  *testkit.Logs
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	h := &harness{
		pool:  testkit.DB(t),
		ids:   testkit.NewIDs(1),
		clock: testkit.NewClock(time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)),
		logs:  &testkit.Logs{},
	}
	h.uow = db.New(h.pool, h.ids, h.clock)
	_, err := h.pool.Exec(t.Context(), `CREATE TABLE things (id int PRIMARY KEY, status text NOT NULL)`)
	if err != nil {
		t.Fatal(err)
	}
	return h
}

func (h *harness) ctx(t *testing.T, actor string) context.Context {
	t.Helper()
	traceID, _ := trace.TraceIDFromHex(traceIDHex)
	spanID, _ := trace.SpanIDFromHex(spanIDHex)
	sc := trace.NewSpanContext(trace.SpanContextConfig{TraceID: traceID, SpanID: spanID})
	ctx := observability.WithActor(trace.ContextWithSpanContext(t.Context(), sc), actor)
	return observability.WithLogger(ctx, observability.NewLogger(config.Config{Env: config.EnvTest}, h.logs))
}

func (h *harness) insertThing(ctx context.Context, tx db.Tx, id int) error {
	_, err := tx.Queries().Exec(ctx, `INSERT INTO things (id, status) VALUES ($1, 'created')`, id)
	return err
}

func (h *harness) count(t *testing.T, table string) int {
	t.Helper()
	var n int
	if err := h.pool.QueryRow(t.Context(), `SELECT count(*) FROM `+table).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func (h *harness) lines(t *testing.T) []map[string]any {
	t.Helper()
	var out []map[string]any
	dec := json.NewDecoder(bytes.NewReader(h.logs.Bytes()))
	for dec.More() {
		var m map[string]any
		if err := dec.Decode(&m); err != nil {
			t.Fatal(err)
		}
		out = append(out, m)
	}
	return out
}

func (h *harness) linesNamed(t *testing.T, msg string) []map[string]any {
	t.Helper()
	var out []map[string]any
	for _, l := range h.lines(t) {
		if l["msg"] == msg {
			out = append(out, l)
		}
	}
	return out
}

func (h *harness) assertOneLine(t *testing.T, msg string, want map[string]any) {
	t.Helper()
	lines := h.linesNamed(t, msg)
	if len(lines) != 1 {
		t.Fatalf("%d %s lines, want 1: %v", len(lines), msg, h.lines(t))
	}
	for k, v := range want {
		if lines[0][k] != v {
			t.Fatalf("%s %s = %v, want %v", msg, k, lines[0][k], v)
		}
	}
}

func (h *harness) pendingSignals() int { return len(h.uow.Signal()) }

func pinged(g *testkit.IDs) events.SystemPinged {
	return events.SystemPinged{V: 1, PingID: g.NewV7(), Note: "hi"}
}

func TestDo_commitsTheStateChangeAndTheEventTogetherAndSignals(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	ev := pinged(testkit.NewIDs(9))
	err := h.uow.Do(h.ctx(t, "user:u1"), func(ctx context.Context, tx db.Tx) error {
		if err := h.insertThing(ctx, tx, 1); err != nil {
			return err
		}
		return tx.Events.Append(ctx, ev)
	})
	if err != nil {
		t.Fatal(err)
	}
	if h.count(t, "things") != 1 || h.pendingSignals() != 1 {
		t.Fatalf("things=%d signals=%d, want 1 and 1", h.count(t, "things"), h.pendingSignals())
	}
	want := testkit.NewIDs(1).NewV7()
	h.assertEventRow(t, want, ev)
	h.assertOneLine(t, "tx.committed", map[string]any{
		"level": "INFO", "attempt": float64(1), "trace_id": traceIDHex, "actor": "user:u1",
	})
	if got, _ := h.linesNamed(t, "tx.committed")[0]["event_ids"].([]any); len(got) != 1 || got[0] != want.String() {
		t.Fatalf("event_ids = %v, want [%s]", got, want)
	}
}

func (h *harness) assertEventRow(t *testing.T, want uuid.UUID, ev events.SystemPinged) {
	t.Helper()
	var (
		id, aggID                            uuid.UUID
		aggType, typ, actorType, actorID, tp string
		payload                              []byte
		createdAt                            time.Time
	)
	err := h.pool.QueryRow(t.Context(), `
		SELECT id, aggregate_type, aggregate_id, type, payload, actor_type, actor_id, trace_parent, created_at
		FROM events`).Scan(&id, &aggType, &aggID, &typ, &payload, &actorType, &actorID, &tp, &createdAt)
	if err != nil {
		t.Fatal(err)
	}
	if id != want || aggType != "system" || aggID != ev.PingID || typ != "system.pinged" ||
		actorType != "user" || actorID != "u1" || tp != "00-"+traceIDHex+"-"+spanIDHex+"-00" ||
		!createdAt.Equal(h.clock.Now()) {
		t.Fatalf("row = %v %s %v %s %s %s %s %v", id, aggType, aggID, typ, actorType, actorID, tp, createdAt)
	}
	var decoded events.SystemPinged
	if err := json.Unmarshal(payload, &decoded); err != nil || decoded != ev {
		t.Fatalf("payload %s decodes to %+v (%v), want %+v", payload, decoded, err, ev)
	}
}

func TestDo_rollsBackBothWritesOnAClosureErrorAndKeepsTheCallersError(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	refused := errs.New(errs.CodeNotFound, "thing.Find")
	err := h.uow.Do(h.ctx(t, "user:u1"), func(ctx context.Context, tx db.Tx) error {
		if err := h.insertThing(ctx, tx, 1); err != nil {
			return err
		}
		if err := tx.Events.Append(ctx, pinged(h.ids)); err != nil {
			return err
		}
		return refused
	})
	if !errors.Is(err, refused) || err.Error() != refused.Error() {
		t.Fatalf("Do = %v, want the closure's own error %v", err, refused)
	}
	if h.count(t, "things") != 0 || h.count(t, "events") != 0 || h.pendingSignals() != 0 {
		t.Fatalf("things=%d events=%d signals=%d after rollback, want zeros",
			h.count(t, "things"), h.count(t, "events"), h.pendingSignals())
	}
	rolled := h.linesNamed(t, "tx.rolled_back")
	if len(rolled) != 1 || rolled[0]["code"] != "not_found" || rolled[0]["attempt"] != float64(1) ||
		len(h.linesNamed(t, "tx.committed")) != 0 {
		t.Fatalf("log lines = %v", h.lines(t))
	}
}

func TestDo_rollsBackAndRePanicsWhenTheClosurePanics(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	defer func() {
		if r := recover(); r != "boom" {
			t.Fatalf("recovered %v, want boom", r)
		}
		if h.count(t, "things") != 0 || h.pendingSignals() != 0 {
			t.Fatalf("things=%d signals=%d after a panic, want zeros", h.count(t, "things"), h.pendingSignals())
		}
		h.assertOneLine(t, "tx.rolled_back", map[string]any{"code": "panic", "attempt": float64(1)})
	}()
	_ = h.uow.Do(h.ctx(t, "user:u1"), func(ctx context.Context, tx db.Tx) error {
		if err := h.insertThing(ctx, tx, 1); err != nil {
			return err
		}
		panic("boom")
	})
}

func TestDo_retriesSerializationFailuresAndDeadlocksOnce(t *testing.T) {
	t.Parallel()
	for name, code := range map[string]string{"serialization": "40001", "deadlock": "40P01"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			h := newHarness(t)
			uow := db.New(h.pool, h.ids, clock.Real{})
			calls := 0
			err := uow.Do(h.ctx(t, "user:u1"), func(ctx context.Context, tx db.Tx) error {
				calls++
				if err := h.insertThing(ctx, tx, calls); err != nil {
					return err
				}
				if calls == 1 {
					return &pgconn.PgError{Code: code}
				}
				return nil
			})
			if err != nil || calls != 2 || h.count(t, "things") != 1 {
				t.Fatalf("Do = %v after %d calls with %d rows, want nil, 2 calls, 1 row",
					err, calls, h.count(t, "things"))
			}
			h.assertOneLine(t, "tx.retry", map[string]any{
				"level": "WARN", "code": "db_unavailable", "attempt": float64(1),
			})
			h.assertOneLine(t, "tx.committed", map[string]any{"attempt": float64(2)})
		})
	}
}

func TestDo_givesUpAfterThreeRetries(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	uow := db.New(h.pool, h.ids, clock.Real{})
	calls := 0
	cause := &pgconn.PgError{Code: "40001"}
	err := uow.Do(h.ctx(t, "user:u1"), func(context.Context, db.Tx) error {
		calls++
		return cause
	})
	if errs.CodeOf(err) != errs.CodeDBUnavailable || !errors.Is(err, cause) || calls != 4 {
		t.Fatalf("Do = %v after %d calls, want db_unavailable wrapping the cause after 4", err, calls)
	}
	if len(h.linesNamed(t, "tx.rolled_back")) != 4 || len(h.linesNamed(t, "tx.retry")) != 3 || h.pendingSignals() != 0 {
		t.Fatalf("log lines = %v", h.lines(t))
	}
}

func TestDo_doesNotRetryANonTransientFailure(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	uow := db.New(h.pool, h.ids, clock.Real{})
	calls := 0
	cause := &pgconn.PgError{Code: "23505"}
	err := uow.Do(h.ctx(t, "user:u1"), func(context.Context, db.Tx) error {
		calls++
		return cause
	})
	if errs.CodeOf(err) != errs.CodeInternal || !errors.Is(err, cause) || calls != 1 {
		t.Fatalf("Do = %v after %d calls, want internal wrapping the cause after exactly 1", err, calls)
	}
	if len(h.linesNamed(t, "tx.retry")) != 0 {
		t.Fatalf("log lines = %v, want no tx.retry", h.lines(t))
	}
}

type cancelOn struct {
	sql    string
	cancel context.CancelFunc
}

func (c cancelOn) TraceQueryStart(ctx context.Context, _ *pgx.Conn, d pgx.TraceQueryStartData) context.Context {
	if strings.EqualFold(d.SQL, c.sql) {
		c.cancel()
	}
	return ctx
}

func (cancelOn) TraceQueryEnd(context.Context, *pgx.Conn, pgx.TraceQueryEndData) {}

func TestDo_commitOutlivesACancelThatArrivesDuringCommit(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	ctx, cancel := context.WithCancel(h.ctx(t, "user:u1"))
	defer cancel()
	cfg := h.pool.Config()
	cfg.ConnConfig.Tracer = cancelOn{sql: "commit", cancel: cancel}
	traced, err := pgxpool.NewWithConfig(t.Context(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer traced.Close()
	err = db.New(traced, h.ids, h.clock).Do(ctx, func(ctx context.Context, tx db.Tx) error {
		return h.insertThing(ctx, tx, 1)
	})
	committed := h.count(t, "things") == 1
	if committed && err != nil {
		t.Fatalf("Do = %v but the row committed; a retrying caller would apply it twice", err)
	}
	if !committed || err != nil {
		t.Fatalf("Do = %v, committed=%v; want the commit to finish under its own context", err, committed)
	}
}

func TestDo_doesNotCommitWhenTheContextEndedBeforeCommit(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	ctx, cancel := context.WithCancel(h.ctx(t, "user:u1"))
	defer cancel()
	err := h.uow.Do(ctx, func(ctx context.Context, tx db.Tx) error {
		if err := h.insertThing(ctx, tx, 1); err != nil {
			return err
		}
		cancel()
		return nil
	})
	if errs.CodeOf(err) != errs.CodeDBUnavailable || !errors.Is(err, context.Canceled) {
		t.Fatalf("Do = %v, want db_unavailable wrapping context.Canceled", err)
	}
	if h.count(t, "things") != 0 || h.pendingSignals() != 0 {
		t.Fatalf("things=%d signals=%d after a cancel before commit, want zeros",
			h.count(t, "things"), h.pendingSignals())
	}
}

func TestDo_stopsRetryingWhenTheContextEndsDuringBackoff(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	ctx, cancel := context.WithCancel(h.ctx(t, "user:u1"))
	defer cancel()
	calls := 0
	err := h.uow.Do(ctx, func(context.Context, db.Tx) error {
		calls++
		cancel()
		return &pgconn.PgError{Code: "40001"}
	})
	if errs.CodeOf(err) != errs.CodeDBUnavailable || !errors.Is(err, context.Canceled) || calls != 1 {
		t.Fatalf("Do = %v after %d calls, want db_unavailable wrapping context.Canceled after 1", err, calls)
	}
}

func TestDo_failsWhenTheTransactionCannotStart(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	ctx, cancel := context.WithCancel(h.ctx(t, "user:u1"))
	cancel()
	err := h.uow.Do(ctx, func(context.Context, db.Tx) error {
		t.Fatal("closure ran without a transaction")
		return nil
	})
	if errs.CodeOf(err) != errs.CodeDBUnavailable || !errors.Is(err, context.Canceled) {
		t.Fatalf("Do = %v, want db_unavailable wrapping context.Canceled", err)
	}
}

func TestDo_reportsACommitRejectedByADeferredConstraint(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	if _, err := h.pool.Exec(t.Context(), `
		CREATE TABLE owners (id int PRIMARY KEY);
		CREATE TABLE pets (owner_id int REFERENCES owners (id) DEFERRABLE INITIALLY DEFERRED)`); err != nil {
		t.Fatal(err)
	}
	err := h.uow.Do(h.ctx(t, "user:u1"), func(ctx context.Context, tx db.Tx) error {
		if _, err := tx.Queries().Exec(ctx, `INSERT INTO pets VALUES (42)`); err != nil {
			return err
		}
		return tx.Events.Append(ctx, pinged(h.ids))
	})
	var pg *pgconn.PgError
	if errs.CodeOf(err) != errs.CodeInternal || !errors.As(err, &pg) || pg.Code != "23503" {
		t.Fatalf("Do = %v, want internal wrapping foreign_key_violation", err)
	}
	if h.count(t, "events") != 0 || h.pendingSignals() != 0 {
		t.Fatalf("events=%d signals=%d after a failed commit, want zeros", h.count(t, "events"), h.pendingSignals())
	}
}

func TestDo_coalescesABurstOfCommitsIntoOnePendingSignal(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	for i := range 100 {
		err := h.uow.Do(h.ctx(t, "user:u1"), func(ctx context.Context, tx db.Tx) error {
			return h.insertThing(ctx, tx, i)
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	if h.count(t, "things") != 100 || h.pendingSignals() != 1 {
		t.Fatalf("things=%d signals=%d after 100 commits, want 100 and 1", h.count(t, "things"), h.pendingSignals())
	}
	<-h.uow.Signal()
	if h.pendingSignals() != 0 {
		t.Fatal("a second signal was pending after one receive")
	}
}

type unmarshalable struct {
	V  int      `json:"v"`
	Ch chan int `json:"ch"`
}

func (unmarshalable) Type() events.Type { return "test.unmarshalable" }

func (unmarshalable) AggregateType() string { return "test" }

func (unmarshalable) AggregateID() uuid.UUID { return uuid.UUID{1} }

func TestAppend_refusesWhatItCannotWrite(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	cases := map[string]struct {
		actor  string
		ev     events.Event
		pgCode string
		attr   string
	}{
		"no actor in context":           {actor: "", ev: pinged(h.ids), attr: "actor"},
		"actor without an id":           {actor: "system", ev: pinged(h.ids), attr: "actor"},
		"unmarshalable payload":         {actor: "user:u1", ev: unmarshalable{V: 1}, attr: "type"},
		"actor type the schema rejects": {actor: "robot:r2", ev: pinged(h.ids), pgCode: "23514"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			err := h.uow.Do(h.ctx(t, tc.actor), func(ctx context.Context, tx db.Tx) error {
				return tx.Events.Append(ctx, tc.ev)
			})
			attrs := codedError(t, err, errs.CodeInternal)
			if tc.attr != "" && attr(attrs, tc.attr) == "<missing>" {
				t.Fatalf("err = %v, want attr %q", err, tc.attr)
			}
			var pg *pgconn.PgError
			if tc.pgCode != "" && (!errors.As(err, &pg) || pg.Code != tc.pgCode) {
				t.Fatalf("err = %v, want to wrap pg code %s", err, tc.pgCode)
			}
		})
	}
	if h.count(t, "events") != 0 {
		t.Fatalf("%d events written by refused appends", h.count(t, "events"))
	}
}

func TestAfterCommit_runsOnlyTheCommittedAttemptsCallbacksAfterTheCommit(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	uow := db.New(h.pool, h.ids, clock.Real{})
	var seen []string
	err := uow.Do(h.ctx(t, "user:u1"), func(ctx context.Context, tx db.Tx) error {
		attempt := len(seen) + 1
		seen = append(seen, "")
		tx.AfterCommit(func(ctx context.Context) {
			var n int
			if err := h.pool.QueryRow(ctx, `SELECT count(*) FROM things`).Scan(&n); err != nil {
				t.Error(err)
			}
			seen[attempt-1] = strings.Repeat("x", n)
		})
		if err := h.insertThing(ctx, tx, attempt); err != nil {
			return err
		}
		if attempt == 1 {
			return &pgconn.PgError{Code: "40001"}
		}
		return nil
	})
	if err != nil || len(seen) != 2 || seen[0] != "" || seen[1] != "x" {
		t.Fatalf("Do = %v, callbacks saw %q; want only the second attempt's, run after its row committed", err, seen)
	}
	refused := errs.New(errs.CodeNotFound, "thing.Find")
	ran := false
	err = uow.Do(h.ctx(t, "user:u1"), func(_ context.Context, tx db.Tx) error {
		tx.AfterCommit(func(context.Context) { ran = true })
		return refused
	})
	if !errors.Is(err, refused) || ran {
		t.Fatalf("Do = %v, callback ran %v; want the refusal and no callback", err, ran)
	}
}
