package notify_test

import (
	"context"
	"errors"
	"reflect"
	"strconv"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/cabal"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity"
	"github.com/monaco/monaco/apps/backend/internal/modules/notify/app"
	"github.com/monaco/monaco/apps/backend/internal/platform/apns"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/testkit/fakes"
	"github.com/monaco/monaco/apps/backend/internal/testkit/marketfake"
)

type presentedKind struct {
	renders  *int
	data     map[string]string
	presents func(n int) app.Presentation
}

func (presentedKind) Name() string { return "test" }

func (presentedKind) Recipients(_ context.Context, e events.NotifyTestRequested) ([]ids.UserID, error) {
	return []ids.UserID{ids.UserIDFrom(e.UserID)}, nil
}

func (k presentedKind) Render(_ context.Context, _ events.NotifyTestRequested, _ ids.UserID) (app.Message, error) {
	*k.renders++
	data := map[string]string{"kind": "test"}
	for key, value := range k.data {
		data[key] = value
	}
	return app.Message{
		Title: "t", Body: "b", Data: data, CollapseID: "c", Presentation: k.presents(*k.renders),
	}, nil
}

func TestPresentation_aKindMayNotSetAReservedKey(t *testing.T) {
	t.Parallel()
	for _, key := range []string{
		"sender_kind", "sender_id", "sender_name", "sender_image_url", "group_id", "group_name", "group_image_url",
	} {
		t.Run(key, func(t *testing.T) {
			t.Parallel()
			r := newPushRig(t)
			user := r.user(t, "active")
			r.device(t, user, token('a'))
			d, e := r.trigger(t, opsActor, user)
			kind := presentedKind{
				renders: new(int), data: map[string]string{key: "spoofed"},
				presents: func(int) app.Presentation { return app.ForAccount() },
			}

			err := handleKinds(t, r, r.sender, d, e, kind)

			if errs.CodeOf(err) != errs.CodeInvalidInput {
				t.Fatalf("Handle = %v, want %s", err, errs.CodeInvalidInput)
			}
			if sent := r.sender.Sent(); len(sent) != 0 {
				t.Fatalf("sent %+v, want nothing", sent)
			}
			r.wantCount(t, "rows written", 0, `SELECT count(*) FROM notifications WHERE source_event_id = $1`,
				d.EventID.UUID())
		})
	}
}

func TestPresentation_aCabalPushCarriesBothPicturesAndMutableContent(t *testing.T) {
	t.Parallel()
	r := newPushRig(t)
	user, proposer := r.user(t, "active"), r.user(t, "active")
	r.device(t, user, token('a'))
	d, e := r.trigger(t, opsActor, user)
	view := cabal.View{ID: ids.CabalIDFrom(r.ids.NewV7()), Name: "AC Capital", PictureURL: "https://img/ac.png"}
	sender := app.PersonSender(identity.UserCard{ID: proposer, DisplayName: "Mia", PhotoURL: "https://img/mia.png"})
	kind := presentedKind{
		renders: new(int),
		presents: func(int) app.Presentation {
			p := app.ForCabal(view, sender)
			p.Category, p.InterruptionLevel = "VOTE", app.InterruptionTimeSensitive
			return p
		},
	}

	if err := handleKinds(t, r, r.sender, d, e, kind); err != nil {
		t.Fatal(err)
	}

	want := apns.Push{
		UserID: user, Token: token('a'), Environment: apns.Sandbox, CollapseID: "c", Title: "t", Body: "b",
		Data: map[string]string{
			"kind": "test", "sender_kind": "person", "sender_id": proposer.String(), "sender_name": "Mia",
			"sender_image_url": "https://img/mia.png", "group_id": view.ID.String(), "group_name": "AC Capital",
			"group_image_url": "https://img/ac.png",
		},
		ThreadID: "cabal-" + view.ID.String(), Category: "VOTE", MutableContent: true,
		InterruptionLevel: "time-sensitive", Sound: "default",
	}
	if sent := r.sender.Sent(); !reflect.DeepEqual(sent, []apns.Push{want}) {
		t.Fatalf("sent %+v, want only %+v", sent, want)
	}
}

