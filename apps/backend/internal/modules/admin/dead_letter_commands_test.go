package admin_test

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/admin/app"
	"github.com/monaco/monaco/apps/backend/internal/platform/bus"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

const waitLong = 30 * time.Second

func (f *letterFixture) reason(t *testing.T) events.Reason {
	t.Helper()
	reason, err := events.NewReason("posthog fixed")
	if err != nil {
		t.Fatal(err)
	}
	return reason
}

func (f *letterFixture) admin() ids.UserID { return ids.UserIDFrom(f.ids.NewV7()) }

func (f *letterFixture) redriveHandler(r app.Redriver) *app.RedriveDeadLetterHandler {
	return app.NewRedriveDeadLetterHandler(f.uow, f.pool, r, f.clock)
}

func (f *letterFixture) openRow(t *testing.T) (id, event uuid.UUID) {
	t.Helper()
	event = f.ids.NewV7()
	f.put(t, f.termLetter(event, "posthog_rejected"))
	f.tick(t)
	if err := f.pool.QueryRow(t.Context(), `SELECT id FROM dead_letters WHERE event_id = $1`, event).
		Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id, event
}

func (f *letterFixture) setStatus(ctx context.Context, t *testing.T, id uuid.UUID, status string) {
	t.Helper()
	if _, err := f.pool.Exec(ctx, `UPDATE dead_letters SET status = $2 WHERE id = $1`, id, status); err != nil {
		t.Fatal(err)
	}
}

func (f *letterFixture) row(t *testing.T, id uuid.UUID) deadLetterRow {
	t.Helper()
	for _, row := range f.rows(t) {
		if f.rowID(t, row) == id {
			return row
		}
	}
	t.Fatalf("no dead_letters row %s", id)
	return deadLetterRow{}
}

