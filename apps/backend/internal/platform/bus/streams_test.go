package bus_test

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	natsserver "github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go/jetstream"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/platform/bus"
	"github.com/monaco/monaco/apps/backend/internal/platform/config"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

func TestStreams_matchTheRFC(t *testing.T) {
	t.Parallel()
	want := []jetstream.StreamConfig{
		{
			Name:       "EVENTS",
			Subjects:   events.Subjects(),
			Retention:  jetstream.LimitsPolicy,
			Storage:    jetstream.FileStorage,
			Replicas:   1,
			MaxBytes:   2 << 30,
			MaxAge:     7 * 24 * time.Hour,
			Discard:    jetstream.DiscardNew,
			Duplicates: 2 * time.Minute,
		},
		{
			Name:       "DEADLETTER",
			Subjects:   []string{"deadletter.>"},
			Retention:  jetstream.LimitsPolicy,
			Storage:    jetstream.FileStorage,
			Replicas:   1,
			MaxBytes:   512 << 20,
			MaxAge:     30 * 24 * time.Hour,
			Discard:    jetstream.DiscardOld,
			Duplicates: 2 * time.Minute,
		},
	}
	if got := bus.Streams(); !reflect.DeepEqual(got, want) {
		t.Fatalf("Streams() = %+v, want %+v", got, want)
	}
}