func TestPresentation_aMessageWithoutOneSendsAsBeforeFromNullColumns(t *testing.T) {
	t.Parallel()
	r := newPushRig(t)
	user := r.user(t, "active")
	r.device(t, user, token('a'))
	d, e := r.trigger(t, opsActor, user)
	kind := presentedKind{renders: new(int), presents: func(int) app.Presentation { return app.Presentation{} }}

	if err := handleKinds(t, r, r.sender, d, e, kind); err != nil {
		t.Fatal(err)
	}

	want := apns.Push{
		UserID: user, Token: token('a'), Environment: apns.Sandbox, CollapseID: "c", Title: "t", Body: "b",
		Data: map[string]string{"kind": "test"},
	}
	if sent := r.sender.Sent(); !reflect.DeepEqual(sent, []apns.Push{want}) {
		t.Fatalf("sent %+v, want only %+v", sent, want)
	}
	r.wantCount(t, "rows with a presentation", 0, `SELECT count(*) FROM notifications
		WHERE source_event_id = $1 AND (thread_id IS NOT NULL OR category IS NOT NULL
			OR interruption_level IS NOT NULL OR presentation IS NOT NULL)`, d.EventID.UUID())
}

func TestPresentation_aResendAfterAFailedSendReadsTheRowNotTheRender(t *testing.T) {
	t.Parallel()
	r := newPushRig(t)
	user := r.user(t, "active")
	r.device(t, user, token('a'))
	d, e := r.trigger(t, opsActor, user)
	view := cabal.View{ID: ids.CabalIDFrom(r.ids.NewV7()), Name: "AC Capital"}
	kind := presentedKind{
		renders: new(int),
		presents: func(n int) app.Presentation {
			name := "render-" + strconv.Itoa(n)
			p := app.ForCabal(view, app.Sender{Kind: app.SenderCabal, ID: view.ID.String(), Name: name})
			p.Category = "VOTE"
			return p
		},
	}
	r.sender.FailOnce("Send", errs.New(errs.CodeAPNSUnavailable, "test.apns"))

	if err := handleKinds(t, r, r.sender, d, e, kind); err == nil {
		t.Fatal("first Handle succeeded, want the send failure")
	}
	if err := handleKinds(t, r, r.sender, d, e, kind); err != nil {
		t.Fatalf("second Handle = %v", err)
	}

	sent := r.sender.Sent()
	if len(sent) != 2 {
		t.Fatalf("sent %d pushes, want the failed attempt and the resend", len(sent))
	}
	if sent[1].ThreadID != "cabal-"+view.ID.String() || sent[1].Category != "VOTE" ||
		sent[1].Data["sender_name"] != "render-1" || !reflect.DeepEqual(sent[0], sent[1]) {
		t.Fatalf("resend %+v, want the first render's presentation as in %+v", sent[1], sent[0])
	}
}

func TestPresentation_theCabalKindsFailWhenTheCabalCannotBeRead(t *testing.T) {
	t.Parallel()
	down := errs.New(errs.CodeDBUnavailable, "test.cabal")
	cabals := fakes.NewCabal(nil, nil)
	cabals.Fail("Cabal", down)
	users := fakes.NewIdentity(nil, nil)
	assets := marketfake.NewCatalog(marketfake.Fixtures()...)
	cases := copyCases(cabals, users, assets, ids.UserID{})
	for _, name := range []string{
		"proposal_result_filled", "proposal_result_fill_failed", "proposal_result_fill_blocked",
		"proposal_result_failed", "proposal_result_expired",
	} {
		c := cases[name]

		_, err := c.render(t.Context(), goldenEvent(t, c.typ), ids.UserID{})

		if !errors.Is(err, down) {
			t.Errorf("%s: Render = %v, want %v", name, err, down)
		}
	}
}

func TestPresentation_aRowWhosePresentationCannotBeReadFailsTheResend(t *testing.T) {
	t.Parallel()
	r := newPushRig(t)
	user := r.user(t, "active")
	r.device(t, user, token('a'))
	d, e := r.trigger(t, opsActor, user)
	kind := presentedKind{renders: new(int), presents: func(int) app.Presentation { return app.ForAccount() }}
	r.sender.FailOnce("Send", errs.New(errs.CodeAPNSUnavailable, "test.apns"))
	if err := handleKinds(t, r, r.sender, d, e, kind); err == nil {
		t.Fatal("first Handle succeeded, want the send failure")
	}
	r.exec(t, `UPDATE notifications SET presentation = '[]'::jsonb WHERE source_event_id = $1`, d.EventID.UUID())

	err := handleKinds(t, r, r.sender, d, e, kind)

	if errs.CodeOf(err) != errs.CodeDecodeFailed {
		t.Fatalf("Handle = %v, want %s", err, errs.CodeDecodeFailed)
	}
}