func (f *letterFixture) rowID(t *testing.T, row deadLetterRow) uuid.UUID {
	t.Helper()
	var id uuid.UUID
	if err := f.pool.QueryRow(t.Context(), `SELECT id FROM dead_letters WHERE stream_seq = $1`, row.StreamSeq).
		Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

func (f *letterFixture) eventsStreamLength(t *testing.T) uint64 {
	t.Helper()
	s, err := f.bus.JS.Stream(t.Context(), f.bus.Events)
	if err != nil {
		t.Fatal(err)
	}
	info, err := s.Info(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	return info.State.Msgs
}

func (f *letterFixture) assertMarked(t *testing.T, row deadLetterRow, status string, by ids.UserID) {
	t.Helper()
	if row.Status != status || row.ResolvedBy == nil || *row.ResolvedBy != by.UUID() || row.ResolveReson == nil ||
		*row.ResolveReson != "posthog fixed" {
		t.Fatalf("row = %+v, want %s by %s with the reason", row, status, by)
	}
}

func TestRedriveDeadLetter_marksTheRowRedrivenAndRepublishesTheEventUnderItsOriginalID(t *testing.T) {
	t.Parallel()
	f := newLetterFixture(t)
	id, event := f.openRow(t)
	by := f.admin()
	f.clock.Advance(time.Minute)

	err := f.redriveHandler(f.bus.Conn).
		Handle(t.Context(), app.RedriveDeadLetter{ID: id, AdminID: by, Reason: f.reason(t)})
	if err != nil {
		t.Fatal(err)
	}
	f.assertMarked(t, f.row(t, id), "redriven", by)
	if row := f.row(t, id); row.RedrivenAt == nil || !row.RedrivenAt.Equal(f.clock.Now()) || row.ResolvedAt != nil {
		t.Fatalf("row = %+v, want redriven_at now and no resolved_at", row)
	}
	consumer := f.row(t, id).Consumer
	again, err := f.bus.Conn.EventAt(t.Context(), 1)
	if err != nil || again.Header.Get(bus.EventIDHeader) != event.String() ||
		again.Header.Get(bus.RedrivenHeader) != consumer {
		t.Fatalf("EVENTS message 1 = %+v, %v, want the original event %s marked redriven for %s",
			again, err, event, consumer)
	}
}

func TestRedriveDeadLetter_aSecondRedriveOfTheSameFailureIsDedupedAndOneAfterANewFailureIsNot(t *testing.T) {
	t.Parallel()
	f := newLetterFixture(t)
	id, _ := f.openRow(t)
	h := f.redriveHandler(f.bus.Conn)
	cmd := app.RedriveDeadLetter{ID: id, AdminID: f.admin(), Reason: f.reason(t)}
	for range 2 {
		if err := h.Handle(t.Context(), cmd); err != nil {
			t.Fatal(err)
		}
	}
	if got := f.eventsStreamLength(t); got != 1 {
		t.Fatalf("EVENTS holds %d messages after two redrives of one failure, want 1", got)
	}
	if _, err := f.pool.Exec(t.Context(), `UPDATE dead_letters SET status = 'open', occurrences = 2`); err != nil {
		t.Fatal(err)
	}
	if err := h.Handle(t.Context(), cmd); err != nil {
		t.Fatal(err)
	}
	if got := f.eventsStreamLength(t); got != 2 {
		t.Fatalf("EVENTS holds %d messages after a redrive of a new failure, want 2", got)
	}
}

type flippingRedriver struct {
	f  *letterFixture
	t  *testing.T
	id uuid.UUID
}

func (r flippingRedriver) Redeliver(ctx context.Context, _ bus.DeadLetter, _ string) error {
	r.f.setStatus(ctx, r.t, r.id, "resolved")
	return nil
}

type refusingRedriver struct{ err error }

func (r refusingRedriver) Redeliver(context.Context, bus.DeadLetter, string) error { return r.err }

func TestRedriveDeadLetter_refusesALetterThatIsMissingClosedUnreadableOrResolvedUnderIt(t *testing.T) {
	t.Parallel()
	f := newLetterFixture(t)
	id, _ := f.openRow(t)
	cmd := func(id uuid.UUID) app.RedriveDeadLetter {
		return app.RedriveDeadLetter{ID: id, AdminID: f.admin(), Reason: f.reason(t)}
	}
	for _, tc := range []struct {
		name string
		id   uuid.UUID
		prep func()
		with app.Redriver
		want errs.Code
	}{
		{"unknown", f.ids.NewV7(), func() {}, f.bus.Conn, errs.CodeNotFound},
		{"resolved", id, func() { f.setStatus(t.Context(), t, id, "resolved") }, f.bus.Conn, errs.CodeDeadLetterNotOpen},
		{"discarded", id, func() { f.setStatus(t.Context(), t, id, "discarded") }, f.bus.Conn, errs.CodeDeadLetterNotOpen},
		{"unreadable", id, func() {
			f.setStatus(t.Context(), t, id, "open")
			if _, err := f.pool.Exec(t.Context(), `UPDATE dead_letters SET letter = '"text"'`); err != nil {
				t.Fatal(err)
			}
		}, f.bus.Conn, errs.CodeInternal},
		{"bus refuses", id, func() {
			if _, err := f.pool.Exec(t.Context(), `UPDATE dead_letters SET letter = '{}'`); err != nil {
				t.Fatal(err)
			}
		}, refusingRedriver{errs.New(errs.CodeUpstreamUnavailable, "nats")}, errs.CodeUpstreamUnavailable},
		{"resolved while republishing", id, func() {}, flippingRedriver{f, t, id}, errs.CodeDeadLetterNotOpen},
	} {
		tc.prep()
		err := f.redriveHandler(tc.with).Handle(t.Context(), cmd(tc.id))
		if errs.CodeOf(err) != tc.want {
			t.Fatalf("%s: redrive = %v, want %s", tc.name, err, tc.want)
		}
	}
	if row := f.row(t, id); row.RedrivenAt != nil {
		t.Fatalf("row = %+v, want no redriven_at after every refusal", row)
	}
}

func TestRedriveDeadLetter_aGoneAdvisoryEventAnswersNotFoundAndLeavesTheRow(t *testing.T) {
	t.Parallel()
	f := newLetterFixture(t)
	f.put(t, advisoryFor("notify", 9))
	f.tick(t)
	var id uuid.UUID
	if err := f.pool.QueryRow(t.Context(), `SELECT id FROM dead_letters`).Scan(&id); err != nil {
		t.Fatal(err)
	}

	err := f.redriveHandler(f.bus.Conn).Handle(t.Context(), app.RedriveDeadLetter{
		ID: id, AdminID: f.admin(), Reason: f.reason(t),
	})
	if errs.CodeOf(err) != errs.CodeNotFound {
		t.Fatalf("redrive = %v, want not_found", err)
	}
	if row := f.row(t, id); row.Status != "open" || row.RedrivenAt != nil || row.ResolvedBy != nil {
		t.Fatalf("row = %+v, want it untouched", row)
	}
}

func TestRedriveDeadLetter_failsWhenTheRowCannotBeRead(t *testing.T) {
	t.Parallel()
	f := newLetterFixture(t)
	if _, err := f.pool.Exec(t.Context(), `ALTER TABLE dead_letters RENAME TO dead_letters_gone`); err != nil {
		t.Fatal(err)
	}
	err := f.redriveHandler(f.bus.Conn).Handle(t.Context(), app.RedriveDeadLetter{
		ID: f.ids.NewV7(), AdminID: f.admin(), Reason: f.reason(t),
	})
	if errs.CodeOf(err) != errs.CodeDBUnavailable {
		t.Fatalf("redrive = %v, want db_unavailable", err)
	}
}

func TestDiscardDeadLetter_marksTheRowDiscardedWithItsReasonAndTheTimeOfTheDiscard(t *testing.T) {
	t.Parallel()
	f := newLetterFixture(t)
	id, _ := f.openRow(t)
	by := f.admin()
	f.clock.Advance(time.Minute)

	err := app.NewDiscardDeadLetterHandler(f.uow, f.clock).Handle(t.Context(),
		app.DiscardDeadLetter{ID: id, AdminID: by, Reason: f.reason(t)})
	if err != nil {
		t.Fatal(err)
	}
	row := f.row(t, id)
	f.assertMarked(t, row, "discarded", by)
	if row.ResolvedAt == nil || !row.ResolvedAt.Equal(f.clock.Now()) || row.RedrivenAt != nil {
		t.Fatalf("row = %+v, want resolved_at now and no redriven_at", row)
	}
}

func TestDiscardDeadLetter_refusesAClosedOrMissingLetterAndFailsWhenTheTableIsGone(t *testing.T) {
	t.Parallel()
	f := newLetterFixture(t)
	id, _ := f.openRow(t)
	h := app.NewDiscardDeadLetterHandler(f.uow, f.clock)
	cmd := func(id uuid.UUID) app.DiscardDeadLetter {
		return app.DiscardDeadLetter{ID: id, AdminID: f.admin(), Reason: f.reason(t)}
	}
	for _, tc := range []struct {
		status string
		id     uuid.UUID
		want   errs.Code
	}{
		{"discarded", id, errs.CodeDeadLetterNotOpen},
		{"resolved", id, errs.CodeDeadLetterNotOpen},
		{"open", f.ids.NewV7(), errs.CodeNotFound},
	} {
		f.setStatus(t.Context(), t, id, tc.status)
		if err := h.Handle(t.Context(), cmd(tc.id)); errs.CodeOf(err) != tc.want {
			t.Fatalf("discard of a %s row %s = %v, want %s", tc.status, tc.id, err, tc.want)
		}
	}
	if _, err := f.pool.Exec(t.Context(), `ALTER TABLE dead_letters RENAME TO dead_letters_gone`); err != nil {
		t.Fatal(err)
	}
	if err := h.Handle(t.Context(), cmd(id)); err == nil {
		t.Fatal("discard succeeded with the table gone")
	}
}

type termedOnce struct {
	f     *letterFixture
	t     *testing.T
	phase atomic.Int32
}

func (h *termedOnce) startConsumer(failUntil int32) {
	handler := bus.Handle("analytics.test", func(context.Context, db.Tx, events.SystemPinged, time.Time) error {
		if h.phase.Load() < failUntil {
			return errs.New(errs.CodeInvalidInput, "posthog.Capture")
		}
		return nil
	})
	reg, err := bus.NewRegistry(h.f.bus.Conn, h.f.uow, h.f.clock,
		[]bus.Consumer{{Durable: "analytics", Handlers: []bus.HandlerSpec{handler}}},
		bus.WithAckWait(testkit.DefaultAckWait))
	if err != nil {
		h.t.Fatal(err)
	}
	stop, err := reg.Start(context.WithoutCancel(h.t.Context()))
	if err != nil {
		h.t.Fatal(err)
	}
	h.t.Cleanup(stop)
}

func (h *termedOnce) publishPing() {
	h.t.Helper()
	ev := events.SystemPinged{V: 1, PingID: h.f.ids.NewV7(), Note: "hi"}
	err := h.f.uow.Do(h.f.ctx(h.t), func(ctx context.Context, tx db.Tx) error { return tx.Events.Append(ctx, ev) })
	if err != nil {
		h.t.Fatal(err)
	}
	var id uuid.UUID
	var payload []byte
	if err := h.f.pool.QueryRow(h.t.Context(), `SELECT id, payload FROM events WHERE aggregate_id = $1`, ev.PingID).
		Scan(&id, &payload); err != nil {
		h.t.Fatal(err)
	}
	if err := h.f.bus.Conn.Publish(
		h.t.Context(),
		events.TypeSystemPinged.Subject(),
		payload,
		ids.EventIDFrom(id),
	); err != nil {
		h.t.Fatal(err)
	}
}

func (f *letterFixture) awaitRow(t *testing.T, what string, ok func(deadLetterRow) bool) deadLetterRow {
	t.Helper()
	var got deadLetterRow
	testkit.Eventually(t, func() bool {
		f.tick(t)
		rows := f.rows(t)
		if len(rows) != 1 {
			return false
		}
		got = rows[0]
		return ok(got)
	}, waitLong)
	t.Logf("%s: %+v", what, got)
	return got
}

func TestDeadLetters_aRedriveThatFailsAgainReopensTheRowAndTheNextOneResolvesIt(t *testing.T) {
	t.Parallel()
	f := newLetterFixture(t)
	c := &termedOnce{f: f, t: t}
	c.startConsumer(2)
	c.publishPing()
	f.awaitRow(t, "termed", func(r deadLetterRow) bool { return r.Status == "open" && r.Occurrences == 1 })
	var id uuid.UUID
	if err := f.pool.QueryRow(t.Context(), `SELECT id FROM dead_letters`).Scan(&id); err != nil {
		t.Fatal(err)
	}
	redrive := f.redriveHandler(f.bus.Conn)
	cmd := app.RedriveDeadLetter{ID: id, AdminID: f.admin(), Reason: f.reason(t)}
	c.phase.Store(1)
	if err := redrive.Handle(t.Context(), cmd); err != nil {
		t.Fatal(err)
	}

	again := f.awaitRow(
		t,
		"failed again",
		func(r deadLetterRow) bool { return r.Status == "open" && r.Occurrences == 2 },
	)
	if again.RedrivenAt == nil || again.ResolvedBy == nil {
		t.Fatalf("row = %+v, want redriven_at and the redriving admin kept on the reopened row", again)
	}
	c.phase.Store(2)
	if err := redrive.Handle(t.Context(), cmd); err != nil {
		t.Fatal(err)
	}
	done := f.awaitRow(t, "resolved", func(r deadLetterRow) bool { return r.Status == "resolved" })
	if done.Occurrences != 2 || done.ResolvedAt == nil || done.ResolvedBy == nil {
		t.Fatalf("row = %+v, want it resolved after the third attempt with 2 occurrences", done)
	}
}

func TestDeadLetters_CLIRetryResolvesRow(t *testing.T) {
	t.Parallel()
	f := newLetterFixture(t)
	c := &termedOnce{f: f, t: t}
	c.startConsumer(1)
	c.publishPing()
	f.awaitRow(t, "termed", func(r deadLetterRow) bool { return r.Status == "open" })
	letters, err := f.bus.Conn.DeadLetters(t.Context())
	if err != nil || len(letters) != 1 {
		t.Fatalf("DeadLetters = %+v, %v, want the one termed letter", letters, err)
	}

	c.phase.Store(1)
	if err := f.bus.Conn.Redeliver(t.Context(), letters[0], "retry-1-1"); err != nil {
		t.Fatal(err)
	}
	done := f.awaitRow(t, "resolved", func(r deadLetterRow) bool { return r.Status == "resolved" })
	if done.ResolvedAt == nil || done.ResolvedBy != nil {
		t.Fatalf("row = %+v, want resolved by the marker alone, with no admin recorded", done)
	}
}

func TestDeadLetterCommands_failWhenTheWriteFailsAfterTheRowWasRead(t *testing.T) {
	t.Parallel()
	f := newLetterFixture(t)
	id, _ := f.openRow(t)
	if _, err := f.pool.Exec(t.Context(), `ALTER TABLE dead_letters DROP COLUMN resolve_reason`); err != nil {
		t.Fatal(err)
	}
	redrive := f.redriveHandler(f.bus.Conn).Handle(t.Context(),
		app.RedriveDeadLetter{ID: id, AdminID: f.admin(), Reason: f.reason(t)})
	discard := app.NewDiscardDeadLetterHandler(f.uow, f.clock).Handle(t.Context(),
		app.DiscardDeadLetter{ID: id, AdminID: f.admin(), Reason: f.reason(t)})
	if redrive == nil || discard == nil {
		t.Fatalf("redrive = %v, discard = %v, want both to fail on the missing column", redrive, discard)
	}
}
