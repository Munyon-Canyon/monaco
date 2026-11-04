package social_test

import (
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

type chatPinner struct {
	chatFixture
	pin   *app.PinChatMessageHandler
	unpin *app.UnpinChatMessageHandler
}

func newChatPinner(t *testing.T) chatPinner {
	t.Helper()
	f := newChatFixture(t)
	return chatPinner{
		chatFixture: f,
		pin:         app.NewPinChatMessageHandler(f.deps),
		unpin:       app.NewUnpinChatMessageHandler(f.deps),
	}
}

func (f chatPinner) pinAs(t *testing.T, user ids.UserID, id uuid.UUID) (app.ChatMessage, error) {
	t.Helper()
	return f.pin.Handle(f.as(t, user), app.PinChatMessage{CabalID: f.cabal.ID, MessageID: id, Caller: user})
}

func (f chatPinner) unpinAs(t *testing.T, user ids.UserID, id uuid.UUID) (app.ChatMessage, error) {
	t.Helper()
	return f.unpin.Handle(f.as(t, user), app.UnpinChatMessage{CabalID: f.cabal.ID, MessageID: id, Caller: user})
}

func (f chatPinner) mustPin(t *testing.T, user ids.UserID, id uuid.UUID) app.ChatMessage {
	t.Helper()
	m, err := f.pinAs(t, user, id)
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func (f chatPinner) pins(t *testing.T) int {
	t.Helper()
	return f.count(t, `SELECT count(*) FROM cabal_messages WHERE pinned_at IS NOT NULL`)
}

func (f chatPinner) pinEvents(t *testing.T) int {
	t.Helper()
	return f.count(t, `SELECT count(*) FROM events WHERE type IN ($1, $2)`,
		string(events.TypeChatMessagePinned), string(events.TypeChatMessageUnpinned))
}

func (f chatPinner) systemRow(t *testing.T) uuid.UUID {
	t.Helper()
	id := f.deps.IDs.NewV7()
	if _, err := f.pool.Exec(t.Context(), `INSERT INTO cabal_messages (id, cabal_id, kind, moment, created_at)
		VALUES ($1, $2, 'system', '{"kind":"cabal_created"}', $3)`, id, f.cabal.ID.UUID(), f.now); err != nil {
		t.Fatal(err)
	}
	return id
}

func TestPinChatMessage_anAdminPinsAnyMessageAndAnAuthorPinsTheirOwn(t *testing.T) {
	t.Parallel()
	f := newChatPinner(t)
	theirs := f.mustSend(t, f.member(1), "theirs", nil)
	mine := f.mustSend(t, f.member(2), "mine", nil)
	f.clock.Advance(time.Minute)
	byAdmin := f.mustPin(t, f.member(0), theirs.ID)
	byAuthor := f.mustPin(t, f.member(2), mine.ID)
	if byAdmin.PinnedAt == nil || byAdmin.PinnedBy != f.member(0) || byAuthor.PinnedBy != f.member(2) ||
		!byAdmin.PinnedAt.Equal(f.clock.Now().UTC()) {
		t.Fatalf("pinned = %+v and %+v, want each stamped with its pinner at %s", byAdmin, byAuthor, f.clock.Now())
	}
	if f.pins(t) != 2 || f.pinEvents(t) != 2 {
		t.Fatalf("%d pins and %d events, want 2 and 2", f.pins(t), f.pinEvents(t))
	}
}

func TestPinChatMessage_appendsAnActorEventAndPublishesPinUpdated(t *testing.T) {
	t.Parallel()
	f := newChatPinner(t)
	m := f.mustSend(t, f.member(1), "rules", nil)
	before := len(f.rt.Published())
	f.mustPin(t, f.member(0), m.ID)
	var payload string
	if err := f.pool.QueryRow(t.Context(), `SELECT payload::text FROM events WHERE type = $1`,
		string(events.TypeChatMessagePinned)).Scan(&payload); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(payload, m.ID.String()) || !strings.Contains(payload, f.member(0).String()) {
		t.Fatalf("event payload = %s, want the message and the actor", payload)
	}
	published := f.rt.Published()
	want := testkit.RealtimePublish{
		Channel: f.channel(), Name: app.EventPinUpdated,
		Data: jsonOf(t, map[string]any{"message_id": m.ID, "pinned": true}),
	}
	if len(published) != before+1 || !reflect.DeepEqual(published[len(published)-1], want) {
		t.Fatalf("published = %+v, want %+v last", published, want)
	}
}

func TestPinChatMessage_refusesAnyoneElseAndOnlyAnAdminPinsASystemRow(t *testing.T) {
	t.Parallel()
	f := newChatPinner(t)
	m := f.mustSend(t, f.member(1), "theirs", nil)
	system := f.systemRow(t)
	_, err := f.pinAs(t, f.member(2), m.ID)
	wantCode(t, err, errs.CodeChatPinForbidden)
	_, err = f.pinAs(t, f.member(2), system)
	wantCode(t, err, errs.CodeChatPinForbidden)
	if f.pins(t) != 0 || f.pinEvents(t) != 0 {
		t.Fatalf("%d pins and %d events after refusals, want none", f.pins(t), f.pinEvents(t))
	}
	if got := f.mustPin(t, f.member(0), system); got.PinnedAt == nil {
		t.Fatalf("system row = %+v, want it pinned by an admin", got)
	}
}

func TestPinChatMessage_refusesWhatCannotBePinned(t *testing.T) {
	t.Parallel()
	f := newChatPinner(t)
	top := f.mustSend(t, f.member(0), "top", nil)
	reply := f.mustSend(t, f.member(1), "reply", &domain.Reply{Parent: top.ID})
	gone := f.mustSend(t, f.member(0), "gone", nil)
	if err := f.del.Handle(f.as(t, f.member(0)), app.DeleteChatMessage{
		CabalID: f.cabal.ID, MessageID: gone.ID, Caller: f.member(0),
	}); err != nil {
		t.Fatal(err)
	}
	elsewhere, err := f.post.Handle(f.as(t, f.other.Creator.ID), app.PostChatMessage{
		CabalID: f.other.ID, Author: f.other.Creator.ID, Input: chatText("elsewhere"),
	})
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name string
		user ids.UserID
		id   uuid.UUID
		want errs.Code
	}{
		{"a reply", f.member(0), reply.ID, errs.CodeInvalidInput},
		{"a deleted message", f.member(0), gone.ID, errs.CodeChatMessageNotFound},
		{"another cabal's message", f.member(0), elsewhere.ID, errs.CodeChatMessageNotFound},
		{"an unknown message", f.member(0), f.deps.IDs.NewV7(), errs.CodeChatMessageNotFound},
		{"an outsider", f.outsider, top.ID, errs.CodeNotCabalMember},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, err := f.pinAs(t, tt.user, tt.id)
			wantCode(t, err, tt.want)
		})
	}
	if f.pins(t) != 0 || f.pinEvents(t) != 0 {
		t.Fatalf("%d pins and %d events after refusals, want none", f.pins(t), f.pinEvents(t))
	}
}

