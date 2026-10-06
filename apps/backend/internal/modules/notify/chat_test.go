package notify_test

import (
	"cmp"
	"errors"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/cabal"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity"
	"github.com/monaco/monaco/apps/backend/internal/modules/notify"
	"github.com/monaco/monaco/apps/backend/internal/modules/notify/app"
	"github.com/monaco/monaco/apps/backend/internal/platform/apns"
	"github.com/monaco/monaco/apps/backend/internal/platform/bus"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/testkit/fakes"
)

const (
	mentionKind = "chat_mention"
	replyKind   = "chat_thread_reply"
)

type chatWorld struct {
	cabalWorld
	names *fakes.Identity
}

func (r *pushRig) chatWorldOf(t *testing.T, members int) chatWorld {
	t.Helper()
	w := chatWorld{cabalWorld: r.cabalOf(t, members)}
	cards := make([]identity.UserCard, len(w.members))
	for i, member := range w.members {
		cards[i] = identity.UserCard{ID: member, DisplayName: memberNames()[i]}
	}
	w.names = fakes.NewIdentity(cards, nil)
	return w
}

func uuidsOf(users ...ids.UserID) []uuid.UUID {
	out := make([]uuid.UUID, len(users))
	for i, user := range users {
		out[i] = user.UUID()
	}
	return out
}

func (r *pushRig) posted(t *testing.T, w chatWorld, poster ids.UserID) events.ChatMessagePosted {
	t.Helper()
	e := goldenEvent(t, events.TypeChatMessagePosted).(events.ChatMessagePosted)
	e.MessageID, e.CabalID, e.AuthorID, e.ParentID = r.ids.NewV7(), w.id.UUID(), poster.UUID(), nil
	e.MentionedUserIDs, e.ThreadParticipantIDs = []uuid.UUID{}, []uuid.UUID{}
	return e
}

func (r *pushRig) emitPosted(t *testing.T, e events.ChatMessagePosted) bus.Delivery {
	t.Helper()
	return r.emit(t, userActor(ids.UserIDFrom(e.AuthorID)), e)
}

func (r *pushRig) handlePosted(t *testing.T, w chatWorld, d bus.Delivery, e events.ChatMessagePosted) error {
	t.Helper()
	return handleKinds(t, r, r.sender, d, e,
		app.ChatMention{Cabals: w.cabals, Users: w.names}, app.ChatThreadReply{Cabals: w.cabals, Users: w.names})
}

func chatPush(
	to ids.UserID, tok string, e events.ChatMessagePosted, kind, body string, thread uuid.UUID,
) apns.Push {
	return apns.Push{
		UserID: to, Token: tok, Environment: apns.Sandbox, CollapseID: "chat-" + thread.String(),
		Title: cabalName, Body: body, Data: map[string]string{
			"kind": kind, "cabal_id": e.CabalID.String(), "message_id": e.MessageID.String(),
		},
	}
}

func (r *pushRig) wantPushed(t *testing.T, want ...apns.Push) {
	t.Helper()
	byToken := func(a, b apns.Push) int { return strings.Compare(a.Token, b.Token) }
	sent := r.sender.Sent()
	slices.SortFunc(sent, byToken)
	slices.SortFunc(want, byToken)
	if !reflect.DeepEqual(sent, want) {
		t.Fatalf("sent %+v, want exactly %+v", sent, want)
	}
}

func TestNotify_ChatMention_CurrentMembersOnly(t *testing.T) {
	t.Parallel()
	r := newPushRig(t)
	w := r.chatWorldOf(t, 2)
	poster, mentioned := w.members[0], w.members[1]
	e := r.posted(t, w, poster)
	e.MentionedUserIDs = uuidsOf(mentioned, w.outsider)
	d := r.emitPosted(t, e)

	wantVerdict(t, r.handlePosted(t, w, d, e), "", errs.VerdictAck)

	r.wantPushed(t, chatPush(mentioned, token('b'), e, mentionKind, "Dana mentioned you", e.MessageID))
	r.wantStates(t, d, map[ids.UserID]string{mentioned: "delivered"})
	r.wantSentEvents(t, d, 1)
	r.wantRecorded(t, d, 1)
}

