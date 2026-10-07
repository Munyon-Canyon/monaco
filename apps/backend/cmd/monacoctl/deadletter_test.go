package main

import (
	"context"
	"encoding/json"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/system"
	"github.com/monaco/monaco/apps/backend/internal/modules/system/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/system/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/bus"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/config"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

const converge = 10 * time.Second

type echoWorker struct {
	conn    *bus.Conn
	pool    *pgxpool.Pool
	uow     *db.UnitOfWork
	ids     *testkit.IDs
	env     []string
	failAs  atomic.Value
	applied atomic.Int32
}

func natsEnv(url string) []string {
	return []string{"MONACO_ENV=test", "DATABASE_URL=postgres://localhost/monaco", "NATS_URL=" + url}
}

func startEcho(t *testing.T, failAs errs.Code) *echoWorker {
	t.Helper()
	url := testkit.StandaloneNATS(t)
	conn, err := bus.Connect(t.Context(), config.NATS{URL: url}, "test")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close(context.WithoutCancel(t.Context())) })
	if _, err := conn.Apply(t.Context()); err != nil {
		t.Fatal(err)
	}
	w := &echoWorker{conn: conn, pool: testkit.DB(t), ids: testkit.NewIDs(1), env: natsEnv(url)}
	w.failAs.Store(failAs)
	w.uow = db.New(w.pool, w.ids, clock.Real{})
	echo := system.New(module.Deps{Clock: clock.Real{}, Pool: w.pool, UoW: w.uow, Bus: conn}).Consumers()[0].Handlers[0]
	flaky := bus.Handle(echo.Name, func(ctx context.Context, tx db.Tx, e events.SystemPinged, at time.Time) error {
		if code := w.failAs.Load().(errs.Code); code != "" {
			return errs.New(code, "fixture.echo")
		}
		if err := echo.Apply(ctx, tx, e, at); err != nil {
			return err
		}
		w.applied.Add(1)
		return nil
	})
	consumer := bus.Consumer{
		Durable:   "system_echo",
		Handlers:  []bus.HandlerSpec{flaky},
		NakDelays: []time.Duration{time.Millisecond},
	}
	registry, err := bus.NewRegistry(
		conn,
		w.uow,
		clock.Real{},
		[]bus.Consumer{consumer},
		bus.WithAckWait(testkit.DefaultAckWait),
	)
	if err != nil {
		t.Fatal(err)
	}
	stop, err := registry.Start(context.WithoutCancel(t.Context()))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(stop)
	return w
}

func (w *echoWorker) ping(t *testing.T) uuid.UUID {
	t.Helper()
	user, err := ids.ParseUserID(w.ids.NewV7().String())
	if err != nil {
		t.Fatal(err)
	}
	note, err := domain.ParseNote("hi")
	if err != nil {
		t.Fatal(err)
	}
	cmd := app.RecordPing{ID: w.ids.NewV7(), UserID: user, Note: note}
	ctx := observability.WithActor(t.Context(), "user:"+user.String())
	if _, err := app.NewRecordPingHandler(w.uow).Handle(ctx, cmd); err != nil {
		t.Fatal(err)
	}
	var id uuid.UUID
	var payload []byte
	if err := w.pool.QueryRow(t.Context(), `SELECT id, payload FROM events WHERE aggregate_id = $1`, cmd.ID).
		Scan(&id, &payload); err != nil {
		t.Fatal(err)
	}
	if err := w.conn.Publish(t.Context(), events.TypeSystemPinged.Subject(), payload, ids.EventIDFrom(id)); err != nil {
		t.Fatal(err)
	}
	return cmd.ID
}

func (w *echoWorker) echoed(t *testing.T, ping uuid.UUID) bool {
	t.Helper()
	var echoed bool
	if err := w.pool.QueryRow(t.Context(), `SELECT echoed_at IS NOT NULL FROM system_pings WHERE id = $1`, ping).
		Scan(&echoed); err != nil {
		t.Fatal(err)
	}
	return echoed
}

func (w *echoWorker) letters(t *testing.T, n int) []string {
	t.Helper()
	var lines []string
	testkit.Eventually(t, func() bool {
		code, stdout, stderr := runOps(w.env, "deadletter", "list")
		if code != 0 {
			t.Fatalf("deadletter list: code=%d stderr=%q", code, stderr)
		}
		lines = strings.Split(strings.TrimSuffix(stdout, "\n"), "\n")
		return stdout != "" && len(lines) >= n
	}, converge)
	return lines
}

func (w *echoWorker) assertNoMarkerListed(t *testing.T) {
	t.Helper()
	code, stdout, _ := runOps(w.env, "deadletter", "list")
	if code != 0 || strings.Count(stdout, "\n") != 1 || strings.Contains(stdout, "\tok\t") {
		t.Fatalf("deadletter list after two acked retries = %q, want the one letter and no resolution marker", stdout)
	}
}