func TestPinChatMessage_aReplyIsRefusedWithTheReplyReason(t *testing.T) {
	t.Parallel()
	f := newChatPinner(t)
	top := f.mustSend(t, f.member(0), "top", nil)
	reply := f.mustSend(t, f.member(1), "reply", &domain.Reply{Parent: top.ID})
	_, err := f.pinAs(t, f.member(1), reply.ID)
	var reason string
	for _, attr := range errs.Detail(err) {
		if attr.Key == "reason" {
			reason = attr.Value.String()
		}
	}
	if errs.CodeOf(err) != errs.CodeInvalidInput || reason != domain.ReasonReply {
		t.Fatalf("err = %v, reason %q, want invalid_input with reason %s", err, reason, domain.ReasonReply)
	}
}

func TestPinChatMessage_aFourthPinIsRefusedAndAnUnpinMakesRoom(t *testing.T) {
	t.Parallel()
	f := newChatPinner(t)
	bodies := []string{"one", "two", "three", "four"}
	posted := make([]app.ChatMessage, 0, len(bodies))
	for _, body := range bodies {
		posted = append(posted, f.mustSend(t, f.member(0), body, nil))
	}
	for _, m := range posted[:3] {
		f.clock.Advance(time.Second)
		f.mustPin(t, f.member(0), m.ID)
	}
	_, err := f.pinAs(t, f.member(0), posted[3].ID)
	wantCode(t, err, errs.CodeChatPinLimit)
	if f.pins(t) != 3 {
		t.Fatalf("%d pins, want 3", f.pins(t))
	}
	if _, err := f.unpinAs(t, f.member(0), posted[0].ID); err != nil {
		t.Fatal(err)
	}
	f.mustPin(t, f.member(0), posted[3].ID)
	if f.pins(t) != 3 {
		t.Fatalf("%d pins after swapping one, want 3", f.pins(t))
	}
}

