package notify_test

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/cabal"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity"
	"github.com/monaco/monaco/apps/backend/internal/modules/notify/app"
	"github.com/monaco/monaco/apps/backend/internal/platform/apns"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/testkit/fakes"
)

type accessWorld struct {
	cabalID            ids.CabalID
	creator, requester ids.UserID
	cabals             *fakes.Cabal
}

func (r *pushRig) accessWorld(t *testing.T) accessWorld {
	t.Helper()
	w := accessWorld{
		cabalID: ids.CabalIDFrom(r.ids.NewV7()),
		creator: r.follower(t, "ada", "Ada"), requester: r.follower(t, "bea", "Bea"),
	}
	r.device(t, w.creator, token('a'))
	r.device(t, w.requester, token('b'))
	view := cabal.View{ID: w.cabalID, Name: cabalName, CreatorID: w.creator}
	w.cabals = fakes.NewCabal([]fakes.CabalSeed{{View: view}}, nil)
	return w
}

func (w accessWorld) requested(r *pushRig, direction string) events.CabalAccessRequested {
	return events.CabalAccessRequested{
		V: 1, RequestID: r.ids.NewV7(), CabalID: w.cabalID.UUID(), UserID: w.requester.UUID(),
		Direction: direction, ActorID: w.requester.UUID(),
	}
}

func (w accessWorld) decided(r *pushRig, direction, decision string) events.CabalAccessDecided {
	return events.CabalAccessDecided{
		V: 1, RequestID: r.ids.NewV7(), CabalID: w.cabalID.UUID(), UserID: w.requester.UUID(),
		Direction: direction, Decision: decision, ActorID: w.creator.UUID(),
	}
}

func TestNotify_CabalAccessRequested_PushesTheCreator(t *testing.T) {
	t.Parallel()
	r := newPushRig(t)
	w := r.accessWorld(t)
	e := w.requested(r, "request")
	d := r.emit(t, userActor(w.requester), e)

	err := handleKinds(t, r, r.sender, d, e, app.CabalAccessRequested{Cabals: w.cabals, Users: r.users})

	wantVerdict(t, err, "", errs.VerdictAck)
	want := apns.Push{
		UserID: w.creator, Token: token('a'), Environment: apns.Sandbox, CollapseID: "access-" + e.RequestID.String(),
		Title: "New request to join", Body: "Bea wants to join " + cabalName,
		Data: map[string]string{"kind": "cabal_access_requested", "cabal_id": w.cabalID.String()},
	}
	if sent := r.sender.Sent(); !reflect.DeepEqual(sent, []apns.Push{want}) {
		t.Fatalf("sent %+v, want only %+v", sent, want)
	}
	r.wantStates(t, d, map[ids.UserID]string{w.creator: "delivered"})
	r.wantRecorded(t, d, 1)
}

func TestNotify_CabalAccessRequested_InviteNotifiesNobody(t *testing.T) {
	t.Parallel()
	r := newPushRig(t)
	w := r.accessWorld(t)
	e := w.requested(r, "invite")
	d := r.emit(t, userActor(w.creator), e)

	err := handleKinds(t, r, r.sender, d, e, app.CabalAccessRequested{Cabals: w.cabals, Users: r.users})

	wantVerdict(t, err, "", errs.VerdictAck)
	if sent := r.sender.Sent(); len(sent) != 0 {
		t.Fatalf("sent %+v for an invite, want nothing", sent)
	}
}

type failingUsers struct{ err error }

func (u failingUsers) UsersByID(context.Context, []ids.UserID) (map[ids.UserID]identity.UserCard, error) {
	return nil, u.err
}

