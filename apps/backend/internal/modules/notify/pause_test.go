package notify_test

import (
	"errors"
	"maps"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/cabal"
	"github.com/monaco/monaco/apps/backend/internal/modules/funding"
	"github.com/monaco/monaco/apps/backend/internal/modules/notify"
	"github.com/monaco/monaco/apps/backend/internal/modules/notify/app"
	"github.com/monaco/monaco/apps/backend/internal/platform/apns"
	"github.com/monaco/monaco/apps/backend/internal/platform/bus"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
	"github.com/monaco/monaco/apps/backend/internal/testkit/fakes"
)

const cabalName = "Moonshot"

type cabalWorld struct {
	id       ids.CabalID
	members  []ids.UserID
	outsider ids.UserID
	cabals   *fakes.Cabal
}

func (r *pushRig) cabalOf(t *testing.T, members int) cabalWorld {
	t.Helper()
	w := cabalWorld{id: ids.CabalIDFrom(r.ids.NewV7()), outsider: r.user(t, "active")}
	r.device(t, w.outsider, token('z'))
	rows := make([]fakes.CabalMember, members)
	for i := range members {
		user := r.user(t, "active")
		r.device(t, user, token(byte('a'+i)))
		w.members = append(w.members, user)
		rows[i] = fakes.CabalMember{CabalID: w.id, Member: cabal.MemberView{
			UserID: user, Role: cabal.RoleMember, CanVote: true, JoinedAt: r.clock.Now(),
		}}
	}
	seed := fakes.CabalSeed{View: cabal.View{ID: w.id, Name: cabalName}}
	w.cabals = fakes.NewCabal([]fakes.CabalSeed{seed}, rows)
	return w
}

type pauseKind struct {
	kind, title, body string
	event             func(r *pushRig, cabalID *uuid.UUID) events.Event
	run               func(t *testing.T, r *pushRig, cabals app.Cabals, cabalID *uuid.UUID) (bus.Delivery, error)
}

func pausedEvent(r *pushRig, cabalID *uuid.UUID) events.CabalPaused {
	return events.CabalPaused{
		V: 1, PauseID: r.ids.NewV7(), CabalID: cabalID, Reason: string(funding.PauseReasonExternalDeposit),
		Scope: scopeOf(cabalID),
	}
}

func resumedEvent(_ *pushRig, cabalID *uuid.UUID) events.CabalResumed {
	return events.CabalResumed{V: 1, CabalID: cabalID, Scope: scopeOf(cabalID)}
}

func scopeOf(cabalID *uuid.UUID) string {
	if cabalID == nil {
		return "global"
	}
	return "cabal"
}

func pausedKind() pauseKind {
	return pauseKind{
		kind:  "cabal_paused",
		title: cabalName + " is paused",
		body:  "Trading is on hold while we check something. Your money is safe; we'll tell you when it resumes.",
		event: func(r *pushRig, cabalID *uuid.UUID) events.Event { return pausedEvent(r, cabalID) },
		run: func(t *testing.T, r *pushRig, cabals app.Cabals, cabalID *uuid.UUID) (bus.Delivery, error) {
			t.Helper()
			e := pausedEvent(r, cabalID)
			d := r.emit(t, opsActor, e)
			return d, handleKinds(t, r, r.sender, d, e, app.CabalPaused{Cabals: cabals})
		},
	}
}

func resumedKind() pauseKind {
	return pauseKind{
		kind:  "cabal_resumed",
		title: cabalName + " is back",
		body:  "Trading has resumed.",
		event: func(r *pushRig, cabalID *uuid.UUID) events.Event { return resumedEvent(r, cabalID) },
		run: func(t *testing.T, r *pushRig, cabals app.Cabals, cabalID *uuid.UUID) (bus.Delivery, error) {
			t.Helper()
			e := resumedEvent(r, cabalID)
			d := r.emit(t, opsActor, e)
			return d, handleKinds(t, r, r.sender, d, e, app.CabalResumed{Cabals: cabals})
		},
	}
}

func pauseKinds() []pauseKind { return []pauseKind{pausedKind(), resumedKind()} }