func TestPinChatMessage_concurrentPinsNeverPassTheLimit(t *testing.T) {
	t.Parallel()
	f := newChatPinner(t)
	const n = 6
	posted := make([]app.ChatMessage, n)
	for i := range posted {
		posted[i] = f.mustSend(t, f.member(0), "m", nil)
	}
	var wg sync.WaitGroup
	errsOut := make(chan error, n)
	for _, m := range posted {
		wg.Go(func() {
			_, err := f.pinAs(t, f.member(0), m.ID)
			errsOut <- err
		})
	}
	wg.Wait()
	close(errsOut)
	refused := 0
	for err := range errsOut {
		if err != nil {
			wantCode(t, err, errs.CodeChatPinLimit)
			refused++
		}
	}
	if f.pins(t) != app.MaxChatPins || refused != n-app.MaxChatPins {
		t.Fatalf("%d pins and %d refusals, want %d and %d", f.pins(t), refused, app.MaxChatPins, n-app.MaxChatPins)
	}
}

func TestPinChatMessage_pinningAPinnedMessageChangesNothing(t *testing.T) {
	t.Parallel()
	f := newChatPinner(t)
	m := f.mustSend(t, f.member(1), "rules", nil)
	first := f.mustPin(t, f.member(0), m.ID)
	published := len(f.rt.Published())
	f.clock.Advance(time.Hour)
	again := f.mustPin(t, f.member(1), m.ID)
	if !again.PinnedAt.Equal(*first.PinnedAt) || again.PinnedBy != f.member(0) ||
		f.pinEvents(t) != 1 || len(f.rt.Published()) != published {
		t.Fatalf("again = %+v after %+v, %d events, %d publishes, want nothing changed",
			again, first, f.pinEvents(t), len(f.rt.Published())-published)
	}
}

func TestUnpinChatMessage_anAdminOrThePinnerUnpinsAndNobodyElseDoes(t *testing.T) {
	t.Parallel()
	f := newChatPinner(t)
	mine := f.mustSend(t, f.member(1), "mine", nil)
	rules := f.mustSend(t, f.member(0), "rules", nil)
	f.mustPin(t, f.member(1), mine.ID)
	f.mustPin(t, f.member(0), rules.ID)
	_, err := f.unpinAs(t, f.member(1), rules.ID)
	wantCode(t, err, errs.CodeChatPinForbidden)
	_, err = f.unpinAs(t, f.member(2), mine.ID)
	wantCode(t, err, errs.CodeChatPinForbidden)
	byPinner, err := f.unpinAs(t, f.member(1), mine.ID)
	if err != nil || byPinner.PinnedAt != nil {
		t.Fatalf("unpin by the pinner = %+v, %v, want it unpinned", byPinner, err)
	}
	byAdmin, err := f.unpinAs(t, f.member(0), rules.ID)
	if err != nil || byAdmin.PinnedAt != nil || f.pins(t) != 0 {
		t.Fatalf("unpin by an admin = %+v, %v, %d pins left", byAdmin, err, f.pins(t))
	}
}

func TestUnpinChatMessage_publishesOnceAndAnUnpinnedMessageChangesNothing(t *testing.T) {
	t.Parallel()
	f := newChatPinner(t)
	m := f.mustSend(t, f.member(1), "rules", nil)
	f.mustPin(t, f.member(1), m.ID)
	if _, err := f.unpinAs(t, f.member(1), m.ID); err != nil {
		t.Fatal(err)
	}
	published, appended := f.rt.Published(), f.pinEvents(t)
	if _, err := f.unpinAs(t, f.member(2), m.ID); err != nil {
		t.Fatal(err)
	}
	want := testkit.RealtimePublish{
		Channel: f.channel(), Name: app.EventPinUpdated,
		Data: jsonOf(t, map[string]any{"message_id": m.ID, "pinned": false}),
	}
	if !reflect.DeepEqual(f.rt.Published(), published) || f.pinEvents(t) != appended ||
		!reflect.DeepEqual(published[len(published)-1], want) {
		t.Fatalf("published = %+v, %d events, want the unpin published once and the repeat silent", published, appended)
	}
}