func TestNotify_ChatThreadReply_OneRowPerUser(t *testing.T) {
	t.Parallel()
	r := newPushRig(t)
	w := r.chatWorldOf(t, 3)
	poster, a, b := w.members[0], w.members[1], w.members[2]
	parent := r.ids.NewV7()
	e := r.posted(t, w, poster)
	e.ParentID = &parent
	e.MentionedUserIDs = uuidsOf(b)
	e.ThreadParticipantIDs = uuidsOf(a, b, poster)
	d := r.emitPosted(t, e)

	wantVerdict(t, r.handlePosted(t, w, d, e), "", errs.VerdictAck)

	r.wantPushed(t,
		chatPush(a, token('b'), e, replyKind, "Dana replied in a thread you're in", parent),
		chatPush(b, token('c'), e, mentionKind, "Dana mentioned you", parent),
	)
	r.wantRows(t, map[string]int{"chat_mention/delivered": 1, "chat_thread_reply/delivered": 1})
	r.wantStates(t, d, map[ids.UserID]string{a: "delivered", b: "delivered"})
	r.wantSentEvents(t, d, 2)
	r.wantRecorded(t, d, 1)
}

func TestNotify_Chat_Silent(t *testing.T) {
	t.Parallel()
	r := newPushRig(t)
	w := r.chatWorldOf(t, 2)
	w.cabals.Fail("Members", errs.New(errs.CodeDBUnavailable, "test.cabal"))
	e := r.posted(t, w, w.members[0])
	d := r.emitPosted(t, e)

	wantVerdict(t, r.handlePosted(t, w, d, e), "", errs.VerdictAck)

	if sent := r.sender.Sent(); len(sent) != 0 {
		t.Fatalf("sent %+v, want nothing", sent)
	}
	r.wantRows(t, map[string]int{})
	r.wantRecorded(t, d, 1)
}

func chatKind(name string, cabals app.Cabals, users app.Users) app.Kind[events.ChatMessagePosted] {
	if name == replyKind {
		return app.ChatThreadReply{Cabals: cabals, Users: users}
	}
	return app.ChatMention{Cabals: cabals, Users: users}
}

func TestNotify_Chat_Recipients(t *testing.T) {
	t.Parallel()
	golden := goldenEvent(t, events.TypeChatMessagePosted).(events.ChatMessagePosted)
	cabalID := ids.CabalIDFrom(golden.CabalID)
	user := func(n byte) ids.UserID { return ids.UserIDFrom(uuid.UUID{15: n}) }
	var rows []fakes.CabalMember
	for n := byte(1); n <= 3; n++ {
		rows = append(rows, fakes.CabalMember{CabalID: cabalID, Member: cabal.MemberView{UserID: user(n)}})
	}
	down := errs.New(errs.CodeDBUnavailable, "test.cabal")
	for name, tc := range map[string]struct {
		kind                    string
		mentioned, participants []ids.UserID
		fail, wantErr           error
		want                    []ids.UserID
	}{
		"a mention of a member and of a former member": {
			kind: mentionKind, mentioned: []ids.UserID{user(1), user(4)}, want: []ids.UserID{user(1)},
		},
		"mentions in the cabal's member order": {
			kind: mentionKind, mentioned: []ids.UserID{user(3), user(1)}, want: []ids.UserID{user(1), user(3)},
		},
		"participants who were not mentioned": {
			kind: replyKind, mentioned: []ids.UserID{user(1)},
			participants: []ids.UserID{user(1), user(2), user(4)}, want: []ids.UserID{user(2)},
		},
		"a mention ignores the participants": {
			kind: mentionKind, participants: []ids.UserID{user(2)},
		},
		"participants who were all mentioned": {
			kind: replyKind, mentioned: []ids.UserID{user(1), user(2)}, participants: []ids.UserID{user(1), user(2)},
		},
		"no one mentioned reads no members": {kind: mentionKind, fail: down},
		"no participants reads no members":  {kind: replyKind, mentioned: []ids.UserID{user(1)}, fail: down},
		"a members failure on a mention": {
			kind: mentionKind, mentioned: []ids.UserID{user(1)}, fail: down, wantErr: down,
		},
		"a members failure on a thread reply": {
			kind: replyKind, participants: []ids.UserID{user(1)}, fail: down, wantErr: down,
		},
	} {
		e := golden
		e.MentionedUserIDs, e.ThreadParticipantIDs = uuidsOf(tc.mentioned...), uuidsOf(tc.participants...)
		cabals := fakes.NewCabal(nil, rows)
		if tc.fail != nil {
			cabals.Fail("Members", tc.fail)
		}

		got, err := chatKind(tc.kind, cabals, users{}).Recipients(t.Context(), e)

		if !errors.Is(err, tc.wantErr) || !slices.Equal(got, tc.want) {
			t.Errorf("%s: Recipients = %v, %v, want %v, %v", name, got, err, tc.want, tc.wantErr)
		}
	}
}

type chatRender struct {
	kind               string
	edit               func(*events.ChatMessagePosted)
	cards              map[ids.UserID]identity.UserCard
	cabalErr, usersErr error
	body, collapse     string
}

