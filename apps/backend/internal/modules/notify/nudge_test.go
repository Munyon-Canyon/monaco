package notify_test

import (
	"errors"
	"reflect"
	"slices"
	"testing"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity"
	"github.com/monaco/monaco/apps/backend/internal/modules/notify"
	"github.com/monaco/monaco/apps/backend/internal/modules/notify/app"
	"github.com/monaco/monaco/apps/backend/internal/platform/apns"
	"github.com/monaco/monaco/apps/backend/internal/platform/bus"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
	"github.com/monaco/monaco/apps/backend/internal/testkit/fakes"
)

const nudgeActor = "system:poller.identity.nudges"

func (r *pushRig) reachableUser(t *testing.T, state identity.AuthState, status identity.AccountStatus) ids.UserID {
	t.Helper()
	user := testkit.SeedUser(t, r.pool, testkit.UserOpts{AuthState: string(state), AccountStatus: string(status)}).ID
	r.device(t, user, token('a'))
	return user
}

func (r *pushRig) nudgeDue(t *testing.T, user ids.UserID, kind string) events.UserNudgeDue {
	t.Helper()
	e := goldenEvent(t, events.TypeUserNudgeDue).(events.UserNudgeDue)
	e.UserID, e.Kind, e.At = user.UUID(), kind, r.clock.Now()
	return e
}

func (r *pushRig) nudged(t *testing.T, user ids.UserID, kind string) (bus.Delivery, events.UserNudgeDue) {
	t.Helper()
	e := r.nudgeDue(t, user, kind)
	return r.emit(t, nudgeActor, e), e
}

func (r *pushRig) handleNudge(t *testing.T, d bus.Delivery, e events.UserNudgeDue) error {
	t.Helper()
	return handleKinds(t, r, r.sender, d, e, app.Nudge{Users: r.users})
}

func nudgePush(user ids.UserID, title, body string) apns.Push {
	return apns.Push{
		UserID: user, Token: token('a'), Environment: apns.Sandbox, CollapseID: "nudge-" + user.String(),
		Title: title, Body: body, Data: map[string]string{"kind": "nudge", "user_id": user.String()},
	}
}

func (r *pushRig) wantNudgeDelivered(t *testing.T, d bus.Delivery, user ids.UserID, title, body string) {
	t.Helper()
	want := nudgePush(user, title, body)
	if sent := r.sender.Sent(); !reflect.DeepEqual(sent, []apns.Push{want}) {
		t.Fatalf("sent %+v, want only %+v", sent, want)
	}
	r.wantStates(t, d, map[ids.UserID]string{user: "delivered"})
	r.wantSentEvents(t, d, 1)
	r.wantRecorded(t, d, 1)
}

func (r *pushRig) wantNudgeSkipped(t *testing.T, user ids.UserID, kind string) {
	t.Helper()
	d, e := r.nudged(t, user, kind)

	wantVerdict(t, r.handleNudge(t, d, e), "", errs.VerdictAck)

	if sent := r.sender.Sent(); len(sent) != 0 {
		t.Fatalf("sent %+v, want nothing", sent)
	}
	r.wantStates(t, d, map[ids.UserID]string{})
	r.wantRecorded(t, d, 1)
}

func wantNudgePushed(t *testing.T, kind string, state identity.AuthState, title, body string) {
	t.Helper()
	r := newPushRig(t)
	user := r.reachableUser(t, state, identity.AccountActive)
	d, e := r.nudged(t, user, kind)

	wantVerdict(t, r.handleNudge(t, d, e), "", errs.VerdictAck)

	r.wantNudgeDelivered(t, d, user, title, body)
}

func TestNotify_Nudge_AwaitingPhone(t *testing.T) {
	t.Parallel()
	wantNudgePushed(
		t, "add_phone", identity.AuthAwaitingPhone,
		"Find your friends on Monaco", "Add your number to see who you know.",
	)
}

func TestNotify_Nudge_AwaitingSocials(t *testing.T) {
	t.Parallel()
	wantNudgePushed(
		t, "link_x", identity.AuthAwaitingSocials,
		"Connect X", "Link X to find people you already follow.",
	)
}

func TestNotify_Nudge_StaleStateSkipped(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct {
		kind  string
		state identity.AuthState
	}{
		"phone nudge after both steps are done": {"add_phone", identity.AuthOnboardingCompleted},
		"X nudge after both steps are done":     {"link_x", identity.AuthOnboardingCompleted},
		"phone nudge after the phone was added": {"add_phone", identity.AuthAwaitingSocials},
		"X nudge after the phone was unlinked":  {"link_x", identity.AuthAwaitingPhone},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			r := newPushRig(t)

			r.wantNudgeSkipped(t, r.reachableUser(t, tc.state, identity.AccountActive), tc.kind)
		})
	}
}