func TestUnpinChatMessage_refusesWhatItCannotFind(t *testing.T) {
	t.Parallel()
	f := newChatPinner(t)
	m := f.mustSend(t, f.member(0), "rules", nil)
	_, err := f.unpinAs(t, f.member(0), f.deps.IDs.NewV7())
	wantCode(t, err, errs.CodeChatMessageNotFound)
	_, err = f.unpinAs(t, f.outsider, m.ID)
	wantCode(t, err, errs.CodeNotCabalMember)
}

func TestDeleteChatMessage_clearsThePin(t *testing.T) {
	t.Parallel()
	f := newChatPinner(t)
	m := f.mustSend(t, f.member(1), "rules", nil)
	f.mustPin(t, f.member(1), m.ID)
	if err := f.del.Handle(f.as(t, f.member(1)), app.DeleteChatMessage{
		CabalID: f.cabal.ID, MessageID: m.ID, Caller: f.member(1),
	}); err != nil {
		t.Fatal(err)
	}
	if f.pins(t) != 0 {
		t.Fatalf("%d pins after deleting the pinned message, want 0", f.pins(t))
	}
	pins, err := app.ListChatPins(t.Context(), f.pool, f.deps.Members, f.cabal.ID, f.member(0))
	if err != nil || len(pins) != 0 {
		t.Fatalf("pins = %+v, %v, want none", pins, err)
	}
}

func TestPinChatMessage_aFailedPublishStillPinsAndLogsIt(t *testing.T) {
	t.Parallel()
	f := newChatPinner(t)
	m := f.mustSend(t, f.member(1), "rules", nil)
	f.rt.Fail(errs.New(errs.CodeUpstreamUnavailable, "test.ably"))
	ctx, logs := f.loggedCtx(t, f.member(0))
	if _, err := f.pin.Handle(ctx, app.PinChatMessage{
		CabalID: f.cabal.ID, MessageID: m.ID, Caller: f.member(0),
	}); err != nil {
		t.Fatal(err)
	}
	out := string(logs.Bytes())
	if f.pins(t) != 1 || strings.Count(out, publishFailedLog) != 1 || !strings.Contains(out, app.EventPinUpdated) {
		t.Fatalf("%d pins, logs = %s, want one pin and one %s for %s",
			f.pins(t), out, publishFailedLog, app.EventPinUpdated)
	}
}

func TestListChatPins_returnsTheNewestThreeForMembersOnly(t *testing.T) {
	t.Parallel()
	f := newChatPinner(t)
	bodies := []string{"a", "b", "c"}
	pinned := make([]app.ChatMessage, 0, len(bodies))
	for _, body := range bodies {
		m := f.mustSend(t, f.member(0), body, nil)
		f.clock.Advance(time.Second)
		pinned = append(pinned, f.mustPin(t, f.member(0), m.ID))
	}
	unpinned := f.mustSend(t, f.member(0), "d", nil)
	got, err := app.ListChatPins(t.Context(), f.pool, f.deps.Members, f.cabal.ID, f.member(2))
	if err != nil || len(got) != 3 || got[0].ID != pinned[2].ID || got[1].ID != pinned[1].ID ||
		got[2].ID != pinned[0].ID || got[0].PinnedAt == nil || got[0].PinnedBy != f.member(0) {
		t.Fatalf("pins = %+v, %v, want c, b, a newest first with their pinner", got, err)
	}
	for _, m := range got {
		if m.ID == unpinned.ID {
			t.Fatalf("pins = %+v, want the unpinned message left out", got)
		}
	}
	_, err = app.ListChatPins(t.Context(), f.pool, f.deps.Members, f.cabal.ID, f.outsider)
	wantCode(t, err, errs.CodeNotCabalMember)
}

func TestChatPins_returnTheMembershipLookupFailure(t *testing.T) {
	t.Parallel()
	f := newChatPinner(t)
	m := f.mustSend(t, f.member(0), "rules", nil)
	deps := f.deps
	deps.Members = membersListDown{deps.Members}
	for name, run := range map[string]func() error{
		"pin": func() error {
			_, err := app.NewPinChatMessageHandler(deps).Handle(f.as(t, f.member(0)), app.PinChatMessage{
				CabalID: f.cabal.ID, MessageID: m.ID, Caller: f.member(0),
			})
			return err
		},
		"unpin": func() error {
			_, err := app.NewUnpinChatMessageHandler(deps).Handle(f.as(t, f.member(0)), app.UnpinChatMessage{
				CabalID: f.cabal.ID, MessageID: m.ID, Caller: f.member(0),
			})
			return err
		},
	} {
		if err := run(); errs.CodeOf(err) != errs.CodeUpstreamUnavailable {
			t.Fatalf("%s err = %v, want upstream_unavailable", name, err)
		}
	}
}