func wantEveryMemberPushed(t *testing.T, k pauseKind) {
	t.Helper()
	r := newPushRig(t)
	w := r.cabalOf(t, 3)
	cabalID := w.id.UUID()

	d, err := k.run(t, r, w.cabals, &cabalID)

	wantVerdict(t, err, "", errs.VerdictAck)
	want := make([]apns.Push, 0, len(w.members))
	states := map[ids.UserID]string{}
	for i, member := range w.members {
		states[member] = "delivered"
		want = append(want, apns.Push{
			UserID: member, Token: token(byte('a' + i)), Environment: apns.Sandbox,
			CollapseID: "pause-" + w.id.String(), Title: k.title, Body: k.body,
			Data: map[string]string{"kind": k.kind, "cabal_id": w.id.String()},
		})
	}
	sent := r.sender.Sent()
	slices.SortFunc(sent, func(a, b apns.Push) int { return strings.Compare(a.Token, b.Token) })
	if !reflect.DeepEqual(sent, want) {
		t.Fatalf("sent %+v, want one push per member and none to the outsider: %+v", sent, want)
	}
	r.wantStates(t, d, states)
	r.wantCount(t, "rows in one broadcast of three", 3, `SELECT count(*) FROM notifications n
		JOIN notification_broadcasts b ON b.id = n.broadcast_id
		WHERE b.source_event_id = $1 AND b.recipient_count = 3 AND b.kind = $2`, d.EventID.UUID(), k.kind)
	r.wantSentEvents(t, d, 3)
	r.wantRecorded(t, d, 1)
}

func TestNotify_CabalPaused_AllMembers(t *testing.T) {
	t.Parallel()
	wantEveryMemberPushed(t, pausedKind())
}

func TestNotify_CabalResumed_AllMembers(t *testing.T) {
	t.Parallel()
	wantEveryMemberPushed(t, resumedKind())
}

func TestNotify_CabalResumed_ReplacesThePausedPush(t *testing.T) {
	t.Parallel()
	r := newPushRig(t)
	w := r.cabalOf(t, 2)
	cabalID := w.id.UUID()
	collapseIDs := func(sent []apns.Push) map[ids.UserID]string {
		byUser := map[ids.UserID]string{}
		for _, p := range sent {
			byUser[p.UserID] = p.CollapseID
		}
		return byUser
	}

	_, paused := pausedKind().run(t, r, w.cabals, &cabalID)
	_, resumed := resumedKind().run(t, r, w.cabals, &cabalID)

	wantVerdict(t, paused, "", errs.VerdictAck)
	wantVerdict(t, resumed, "", errs.VerdictAck)
	sent := r.sender.Sent()
	if len(sent) != 4 {
		t.Fatalf("sent %d pushes, want a paused and a resumed push for each of 2 members", len(sent))
	}
	before, after := collapseIDs(sent[:2]), collapseIDs(sent[2:])
	if len(before) != 2 || !maps.Equal(before, after) {
		t.Fatalf("collapse ids %v then %v, want the same id per member so resumed replaces paused", before, after)
	}
}

func TestNotify_CabalPause_GlobalScopeNotifiesNobodyAndReadsNoCabal(t *testing.T) {
	t.Parallel()
	for _, k := range pauseKinds() {
		t.Run(k.kind, func(t *testing.T) {
			t.Parallel()
			r := newPushRig(t)
			w := r.cabalOf(t, 2)
			down := errs.New(errs.CodeDBUnavailable, "test.cabals")
			w.cabals.Fail("Members", down)
			w.cabals.Fail("Cabal", down)

			d, err := k.run(t, r, w.cabals, nil)

			wantVerdict(t, err, "", errs.VerdictAck)
			if sent := r.sender.Sent(); len(sent) != 0 {
				t.Fatalf("sent %+v for a global pause, want nothing", sent)
			}
			r.wantStates(t, d, map[ids.UserID]string{})
			r.wantRecorded(t, d, 1)
		})
	}
}

func TestNotify_CabalPause_PortFailuresReturnAsIsAndWriteNothing(t *testing.T) {
	t.Parallel()
	for _, k := range pauseKinds() {
		for _, op := range []string{"Members", "Cabal"} {
			t.Run(k.kind+"/"+op, func(t *testing.T) {
				t.Parallel()
				r := newPushRig(t)
				w := r.cabalOf(t, 2)
				down := errs.New(errs.CodeDBUnavailable, "test.cabals")
				w.cabals.Fail(op, down)
				cabalID := w.id.UUID()

				d, err := k.run(t, r, w.cabals, &cabalID)

				if !errors.Is(err, down) {
					t.Fatalf("Handle = %v, want the port error itself, %v", err, down)
				}
				wantVerdict(t, err, errs.CodeDBUnavailable, errs.VerdictNak)
				if sent := r.sender.Sent(); len(sent) != 0 {
					t.Fatalf("sent %+v after a failed %s, want nothing", sent, op)
				}
				r.wantStates(t, d, map[ids.UserID]string{})
				r.wantRecorded(t, d, 0)
			})
		}
	}
}