func TestApply_createsThenReportsNoChanges(t *testing.T) {
	t.Parallel()
	b := testkit.NATS(t)

	changes, err := b.Conn.Apply(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	want := b.Events + ": no changes\n" + b.DeadLetter + ": no changes\n"
	if got := render(changes); got != want {
		t.Fatalf("second apply = %q, want %q", got, want)
	}
	s, err := b.JS.Stream(t.Context(), b.Events)
	if err != nil {
		t.Fatal(err)
	}
	cfg := s.CachedInfo().Config
	if cfg.Discard != jetstream.DiscardNew || cfg.Duplicates != 2*time.Minute {
		t.Fatalf("EVENTS discard %s duplicates %s, want DiscardNew and 2m", cfg.Discard, cfg.Duplicates)
	}
}

func TestApply_onAFreshAccountCreatesBothStreams(t *testing.T) {
	t.Parallel()
	conn := freshConn(t)

	changes, err := conn.Apply(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	want := conn.Stream("EVENTS") + ": created\n" + conn.Stream("DEADLETTER") + ": created\n"
	if got := render(changes); got != want {
		t.Fatalf("first apply = %q, want %q", got, want)
	}
}

func TestApply_revertsDriftAndPrintsTheDiff(t *testing.T) {
	t.Parallel()
	b := testkit.NATS(t)
	s, err := b.JS.Stream(t.Context(), b.Events)
	if err != nil {
		t.Fatal(err)
	}
	drifted := s.CachedInfo().Config
	drifted.MaxBytes = 1 << 20
	drifted.Duplicates = time.Minute
	if _, err := b.JS.UpdateStream(t.Context(), drifted); err != nil {
		t.Fatal(err)
	}

	changes, err := b.Conn.Apply(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	want := b.Events + ": updated\n  MaxBytes: 1048576 -> 2147483648\n  Duplicates: 1m0s -> 2m0s\n" +
		b.DeadLetter + ": no changes\n"
	if got := render(changes); got != want {
		t.Fatalf("apply after drift = %q, want %q", got, want)
	}
	again, err := b.Conn.Apply(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if got := render(again); strings.Count(got, "no changes") != 2 {
		t.Fatalf("apply after revert = %q, want no changes for both streams", got)
	}
}

func TestApply_failsWhenTheServerRefusesTheUpdate(t *testing.T) {
	t.Parallel()
	b := testkit.NATS(t)
	conn := freshConn(t)
	memory := bus.Streams()[0]
	memory.Name = conn.Stream(memory.Name)
	memory.Subjects = []string{conn.Subject("events.>")}
	memory.Storage = jetstream.MemoryStorage
	if _, err := b.JS.CreateStream(t.Context(), memory); err != nil {
		t.Fatal(err)
	}

	changes, err := conn.Apply(t.Context())
	if errs.CodeOf(err) != errs.CodeUpstreamUnavailable || len(changes) != 0 {
		t.Fatalf("apply over a memory stream = %v %v, want upstream_unavailable and no changes", changes, err)
	}
}

func TestApply_neverReportsNoChangesOverAStreamWithTheWrongZeroValuedFields(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name  string
		drift func(*jetstream.StreamConfig)
	}{
		{"events work queue", func(c *jetstream.StreamConfig) { c.Retention = jetstream.WorkQueuePolicy }},
		{"events memory", func(c *jetstream.StreamConfig) { c.Storage = jetstream.MemoryStorage }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			b := testkit.NATS(t)
			conn := freshConn(t)
			drifted := bus.Streams()[0]
			drifted.Name = conn.Stream(drifted.Name)
			for i, s := range drifted.Subjects {
				drifted.Subjects[i] = conn.Subject(s)
			}
			tc.drift(&drifted)
			if _, err := b.JS.CreateStream(t.Context(), drifted); err != nil {
				t.Fatal(err)
			}

			changes, err := conn.Apply(t.Context())
			if err == nil && (len(changes) == 0 || changes[0].Action != bus.ActionUpdated) {
				t.Fatalf("apply over %s = %q, want updated or refused", tc.name, render(changes))
			}
		})
	}
}

func TestApply_revertsADeadLetterEditedToDiscardNew(t *testing.T) {
	t.Parallel()
	b := testkit.NATS(t)
	s, err := b.JS.Stream(t.Context(), b.DeadLetter)
	if err != nil {
		t.Fatal(err)
	}
	drifted := s.CachedInfo().Config
	drifted.Discard = jetstream.DiscardNew
	if _, err := b.JS.UpdateStream(t.Context(), drifted); err != nil {
		t.Fatal(err)
	}

	changes, err := b.Conn.Apply(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if got, want := render(
		changes,
	), b.Events+": no changes\n"+b.DeadLetter+": updated\n  Discard: DiscardNew -> DiscardOld\n"; got != want {
		t.Fatalf("apply after DEADLETTER drift = %q, want %q", got, want)
	}
}

func TestVerifyStreams_failsBootWithAPointerToBusApply(t *testing.T) {
	t.Parallel()
	conn := freshConn(t)

	err := conn.VerifyStreams(t.Context())
	if errs.CodeOf(err) != errs.CodeNotFound || !strings.Contains(err.Error(), "run monacoctl bus apply") {
		t.Fatalf("VerifyStreams = %v, want not_found pointing at monacoctl bus apply", err)
	}
	if _, err := conn.Apply(t.Context()); err != nil {
		t.Fatal(err)
	}
	if err := conn.VerifyStreams(t.Context()); err != nil {
		t.Fatalf("VerifyStreams after apply = %v, want nil", err)
	}
}

func TestConnect_failsWhenNATSIsUnreachable(t *testing.T) {
	t.Parallel()
	_, err := bus.Connect(t.Context(), config.NATS{URL: "nats://127.0.0.1:1"}, bus.ProcessAPI)
	if errs.CodeOf(err) != errs.CodeUpstreamUnavailable {
		t.Fatalf("Connect = %v, want upstream_unavailable", err)
	}
}

func freshConn(t *testing.T) *bus.Conn {
	t.Helper()
	admin := testkit.NATS(t).JS
	ns := "fresh_" + strings.NewReplacer("/", "_", "#", "_").Replace(t.Name())
	conn, err := bus.Connect(t.Context(), config.NATS{URL: testkit.NATSURL()}, bus.ProcessMonacoctl,
		bus.WithNamespace(ns))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		conn.Close(context.Background())
		for _, name := range []string{conn.Stream("EVENTS"), conn.Stream("DEADLETTER")} {
			if err := admin.DeleteStream(context.Background(), name); err != nil &&
				!errors.Is(err, jetstream.ErrStreamNotFound) {
				t.Errorf("delete %s: %v", name, err)
			}
		}
	})
	return conn
}

func render(changes []bus.StreamChange) string {
	var b strings.Builder
	for _, c := range changes {
		b.WriteString(c.String())
	}
	return b.String()
}

func TestConnect_failsWhenTheServerHasNoJetStream(t *testing.T) {
	t.Parallel()
	srv, err := natsserver.NewServer(&natsserver.Options{
		Host: "127.0.0.1", Port: natsserver.RANDOM_PORT, NoLog: true, NoSigs: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	srv.Start()
	t.Cleanup(func() {
		srv.Shutdown()
		srv.WaitForShutdown()
	})
	if !srv.ReadyForConnections(5 * time.Second) {
		t.Fatal("core nats-server not ready")
	}
	_, err = bus.Connect(t.Context(), config.NATS{URL: srv.ClientURL()}, bus.ProcessAPI)
	if errs.CodeOf(err) != errs.CodeUpstreamUnavailable {
		t.Fatalf("Connect without JetStream = %v, want upstream_unavailable", err)
	}
}

func TestApply_failsWhenAForeignStreamHoldsTheSubjects(t *testing.T) {
	t.Parallel()
	b := testkit.NATS(t)
	conn := freshConn(t)
	foreign := jetstream.StreamConfig{Name: conn.Stream("FOREIGN"), Subjects: []string{conn.Subject("events.>")}}
	if _, err := b.JS.CreateStream(t.Context(), foreign); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = b.JS.DeleteStream(context.Background(), foreign.Name) })

	changes, err := conn.Apply(t.Context())
	if errs.CodeOf(err) != errs.CodeUpstreamUnavailable || len(changes) != 0 {
		t.Fatalf("apply with the subjects taken = %v %v, want upstream_unavailable and no changes", changes, err)
	}
}

func TestApply_failsWhenTheStreamLookupFails(t *testing.T) {
	t.Parallel()
	conn := freshConn(t)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	changes, err := conn.Apply(ctx)
	if errs.CodeOf(err) != errs.CodeUpstreamUnavailable || len(changes) != 0 {
		t.Fatalf("apply with a cancelled context = %v %v, want upstream_unavailable and no changes", changes, err)
	}
}