func TestChatPins_failWithInternalWhenTheStoreFails(t *testing.T) {
	t.Parallel()
	const armRead = `CREATE FUNCTION arm_reads() RETURNS trigger LANGUAGE plpgsql AS $$
		BEGIN PERFORM set_config('chat.reads', 'broken', true); RETURN NULL; END $$;
		CREATE TRIGGER arm_reads AFTER UPDATE ON cabal_messages FOR EACH ROW EXECUTE FUNCTION arm_reads();
		ALTER TABLE cabal_messages ENABLE ROW LEVEL SECURITY;
		ALTER TABLE cabal_messages FORCE ROW LEVEL SECURITY;
		CREATE POLICY open ON cabal_messages USING (true) WITH CHECK (true);
		CREATE POLICY broken_reads ON cabal_messages AS RESTRICTIVE FOR SELECT
		  USING (1 / (CASE WHEN current_setting('chat.reads', true) = 'broken' THEN 0 ELSE 1 END) = 1)`
	const pinsUnreadable = `ALTER TABLE cabal_messages ENABLE ROW LEVEL SECURITY;
		ALTER TABLE cabal_messages FORCE ROW LEVEL SECURITY;
		CREATE POLICY open ON cabal_messages USING (true) WITH CHECK (true);
		CREATE POLICY no_pins ON cabal_messages AS RESTRICTIVE FOR SELECT
		  USING (1 / (CASE WHEN pinned_at IS NOT NULL THEN 0 ELSE 1 END) = 1)`
	tests := []struct {
		name    string
		prepare func(t *testing.T, f chatPinner, id uuid.UUID)
		breaker string
		run     func(t *testing.T, f chatPinner, id uuid.UUID) error
	}{
		{"pin when the lock fails", nil, `DROP FUNCTION lock_chat_pins`, pinRun},
		{"pin when the lookup fails", nil, `DROP TABLE cabal_messages CASCADE`, pinRun},
		{"pin when the count fails", pinAnother, pinsUnreadable, pinRun},
		{
			"pin when the update fails", nil,
			`ALTER TABLE cabal_messages ADD CONSTRAINT no_pins CHECK (pinned_at IS NULL) NOT VALID`, pinRun,
		},
		{"pin when the event cannot be appended", nil, `DROP TABLE events CASCADE`, pinRun},
		{"pin when the message cannot be read back", nil, armRead, pinRun},
		{"unpin when the lookup fails", nil, `DROP TABLE cabal_messages CASCADE`, unpinRun},
		{
			"unpin when the update fails", pinTarget,
			`ALTER TABLE cabal_messages ADD CONSTRAINT keep_pins CHECK (pinned_at IS NOT NULL) NOT VALID`, unpinRun,
		},
		{"unpin when the event cannot be appended", pinTarget, `DROP TABLE events CASCADE`, unpinRun},
		{"unpin when the message cannot be read back", pinTarget, armRead, unpinRun},
		{
			"list when the read fails", nil, `DROP TABLE cabal_messages CASCADE`,
			func(t *testing.T, f chatPinner, _ uuid.UUID) error {
				t.Helper()
				_, err := app.ListChatPins(t.Context(), f.pool, f.deps.Members, f.cabal.ID, f.member(0))
				return err
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			f := newChatPinner(t)
			m := f.mustSend(t, f.member(0), "rules", nil)
			if tt.prepare != nil {
				tt.prepare(t, f, m.ID)
			}
			if _, err := f.pool.Exec(t.Context(), tt.breaker); err != nil {
				t.Fatal(err)
			}
			wantCode(t, tt.run(t, f, m.ID), errs.CodeInternal)
		})
	}
}

func pinRun(t *testing.T, f chatPinner, id uuid.UUID) error {
	t.Helper()
	_, err := f.pinAs(t, f.member(0), id)
	return err
}

func unpinRun(t *testing.T, f chatPinner, id uuid.UUID) error {
	t.Helper()
	_, err := f.unpinAs(t, f.member(0), id)
	return err
}

func pinTarget(t *testing.T, f chatPinner, id uuid.UUID) {
	t.Helper()
	f.mustPin(t, f.member(0), id)
}

func pinAnother(t *testing.T, f chatPinner, _ uuid.UUID) {
	t.Helper()
	f.mustPin(t, f.member(0), f.mustSend(t, f.member(0), "other", nil).ID)
}