func TestNotify_CabalAccessRequested_PortFailuresReturnAsIs(t *testing.T) {
	t.Parallel()
	down := errs.New(errs.CodeDBUnavailable, "test.port")
	for _, op := range []string{"Cabal", "UsersByID"} {
		t.Run(op, func(t *testing.T) {
			t.Parallel()
			r := newPushRig(t)
			w := r.accessWorld(t)
			e := w.requested(r, "request")
			d := r.emit(t, userActor(w.requester), e)
			kind := app.CabalAccessRequested{Cabals: w.cabals, Users: failingUsers{down}}
			if op == "Cabal" {
				w.cabals.Fail("Cabal", down)
			}

			err := handleKinds(t, r, r.sender, d, e, kind)

			if !errors.Is(err, down) {
				t.Fatalf("Handle = %v, want %v", err, down)
			}
			if sent := r.sender.Sent(); len(sent) != 0 {
				t.Fatalf("sent %+v after a failed %s, want nothing", sent, op)
			}
			r.wantRecorded(t, d, 0)
		})
	}
}

func TestNotify_CabalAccessApproved_PushesTheRequester(t *testing.T) {
	t.Parallel()
	r := newPushRig(t)
	w := r.accessWorld(t)
	e := w.decided(r, "request", "approved")
	d := r.emit(t, userActor(w.creator), e)

	err := handleKinds(t, r, r.sender, d, e, app.CabalAccessApproved{Cabals: w.cabals})

	wantVerdict(t, err, "", errs.VerdictAck)
	want := apns.Push{
		UserID: w.requester, Token: token('b'), Environment: apns.Sandbox, CollapseID: "access-" + e.RequestID.String(),
		Title: "You're in " + cabalName, Body: "Your request was approved. Say hi.",
		Data: map[string]string{"kind": "cabal_access_approved", "cabal_id": w.cabalID.String()},
	}
	if sent := r.sender.Sent(); !reflect.DeepEqual(sent, []apns.Push{want}) {
		t.Fatalf("sent %+v, want only %+v", sent, want)
	}
	r.wantStates(t, d, map[ids.UserID]string{w.requester: "delivered"})
}

func TestNotify_CabalAccessApproved_OtherOutcomesNotifyNobody(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ direction, decision string }{
		{"request", "denied"}, {"request", "revoked"}, {"request", "expired"}, {"invite", "approved"},
	} {
		t.Run(tc.direction+"/"+tc.decision, func(t *testing.T) {
			t.Parallel()
			r := newPushRig(t)
			w := r.accessWorld(t)
			e := w.decided(r, tc.direction, tc.decision)
			d := r.emit(t, userActor(w.creator), e)

			err := handleKinds(t, r, r.sender, d, e, app.CabalAccessApproved{Cabals: w.cabals})

			wantVerdict(t, err, "", errs.VerdictAck)
			if sent := r.sender.Sent(); len(sent) != 0 {
				t.Fatalf("sent %+v, want nothing", sent)
			}
		})
	}
}

func TestNotify_CabalAccessApproved_CabalFailureReturnsAsIs(t *testing.T) {
	t.Parallel()
	r := newPushRig(t)
	w := r.accessWorld(t)
	down := errs.New(errs.CodeDBUnavailable, "test.cabals")
	w.cabals.Fail("Cabal", down)
	e := w.decided(r, "request", "approved")
	d := r.emit(t, userActor(w.creator), e)

	err := handleKinds(t, r, r.sender, d, e, app.CabalAccessApproved{Cabals: w.cabals})

	if !errors.Is(err, down) {
		t.Fatalf("Handle = %v, want %v", err, down)
	}
	r.wantRecorded(t, d, 0)
}

func TestNotify_CabalAccessRequested_RenderReturnsACabalFailureAsIs(t *testing.T) {
	t.Parallel()
	down := errs.New(errs.CodeDBUnavailable, "test.cabals")
	cabals := fakes.NewCabal(nil, nil)
	cabals.Fail("Cabal", down)

	kind := app.CabalAccessRequested{Cabals: cabals}
	msg, err := kind.Render(t.Context(), events.CabalAccessRequested{}, ids.UserID{})

	if !errors.Is(err, down) || msg.Title != "" || msg.Body != "" {
		t.Fatalf("Render = %+v, %v, want an empty message and %v", msg, err, down)
	}
}