func chatRenders(golden events.ChatMessagePosted) map[string]chatRender {
	poster := ids.UserIDFrom(golden.AuthorID)
	card := func(name string, deleted bool) map[ids.UserID]identity.UserCard {
		return map[ids.UserID]identity.UserCard{poster: {ID: poster, DisplayName: name, Deleted: deleted}}
	}
	keep := func(*events.ChatMessagePosted) {}
	down := func(port string) error { return errs.New(errs.CodeDBUnavailable, "test."+port) }
	inThread, alone := "chat-"+golden.ParentID.String(), "chat-"+golden.MessageID.String()
	return map[string]chatRender{
		"a mention in a thread": {
			kind: mentionKind, edit: keep, cards: card("Dana", false), body: "Dana mentioned you", collapse: inThread,
		},
		"a mention outside a thread": {
			kind: mentionKind, edit: func(e *events.ChatMessagePosted) { e.ParentID = nil },
			cards: card("Dana", false), body: "Dana mentioned you", collapse: alone,
		},
		"a thread reply": {
			kind: replyKind, edit: keep, cards: card("Dana", false),
			body: "Dana replied in a thread you're in", collapse: inThread,
		},
		"a deleted author who kept a name": {
			kind: mentionKind, edit: keep, cards: card("Dana", true), body: "Someone mentioned you", collapse: inThread,
		},
		"an author with no name": {
			kind: replyKind, edit: keep, cards: card("", false),
			body: "Someone replied in a thread you're in", collapse: inThread,
		},
		"an author the port does not know": {
			kind: mentionKind, edit: keep, body: "Someone mentioned you", collapse: inThread,
		},
		"a users port failure": {kind: replyKind, edit: keep, usersErr: down("users")},
		"a cabal port failure": {kind: mentionKind, edit: keep, cabalErr: down("cabal")},
	}
}

func TestNotify_Chat_Render(t *testing.T) {
	t.Parallel()
	golden := goldenEvent(t, events.TypeChatMessagePosted).(events.ChatMessagePosted)
	seed := fakes.CabalSeed{View: cabal.View{ID: ids.CabalIDFrom(golden.CabalID), Name: cabalName}}
	for name, tc := range chatRenders(golden) {
		e := golden
		tc.edit(&e)
		cabals := fakes.NewCabal([]fakes.CabalSeed{seed}, nil)
		if tc.cabalErr != nil {
			cabals.Fail("Cabal", tc.cabalErr)
		}

		kind := chatKind(tc.kind, cabals, users{cards: tc.cards, err: tc.usersErr})

		msg, err := kind.Render(t.Context(), e, ids.UserID{})

		wantErr, want := cmp.Or(tc.cabalErr, tc.usersErr), app.Message{}
		if wantErr == nil {
			want = app.Message{Title: cabalName, Body: tc.body, CollapseID: tc.collapse, Data: map[string]string{
				"kind": tc.kind, "cabal_id": golden.CabalID.String(), "message_id": golden.MessageID.String(),
			}}
		}
		if !errors.Is(err, wantErr) || !reflect.DeepEqual(msg, want) {
			t.Errorf("%s: Render = %+v, %v, want %+v, %v", name, msg, err, want, wantErr)
		}
	}
}

func TestNotify_Chat_ReachesBothKindsThroughTheModule(t *testing.T) {
	t.Parallel()
	r := newPushRig(t)
	w := r.chatWorldOf(t, 3)
	poster, a, b := w.members[0], w.members[1], w.members[2]
	e := r.posted(t, w, poster)
	e.MentionedUserIDs, e.ThreadParticipantIDs = uuidsOf(b), uuidsOf(a, b)
	deps := module.Deps{Pool: r.pool, UoW: r.uow, IDs: r.ids, Clock: r.clock}
	m := notify.New(deps, notify.WithSender(r.sender), notify.WithCabals(w.cabals), notify.WithUsers(w.names))

	d := r.dispatchTo(t, m, userActor(poster), e)

	r.wantPushed(t,
		chatPush(a, token('b'), e, replyKind, "Dana replied in a thread you're in", e.MessageID),
		chatPush(b, token('c'), e, mentionKind, "Dana mentioned you", e.MessageID),
	)
	r.wantRows(t, map[string]int{"chat_mention/delivered": 1, "chat_thread_reply/delivered": 1})
	r.wantCount(t, "broadcasts led by the mention kind", 1, `SELECT count(*) FROM notification_broadcasts
		WHERE source_event_id = $1 AND kind = 'chat_mention' AND recipient_count = 2`, d.EventID.UUID())
	r.wantRecorded(t, d, 1)
}
