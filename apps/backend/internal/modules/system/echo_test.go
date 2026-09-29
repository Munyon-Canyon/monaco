package system_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/system/adapters"
	"github.com/monaco/monaco/apps/backend/internal/modules/system/app"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
)

type hints struct {
	mu   sync.Mutex
	keys []string
}

func (h *hints) PublishHint(_ context.Context, key string, _ []byte) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.keys = append(h.keys, key)
}

func (f fixture) echo(t *testing.T, e adapters.Echo, ev events.SystemPinged) error {
	t.Helper()
	return f.uow.Do(t.Context(), func(ctx context.Context, tx db.Tx) error {
		return e.Handle(ctx, tx, ev, time.Date(2026, 3, 1, 12, 0, 1, 0, time.UTC))
	})
}

func TestEcho_marksThePingEchoedOnceAndHintsItsUserAfterCommit(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	ping, err := f.record(t, "hi")
	if err != nil {
		t.Fatal(err)
	}
	sent := &hints{}
	e := adapters.Echo{Hints: sent}
	ev := events.SystemPinged{V: 1, PingID: ping.ID, UserID: f.user.UUID(), Note: "hi"}
	for range 2 {
		if err := f.echo(t, e, ev); err != nil {
			t.Fatal(err)
		}
	}
	got, err := app.GetPing(t.Context(), f.pool, ping.ID, f.user)
	want := []string{"user." + f.user.String() + ".ping_echoed"}
	if err != nil || !got.Echoed || len(sent.keys) != 1 || sent.keys[0] != want[0] {
		t.Fatalf("after two echoes ping = %+v, %v, hints %q; want echoed once with hint %q", got, err, sent.keys, want)
	}
}

func TestEcho_rebuildsAPingItHasNotSeenFromTheEvent(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	id := f.ids.NewV7()
	e := adapters.Echo{Hints: &hints{}}
	if err := f.echo(t, e, events.SystemPinged{V: 1, PingID: id, UserID: f.user.UUID(), Note: "replayed"}); err != nil {
		t.Fatal(err)
	}
	got, err := app.GetPing(t.Context(), f.pool, id, f.user)
	if err != nil || got != (app.Ping{ID: id, Note: "replayed", Echoed: true}) {
		t.Fatalf("GetPing = %+v, %v, want the replayed ping echoed", got, err)
	}
}

func TestEcho_returnsTheWriteErrorAndHintsNothing(t *testing.T) {
	t.Parallel()
	for name, ddl := range map[string]string{
		"insert": `ALTER TABLE system_pings RENAME TO system_pings_gone`,
		"echo":   `ALTER TABLE system_pings ADD CONSTRAINT never_echoed CHECK (echoed_at IS NULL)`,
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			f := newFixture(t)
			if _, err := f.pool.Exec(t.Context(), ddl); err != nil {
				t.Fatal(err)
			}
			sent := &hints{}
			e := adapters.Echo{Hints: sent}
			err := f.echo(t, e, events.SystemPinged{V: 1, PingID: f.ids.NewV7(), UserID: f.user.UUID()})
			if errs.CodeOf(err) != errs.CodeInternal || len(sent.keys) != 0 {
				t.Fatalf("echo = %v with hints %q, want internal and none", err, sent.keys)
			}
		})
	}
}