func TestDeadletterRetry_convergesOnceTheHandlerIsFixedAndARepeatIsANoOp(t *testing.T) {
	t.Parallel()
	w := startEcho(t, errs.CodeInvalidInput)
	ping := w.ping(t)
	lines := w.letters(t, 1)
	if len(lines) != 1 || !strings.HasPrefix(lines[0], "1\tsystem_echo\tsystem.echo\tinvalid_input\t") {
		t.Fatalf("deadletter list = %q, want the termed ping", lines)
	}
	w.failAs.Store(errs.Code(""))
	code, stdout, stderr := runOps(w.env, "deadletter", "retry", "1")
	if code != 0 || !strings.HasPrefix(stdout, "retried 1 ") || stderr != "" {
		t.Fatalf("retry: code=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
	testkit.Eventually(t, func() bool { return w.echoed(t, ping) }, converge)
	code, stdout, stderr = runOps(w.env, "deadletter", "retry", "--all", "--consumer", "system_echo")
	if code != 0 || !strings.HasPrefix(stdout, "retried 1 ") || stderr != "" {
		t.Fatalf("second retry: code=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
	barrier := w.ping(t)
	testkit.Eventually(t, func() bool { return w.echoed(t, barrier) }, converge)
	w.assertNoMarkerListed(t)
	var deliveries int
	if err := w.pool.QueryRow(t.Context(), `SELECT count(*) FROM event_deliveries`).Scan(&deliveries); err != nil {
		t.Fatal(err)
	}
	if applied := w.applied.Load(); applied != 2 || deliveries != 2 {
		t.Fatalf(
			"handler applied %d times with %d deliveries, want 2 and 2: the repeated retry must dedupe",
			applied,
			deliveries,
		)
	}
}

func TestDeadletterRetry_refetchesAMessageThatExhaustedMaxDeliver(t *testing.T) {
	t.Parallel()
	w := startEcho(t, errs.CodeUpstreamUnavailable)
	ping := w.ping(t)
	lines := w.letters(t, 1)
	if !strings.Contains(lines[0], "\tmax_deliveries\tsystem_echo/1\t") {
		t.Fatalf("deadletter list = %q, want the max-deliveries advisory", lines)
	}
	w.failAs.Store(errs.Code(""))
	if code, stdout, stderr := runOps(w.env, "deadletter", "retry", "--all", "--consumer", "system_echo"); code != 0 {
		t.Fatalf("retry: code=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
	testkit.Eventually(t, func() bool { return w.echoed(t, ping) }, converge)
}

func TestDeadletter_refusesBadArgumentsAndReportsWhatItCannotRetry(t *testing.T) {
	t.Parallel()
	w := startEcho(t, "")
	for _, letter := range []bus.DeadLetter{
		{Consumer: "lost", Advisory: json.RawMessage(`{"stream_seq": 999}`)},
		{Consumer: "nowhere", Subject: "nowhere", Data: json.RawMessage(`{}`)},
	} {
		body, err := json.Marshal(letter)
		if err != nil {
			t.Fatal(err)
		}
		if err := w.conn.Publish(
			t.Context(),
			"deadletter."+letter.Consumer,
			body,
			ids.EventIDFrom(w.ids.NewV7()),
		); err != nil {
			t.Fatal(err)
		}
	}
	for _, tc := range []struct {
		environ []string
		args    []string
		code    int
		stderr  string
	}{
		{w.env, []string{"deadletter", "list", "now"}, 2, deadletterUsage + "\n"},
		{w.env, []string{"deadletter", "retry"}, 2, deadletterUsage + "\n"},
		{w.env, []string{"deadletter", "retry", "first"}, 2, deadletterUsage + "\n"},
		{w.env, []string{"deadletter", "retry", "--all"}, 2, deadletterUsage + "\n"},
		{w.env, []string{"deadletter", "retry", "7"}, 1, "monacoctl: no dead letter matches\n"},
		{w.env, []string{"deadletter", "retry", "--all", "--consumer", "lost"}, 1, "monacoctl: bus.Redeliver: not_found"},
		{w.env, []string{"deadletter", "retry", "--all", "--consumer", "nowhere"}, 1, "monacoctl: bus.Redeliver: "},
		{natsEnv("nats://127.0.0.1:1"), []string{"deadletter", "list"}, 1, "monacoctl: bus.Connect: "},
		{natsEnv(testkit.StandaloneNATS(t)), []string{"deadletter", "list"}, 1, "monacoctl: bus.DeadLetters: "},
	} {
		code, _, stderr := runOps(tc.environ, tc.args...)
		if code != tc.code || !strings.HasPrefix(stderr, tc.stderr) {
			t.Errorf("%q: code=%d stderr=%q, want %d and %q", tc.args, code, stderr, tc.code, tc.stderr)
		}
	}
	lines := w.letters(t, 2)
	if len(lines) != 2 || !strings.HasPrefix(lines[0], "1\tlost\t\tmax_deliveries\t") {
		t.Fatalf("deadletter list = %q, want both crafted letters", lines)
	}
}