func TestNotify_CabalPaused_NeverRendersThePauseReason(t *testing.T) {
	t.Parallel()
	id := ids.CabalIDFrom(uuid.NewSHA1(uuid.NameSpaceOID, []byte("notify-pause-reason")))
	cabals := fakes.NewCabal([]fakes.CabalSeed{{View: cabal.View{ID: id, Name: cabalName}}}, nil)
	cabalID := id.UUID()
	rendered := make([]app.Message, 0, 2)
	for _, reason := range []funding.PauseReason{funding.PauseReasonExternalDeposit, funding.PauseReasonOps} {
		e := events.CabalPaused{V: 1, CabalID: &cabalID, Reason: string(reason), Scope: "cabal"}
		msg, err := app.CabalPaused{Cabals: cabals}.Render(t.Context(), e, ids.UserID{})
		if err != nil {
			t.Fatal(err)
		}
		texts := append([]string{msg.Title, msg.Body}, slices.Collect(maps.Values(msg.Data))...)
		for _, text := range texts {
			if strings.Contains(text, string(reason)) {
				t.Errorf("reason %q appears in %q", reason, text)
			}
		}
		rendered = append(rendered, msg)
	}
	if !reflect.DeepEqual(rendered[0], rendered[1]) {
		t.Fatalf("copy differs by reason: %+v and %+v", rendered[0], rendered[1])
	}
}

func (r *pushRig) wantModulePushedToMembers(t *testing.T, d bus.Delivery, k pauseKind, members []ids.UserID) {
	t.Helper()
	states := map[ids.UserID]string{}
	for _, member := range members {
		states[member] = "delivered"
	}
	r.wantStates(t, d, states)
	sent := r.sender.Sent()
	if len(sent) != len(members) {
		t.Fatalf("sent %d pushes, want one per member of %d", len(sent), len(members))
	}
	for _, p := range sent {
		if p.Title != k.title || p.Body != k.body {
			t.Errorf("push %q / %q, want %q / %q", p.Title, p.Body, k.title, k.body)
		}
	}
	r.wantRecorded(t, d, 1)
}

func TestNotify_CabalPause_ReachesTheMembersOfARealCabalByDefault(t *testing.T) {
	t.Parallel()
	for _, k := range pauseKinds() {
		t.Run(k.kind, func(t *testing.T) {
			t.Parallel()
			r := newPushRig(t)
			c := testkit.NewCabal(t, r.pool, testkit.WithMembers(3), testkit.WithName(cabalName))
			members := make([]ids.UserID, len(c.Members))
			for i, member := range c.Members {
				members[i] = member.ID
				r.device(t, member.ID, token(byte('a'+i)))
			}
			r.device(t, r.user(t, "active"), token('z'))
			deps := module.Deps{Pool: r.pool, UoW: r.uow, IDs: r.ids, Clock: r.clock}
			cabalID := c.ID.UUID()

			d := r.dispatchTo(t, notify.New(deps, notify.WithSender(r.sender)), opsActor, k.event(r, &cabalID))

			r.wantModulePushedToMembers(t, d, k, members)
		})
	}
}

func TestNotify_CabalPause_ReachesTheMembersOfTheCabalPortItIsGiven(t *testing.T) {
	t.Parallel()
	for _, k := range pauseKinds() {
		t.Run(k.kind, func(t *testing.T) {
			t.Parallel()
			r := newPushRig(t)
			w := r.cabalOf(t, 2)
			deps := module.Deps{Pool: r.pool, UoW: r.uow, IDs: r.ids, Clock: r.clock}
			cabalID := w.id.UUID()
			m := notify.New(deps, notify.WithSender(r.sender), notify.WithCabals(w.cabals))

			d := r.dispatchTo(t, m, opsActor, k.event(r, &cabalID))

			r.wantModulePushedToMembers(t, d, k, w.members)
		})
	}
}
