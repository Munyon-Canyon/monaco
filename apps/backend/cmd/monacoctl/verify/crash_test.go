package verify

import (
	"context"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/nats-io/nats.go/jetstream"

	"github.com/monaco/monaco/apps/backend/internal/platform/bus"
	"github.com/monaco/monaco/apps/backend/internal/platform/faultpoint"
	tools "github.com/monaco/monaco/apps/backend/internal/tools/flows"
)

func TestStack_crashRestartsTheWorkerUnarmedAndArmReArmsIt(t *testing.T) {
	t.Parallel()
	o := testOptions(t, "ok")
	o.Faultpoint = string(faultpoint.AfterPublish)
	s, err := Up(t.Context(), o)
	defer func() { _ = s.Down(t.Context()) }()
	if err != nil {
		t.Fatal(err)
	}
	if err := s.arm(t.Context()); err != nil {
		t.Fatalf("arm while armed: %v", err)
	}
	if err := s.armProcess(t.Context(), procWorker, "ignored"); err != nil {
		t.Fatalf("arm twice: %v", err)
	}
	for range 2 {
		if err := s.crash(t.Context(), faultpoint.AfterPublish); err != nil {
			t.Fatalf("crash: %v", err)
		}
		if !s.procs[procWorker].running() || s.armed {
			t.Fatal("worker not restarted unarmed")
		}
		if err := s.arm(t.Context()); err != nil || !s.armed {
			t.Fatalf("arm: %v", err)
		}
	}
	if s.Crashes != 2 {
		t.Fatalf("Crashes = %d, want 2", s.Crashes)
	}
	checkCrashFailures(t, s)
	restartAPI(t, s)
}

func TestStack_crashReturnsAnAPIStopFailure(t *testing.T) {
	t.Parallel()
	o := testOptions(t, fakeDeaf)
	o.Budget.Teardown = 100 * time.Millisecond
	s, err := Up(t.Context(), o)
	defer func() { _ = s.Down(t.Context()) }()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Millisecond)
	defer cancel()
	u := Unit{Flow: tools.Flow{ID: "01", Trigger: "GET /healthz"}}
	if err := s.crashUnit(ctx, u, faultpoint.BeforeCommit); err == nil ||
		!strings.Contains(err.Error(), "api did not exit within the budget") {
		t.Fatalf("api crash = %v", err)
	}
}

func restartAPI(t *testing.T, s *Stack) {
	t.Helper()
	route := Unit{Flow: tools.Flow{ID: "01", Trigger: "GET /healthz"}}
	s.opts.Faultpoint = string(faultpoint.BeforeCommit)
	if err := s.armUnit(t.Context(), route); err != nil {
		t.Fatalf("arm api: %v", err)
	}
	if err := s.crashUnit(t.Context(), route, faultpoint.BeforeCommit); err != nil {
		t.Fatalf("restart api: %v", err)
	}
	if !s.procs[procAPI].running() || s.armed {
		t.Fatal("api not restarted unarmed")
	}
	for _, want := range []string{"TRUST_PROXY_HEADERS=true", "MONACO_BUS_API_RELAY=off"} {
		if !slices.Contains(s.procs[procAPI].cmd.Env, want) {
			t.Fatalf("restarted API env = %v, want %q", s.procs[procAPI].cmd.Env, want)
		}
	}
}

