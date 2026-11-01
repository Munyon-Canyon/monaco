package notify_test

import (
	"context"
	"errors"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/cabal"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity"
	"github.com/monaco/monaco/apps/backend/internal/modules/notify/app"
	"github.com/monaco/monaco/apps/backend/internal/platform/apns"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/testkit/fakes"
)

func (r *pushRig) adminWorld(t *testing.T) (accessWorld, ids.UserID) {
	t.Helper()
	w := r.accessWorld(t)
	admin := r.follower(t, "cyd", "Cyd")
	r.device(t, admin, token('c'))
	member := r.follower(t, "dee", "Dee")
	r.device(t, member, token('d'))
	joined := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	view := cabal.View{ID: w.cabalID, Name: cabalName}
	w.cabals = fakes.NewCabal([]fakes.CabalSeed{{View: view}},
		[]fakes.CabalMember{
			{CabalID: w.cabalID, Member: cabal.MemberView{UserID: member, Role: cabal.RoleMember, JoinedAt: joined}},
			{CabalID: w.cabalID, Member: cabal.MemberView{
				UserID: admin, Role: cabal.Role("admin"), CanVote: true, JoinedAt: joined.Add(time.Hour),
			}},
			{CabalID: w.cabalID, Member: cabal.MemberView{
				UserID: w.creator, Role: cabal.RoleCreator, CanVote: true, JoinedAt: joined.Add(2 * time.Hour),
			}},
		})
	return w, admin
}

func TestNotify_CabalAccessRequestedAdmins_PushesEveryAdminOnceAndNobodyElse(t *testing.T) {
	t.Parallel()
	r := newPushRig(t)
	w, admin := r.adminWorld(t)
	e := w.requested(r, "request")
	d := r.emit(t, userActor(w.requester), e)
	kind := app.CabalAccessRequestedAdmins{
		CabalAccessRequested: app.CabalAccessRequested{Cabals: w.cabals, Users: r.users}, Admins: w.cabals,
	}

	err := handleKinds(t, r, r.sender, d, e, kind)

	wantVerdict(t, err, "", errs.VerdictAck)
	push := func(user ids.UserID, tok string) apns.Push {
		return inCabal(apns.Push{
			UserID: user, Token: tok, Environment: apns.Sandbox, CollapseID: "access-" + e.RequestID.String(),
			Title: "New request to join", Body: "Bea wants to join " + cabalName,
			Data: map[string]string{"kind": "cabal_access_requested", "cabal_id": w.cabalID.String()},
		}, w.cabalID, wantSender{"person", w.requester.String(), "Bea"})
	}
	want := []apns.Push{push(w.creator, token('a')), push(admin, token('c'))}
	sent := r.sender.Sent()
	slices.SortFunc(sent, func(a, b apns.Push) int { return strings.Compare(a.Token, b.Token) })
	if !reflect.DeepEqual(sent, want) {
		t.Fatalf("sent %+v, want one push for the creator and one for the admin: %+v", sent, want)
	}
	r.wantStates(t, d, map[ids.UserID]string{w.creator: "delivered", admin: "delivered"})
	r.wantRecorded(t, d, 1)
}

func TestNotify_CabalAccessRequestedAdmins_InviteNotifiesNobody(t *testing.T) {
	t.Parallel()
	r := newPushRig(t)
	w, _ := r.adminWorld(t)
	e := w.requested(r, "invite")
	d := r.emit(t, userActor(w.creator), e)
	kind := app.CabalAccessRequestedAdmins{
		CabalAccessRequested: app.CabalAccessRequested{Cabals: w.cabals, Users: r.users}, Admins: w.cabals,
	}

	err := handleKinds(t, r, r.sender, d, e, kind)

	wantVerdict(t, err, "", errs.VerdictAck)
	if sent := r.sender.Sent(); len(sent) != 0 {
		t.Fatalf("sent %+v for an invite, want nothing", sent)
	}
}

func TestNotify_CabalAccessRequestedAdmins_AdminsFailureReturnsAsIs(t *testing.T) {
	t.Parallel()
	r := newPushRig(t)
	w, _ := r.adminWorld(t)
	down := errs.New(errs.CodeDBUnavailable, "test.admins")
	w.cabals.Fail("Admins", down)
	e := w.requested(r, "request")
	d := r.emit(t, userActor(w.requester), e)
	kind := app.CabalAccessRequestedAdmins{
		CabalAccessRequested: app.CabalAccessRequested{Cabals: w.cabals, Users: r.users}, Admins: w.cabals,
	}

	err := handleKinds(t, r, r.sender, d, e, kind)

	if !errors.Is(err, down) {
		t.Fatalf("Handle = %v, want %v", err, down)
	}
	if sent := r.sender.Sent(); len(sent) != 0 {
		t.Fatalf("sent %+v after a failed Admins read, want nothing", sent)
	}
	r.wantRecorded(t, d, 0)
}

func TestNotify_CabalAccessRequestedAdmins_NamesTheLinkOwnerWhenKnown(t *testing.T) {
	t.Parallel()
	r := newPushRig(t)
	w, _ := r.adminWorld(t)
	jordan := r.follower(t, "jordan", "Jordan")
	e := w.requested(r, "request")
	e.ViaUserID = new(jordan.UUID())
	d := r.emit(t, userActor(w.requester), e)
	kind := app.CabalAccessRequestedAdmins{
		CabalAccessRequested: app.CabalAccessRequested{Cabals: w.cabals, Users: r.users}, Admins: w.cabals,
	}

	err := handleKinds(t, r, r.sender, d, e, kind)

	wantVerdict(t, err, "", errs.VerdictAck)
	sent := r.sender.Sent()
	if len(sent) != 2 {
		t.Fatalf("sent %+v, want one push per admin", sent)
	}
	for _, push := range sent {
		if want := "Bea wants to join " + cabalName + " through Jordan's link"; push.Body != want {
			t.Fatalf("body %q, want %q", push.Body, want)
		}
	}
}

type failVia struct {
	app.Users
	via  ids.UserID
	fail error
}

func (f failVia) UsersByID(ctx context.Context, userIDs []ids.UserID) (map[ids.UserID]identity.UserCard, error) {
	if slices.Contains(userIDs, f.via) {
		return nil, f.fail
	}
	return f.Users.UsersByID(ctx, userIDs)
}

func TestNotify_CabalAccessRequestedAdmins_LinkOwnerReadFailureReturnsAsIs(t *testing.T) {
	t.Parallel()
	r := newPushRig(t)
	w, admin := r.adminWorld(t)
	jordan := r.follower(t, "jordan", "Jordan")
	e := w.requested(r, "request")
	e.ViaUserID = new(jordan.UUID())
	down := errs.New(errs.CodeDBUnavailable, "test.users")
	users := failVia{Users: r.users, via: jordan, fail: down}
	kind := app.CabalAccessRequestedAdmins{
		CabalAccessRequested: app.CabalAccessRequested{Cabals: w.cabals, Users: users}, Admins: w.cabals,
	}

	_, err := kind.Render(t.Context(), e, admin)

	if !errors.Is(err, down) {
		t.Fatalf("Render = %v, want %v", err, down)
	}
}