func TestNotify_Nudge_InactiveSkipped(t *testing.T) {
	t.Parallel()
	for _, status := range []identity.AccountStatus{
		identity.AccountSuspended, identity.AccountBanned, identity.AccountDeleted,
	} {
		t.Run(string(status), func(t *testing.T) {
			t.Parallel()
			r := newPushRig(t)

			r.wantNudgeSkipped(t, r.reachableUser(t, identity.AuthAwaitingPhone, status), "add_phone")
		})
	}
}

func TestNotify_Nudge_RecipientsAreTheUserOnlyWhileTheCardStillFits(t *testing.T) {
	t.Parallel()
	user := ids.UserIDFrom(uuid.NewSHA1(uuid.NameSpaceOID, []byte("notify-nudge-recipient")))
	card := func(state identity.AuthState, status identity.AccountStatus) identity.UserCard {
		return identity.UserCard{ID: user, AuthState: state, AccountStatus: status}
	}
	type scenario struct {
		kind  string
		cards []identity.UserCard
		want  []ids.UserID
	}
	scenarios := map[string]scenario{
		"a card the port does not know": {"add_phone", nil, nil},
		"a kind the module does not know": {
			"invite_friends", []identity.UserCard{card(identity.AuthAwaitingPhone, identity.AccountActive)}, nil,
		},
		"a deleted card that is otherwise due": {"add_phone", []identity.UserCard{{
			ID: user, AuthState: identity.AuthAwaitingPhone, AccountStatus: identity.AccountActive, Deleted: true,
		}}, nil},
	}
	for kind, due := range map[string]identity.AuthState{
		"add_phone": identity.AuthAwaitingPhone, "link_x": identity.AuthAwaitingSocials,
	} {
		for _, state := range []identity.AuthState{
			identity.AuthCreated, identity.AuthAwaitingPhone, identity.AuthAwaitingSocials,
			identity.AuthOnboardingCompleted,
		} {
			var want []ids.UserID
			if state == due {
				want = []ids.UserID{user}
			}
			cards := []identity.UserCard{card(state, identity.AccountActive)}
			scenarios[kind+" while "+string(state)] = scenario{kind, cards, want}
		}
		for _, status := range []identity.AccountStatus{
			identity.AccountSuspended, identity.AccountBanned, identity.AccountDeleted,
		} {
			cards := []identity.UserCard{card(due, status)}
			scenarios[kind+" on a "+string(status)+" account"] = scenario{kind, cards, nil}
		}
	}
	for name, tc := range scenarios {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			e := events.UserNudgeDue{V: 1, UserID: user.UUID(), Kind: tc.kind, NudgeNumber: 1}

			got, err := app.Nudge{Users: fakes.NewIdentity(tc.cards, nil)}.Recipients(t.Context(), e)

			if err != nil || !slices.Equal(got, tc.want) {
				t.Fatalf("Recipients = %v, %v, want %v", got, err, tc.want)
			}
		})
	}
}

func TestNotify_Nudge_ReturnsAUsersPortFailureAsIsAndNaks(t *testing.T) {
	t.Parallel()
	r := newPushRig(t)
	user := r.reachableUser(t, identity.AuthAwaitingPhone, identity.AccountActive)
	down := errs.New(errs.CodeDBUnavailable, "test.users")
	d, e := r.nudged(t, user, "add_phone")

	err := handleKinds(t, r, r.sender, d, e, app.Nudge{Users: users{err: down}})

	if !errors.Is(err, down) {
		t.Fatalf("Handle = %v, want the port error itself, %v", err, down)
	}
	wantVerdict(t, err, errs.CodeDBUnavailable, errs.VerdictNak)
	r.wantRows(t, map[string]int{})
	r.wantRecorded(t, d, 0)
}

func TestNotify_Nudge_ReachesTheUserThroughTheModule(t *testing.T) {
	t.Parallel()
	r := newPushRig(t)
	user := r.reachableUser(t, identity.AuthAwaitingSocials, identity.AccountActive)
	deps := module.Deps{Pool: r.pool, UoW: r.uow, IDs: r.ids, Clock: r.clock}

	d := r.dispatchTo(t, notify.New(deps, notify.WithSender(r.sender)), nudgeActor, r.nudgeDue(t, user, "link_x"))

	r.wantNudgeDelivered(t, d, user, "Connect X", "Link X to find people you already follow.")
}

func TestNotify_Nudge_LinkXCopyPassesTheCopyAudit(t *testing.T) {
	t.Parallel()
	e := goldenEvent(t, events.TypeUserNudgeDue).(events.UserNudgeDue)
	e.Kind = "link_x"

	msg, err := app.Nudge{}.Render(t.Context(), e, ids.UserIDFrom(e.UserID))

	if faults := copyFaults(msg); err != nil || len(faults) > 0 || msg.Title == "" || msg.Body == "" {
		t.Fatalf("link_x copy %+v, %v breaks the rules: %q", msg, err, faults)
	}
}