func TestStack_crashRecognizesAnExitedWorkerAndACancelledWait(t *testing.T) {
	t.Parallel()
	o := testOptions(t, "ok")
	o.Faultpoint = string(faultpoint.AfterPublish)
	s, err := Up(t.Context(), o)
	defer func() { _ = s.Down(t.Context()) }()
	if err != nil {
		t.Fatal(err)
	}
	if err := s.arm(t.Context()); err != nil {
		t.Fatal(err)
	}
	_ = healthz(t.Context(), s.Worker)
	<-s.procs[procWorker].exited
	if err := s.crash(t.Context(), faultpoint.AfterPublish); err != nil {
		t.Fatalf("restart exited worker: %v", err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if err := s.crash(ctx, faultpoint.AfterPublish); err == nil ||
		!strings.Contains(err.Error(), "worker did not crash at after-publish") {
		t.Fatalf("cancelled crash = %v", err)
	}
}

func TestProcessFor_routesCrashToTheProcessThatRunsTheCommand(t *testing.T) {
	t.Parallel()
	if got := processFor(Unit{Flow: tools.Flow{Trigger: "GET /healthz"}}, faultpoint.BeforeCommit); got != procAPI {
		t.Fatalf("route process = %q, want %q", got, procAPI)
	}
	if got := processFor(Unit{Flow: tools.Flow{Trigger: "GET /healthz"}}, faultpoint.AfterPublish); got != procWorker {
		t.Fatalf("published route process = %q, want %q", got, procWorker)
	}
	for _, point := range []faultpoint.Name{faultpoint.AfterCreate, faultpoint.AfterExecute} {
		if got := processFor(Unit{Flow: tools.Flow{Trigger: "POST /v1/swaps/{id}/retry"}}, point); got != procWorker {
			t.Fatalf("%s on a route = %q, want %q: only the worker's swap layer hits it", point, got, procWorker)
		}
	}
	if got := processFor(
		Unit{Flow: tools.Flow{Trigger: "poller:market.prices"}}, faultpoint.BeforeCommit,
	); got != procWorker {
		t.Fatalf("poller process = %q, want %q", got, procWorker)
	}
}

func TestProcessLine_keepsTheFirstListeningAddress(t *testing.T) {
	t.Parallel()
	p := &process{logs: &Logs{}, listening: make(chan string, 1)}
	p.line(Line{Text: `{"msg":"boot.listening","addr":"127.0.0.1:1"}`})
	p.line(Line{Text: `{"msg":"boot.listening","addr":"127.0.0.1:2"}`})
	if got := <-p.listening; got != "127.0.0.1:1" {
		t.Fatalf("listening address = %q, want first address", got)
	}
}

func checkCrashFailures(t *testing.T, s *Stack) {
	t.Helper()
	if err := s.crash(t.Context(), faultpoint.AfterSign); err == nil ||
		!strings.Contains(err.Error(), `without "faultpoint: crash at after-sign"`) {
		t.Fatalf("crash at the wrong point = %v", err)
	}
	s.opts.Faultpoint, s.armed = "", false
	if err := s.arm(t.Context()); err != nil {
		t.Fatalf("arm without a faultpoint: %v", err)
	}
	if err := s.startWorker(t.Context()); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 100*time.Millisecond)
	defer cancel()
	if err := s.crash(ctx, faultpoint.AfterPublish); err == nil ||
		!strings.Contains(err.Error(), "worker did not crash at after-publish") {
		t.Fatalf("crash of a worker that never crashes = %v", err)
	}
}

func TestStack_armWaitsForEarlierEventsAndIdleConsumers(t *testing.T) {
	t.Parallel()
	s, err := Up(t.Context(), testOptions(t, "ok"))
	defer func() { _ = s.Down(t.Context()) }()
	if err != nil {
		t.Fatal(err)
	}
	if busy, err := s.busy(t.Context()); err != nil || busy != "" {
		t.Fatalf("idle stack busy = %q, %v", busy, err)
	}
	insertUnpublishedEvent(t, s)
	ctx, cancel := context.WithTimeout(t.Context(), 50*time.Millisecond)
	defer cancel()
	err = s.armProcess(ctx, procWorker, string(faultpoint.AfterPublish))
	if err == nil || !strings.Contains(err.Error(), "1 events unpublished") {
		t.Fatalf("arm with an unpublished row = %v, want it to name the row", err)
	}
	if s.armed || !s.procs[procWorker].running() {
		t.Fatal("arm restarted the worker while an earlier row was unpublished")
	}
	cancelled, stop := context.WithCancel(t.Context())
	stop()
	if _, err := s.busy(cancelled); err == nil {
		t.Fatal("busy with a cancelled context returned no error")
	}
	addIdleConsumer(t, s)
	if busy, err := s.busy(t.Context()); err != nil || !strings.Contains(busy, "events unpublished") {
		t.Fatalf("busy with an idle consumer = %q, %v, want the unpublished row", busy, err)
	}
}

func TestConsumerBusy_namesPendingAndUnackedMessages(t *testing.T) {
	t.Parallel()
	if got := consumerBusy(&jetstream.ConsumerInfo{Name: "idle"}); got != "" {
		t.Fatalf("idle consumer = %q", got)
	}
	got := consumerBusy(&jetstream.ConsumerInfo{Name: "trading", NumPending: 2, NumAckPending: 1})
	if got != "consumer trading has 2 pending and 1 unacked messages" {
		t.Fatalf("busy consumer = %q", got)
	}
}

func insertUnpublishedEvent(t *testing.T, s *Stack) {
	t.Helper()
	_, err := s.Pool.Exec(t.Context(), `INSERT INTO events
		(id, aggregate_type, aggregate_id, type, payload, actor_type, actor_id)
		VALUES (gen_random_uuid(), 'system', gen_random_uuid(), 'system.pinged', '{"v":1}', 'system', 'test')`)
	if err != nil {
		t.Fatal(err)
	}
}

func addIdleConsumer(t *testing.T, s *Stack) {
	t.Helper()
	stream, err := s.NATS.JS.Stream(t.Context(), bus.StreamEvents)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := stream.CreateConsumer(t.Context(), jetstream.ConsumerConfig{Durable: "probe"}); err != nil {
		t.Fatal(err)
	}
}
