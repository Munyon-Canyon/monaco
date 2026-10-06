package social_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/cabal"
	cabalport "github.com/monaco/monaco/apps/backend/internal/modules/cabal/port"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
	"github.com/monaco/monaco/apps/backend/internal/testkit/fakes"
)

type chatFixture struct {
	now      time.Time
	pool     *pgxpool.Pool
	clock    *testkit.Clock
	deps     app.ChatDeps
	rt       *testkit.FakeRealtime
	post     *app.PostChatMessageHandler
	del      *app.DeleteChatMessageHandler
	cabal    testkit.SeededCabal
	other    testkit.SeededCabal
	outsider ids.UserID
	users    *fakes.Identity
}

func newChatFixture(t *testing.T) chatFixture {
	t.Helper()
	pool := testkit.DB(t)
	g := testkit.NewIDs(591)
	now := clock.Real{}.Now().UTC().Truncate(time.Microsecond)
	clk := testkit.NewClock(now)
	uow := db.New(pool, g, clk)
	rt := &testkit.FakeRealtime{}
	deps := app.ChatDeps{
		UoW: uow, Members: cabal.New(module.Deps{Pool: pool}).Queries(), IDs: g, Clock: clk,
		Publish: app.NewChatPublisher(rt, plainWire),
	}
	f := chatFixture{
		now: now, pool: pool, clock: clk, rt: rt,
		cabal: testkit.NewCabal(t, pool, testkit.WithMembers(3)), other: testkit.NewCabal(t, pool),
		outsider: testkit.SeedUser(t, pool, testkit.UserOpts{}).ID,
	}
	cards := make([]identity.UserCard, 0, 1+len(f.cabal.Members))
	cards = append(cards, identity.UserCard{ID: f.outsider, Handle: "outsider", AccountStatus: identity.AccountActive})
	for i, m := range f.cabal.Members {
		cards = append(cards, identity.UserCard{
			ID: m.ID, Handle: fmt.Sprintf("member%d", i), AccountStatus: identity.AccountActive,
		})
	}
	f.users = fakes.NewIdentity(cards, nil)
	deps.Users = f.users
	f.deps = deps
	f.post, f.del = app.NewPostChatMessageHandler(deps), app.NewDeleteChatMessageHandler(deps)
	return f
}

func plainWire(_ context.Context, m app.ChatMessage) (any, error) {
	return map[string]any{"id": m.ID, "body": m.Body}, nil
}

func (f chatFixture) member(i int) ids.UserID { return f.cabal.Members[i].ID }

func (f chatFixture) as(t *testing.T, user ids.UserID) context.Context {
	t.Helper()
	return observability.WithActor(t.Context(), "user:"+user.String())
}

func (f chatFixture) send(t *testing.T, author ids.UserID, body string, reply *domain.Reply) (app.ChatMessage, error) {
	t.Helper()
	parsed, err := domain.ParseChatBody(body)
	if err != nil {
		t.Fatal(err)
	}
	return f.post.Handle(f.as(t, author), app.PostChatMessage{
		CabalID: f.cabal.ID, Author: author, Body: parsed, Reply: reply,
	})
}

func (f chatFixture) mustSend(t *testing.T, author ids.UserID, body string, reply *domain.Reply) app.ChatMessage {
	t.Helper()
	m, err := f.send(t, author, body, reply)
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func (f chatFixture) count(t *testing.T, query string, args ...any) int {
	t.Helper()
	var n int
	if err := f.pool.QueryRow(t.Context(), query, args...).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func (f chatFixture) posted(t *testing.T) int {
	t.Helper()
	return f.count(t, `SELECT count(*) FROM events WHERE type = $1`, string(events.TypeChatMessagePosted))
}

func (f chatFixture) parentState(t *testing.T, id uuid.UUID) (int, *time.Time) {
	t.Helper()
	var n int
	var last *time.Time
	if err := f.pool.QueryRow(t.Context(), `SELECT reply_count, last_reply_at FROM cabal_messages WHERE id = $1`, id).
		Scan(&n, &last); err != nil {
		t.Fatal(err)
	}
	return n, last
}

func TestPostChatMessage_storesTheMessageAndAppendsAnIDsOnlyEvent(t *testing.T) {
	t.Parallel()
	f := newChatFixture(t)
	got := f.mustSend(t, f.member(0), "  gm frens  ", nil)
	want := app.ChatMessage{ID: got.ID, CabalID: f.cabal.ID, AuthorID: f.member(0), Body: "gm frens", CreatedAt: f.now}
	if got != want {
		t.Fatalf("posted = %+v, want %+v", got, want)
	}
	var (
		body, actor string
		aggregate   uuid.UUID
		payload     []byte
	)
	if err := f.pool.QueryRow(t.Context(),
		`SELECT m.body, e.actor_type || ':' || e.actor_id, e.aggregate_id, e.payload
		 FROM cabal_messages m, events e WHERE e.type = 'chat.message_posted'`).
		Scan(&body, &actor, &aggregate, &payload); err != nil {
		t.Fatal(err)
	}
	var ev events.ChatMessagePosted
	if err := json.Unmarshal(payload, &ev); err != nil {
		t.Fatal(err)
	}
	wantEv := events.ChatMessagePosted{
		V: 1, MessageID: got.ID, CabalID: f.cabal.ID.UUID(), AuthorID: f.member(0).UUID(), CreatedAt: f.now,
		MentionedUserIDs: []uuid.UUID{}, ThreadParticipantIDs: []uuid.UUID{},
	}
	if !reflect.DeepEqual(ev, wantEv) {
		t.Fatalf("event = %+v, want %+v", ev, wantEv)
	}
	var fields map[string]any
	if err := json.Unmarshal(payload, &fields); err != nil {
		t.Fatal(err)
	}
	if _, leaked := fields["body"]; leaked || body != "gm frens" || aggregate != got.ID ||
		actor != "user:"+f.member(0).String() {
		t.Fatalf("payload %s, row body %q, aggregate %s, actor %s", payload, body, aggregate, actor)
	}
}

func TestPostChatMessage_aReplyBumpsItsParent(t *testing.T) {
	t.Parallel()
	f := newChatFixture(t)
	parent := f.mustSend(t, f.member(0), "proposal idea", nil)
	f.clock.Advance(time.Minute)
	reply := f.mustSend(t, f.member(1), "agreed", &domain.Reply{Parent: parent.ID, AlsoInChannel: true})
	if reply.ParentID != parent.ID || !reply.AlsoInChannel {
		t.Fatalf("reply = %+v, want parent %s also in channel", reply, parent.ID)
	}
	n, last := f.parentState(t, parent.ID)
	if want := f.now.Add(time.Minute); n != 1 || last == nil || !last.Equal(want) {
		t.Fatalf("parent reply_count %d, last_reply_at %v; want 1, %s", n, last, want)
	}
	var ev events.ChatMessagePosted
	var payload []byte
	if err := f.pool.QueryRow(t.Context(), `SELECT payload FROM events WHERE aggregate_id = $1`, reply.ID).
		Scan(&payload); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(payload, &ev); err != nil {
		t.Fatal(err)
	}
	if ev.ParentID == nil || *ev.ParentID != parent.ID || !ev.AlsoInChannel {
		t.Fatalf("event = %+v, want parent %s also in channel", ev, parent.ID)
	}
}

func TestPostChatMessage_concurrentRepliesCountEveryReply(t *testing.T) {
	t.Parallel()
	f := newChatFixture(t)
	parent := f.mustSend(t, f.member(0), "thread", nil)
	const replies = 20
	var wg sync.WaitGroup
	errc := make(chan error, replies)
	for i := range replies {
		author := f.member(i % 3)
		wg.Go(func() {
			_, err := f.send(t, author, "reply", &domain.Reply{Parent: parent.ID})
			errc <- err
		})
	}
	wg.Wait()
	close(errc)
	for err := range errc {
		if err != nil {
			t.Fatal(err)
		}
	}
	if n, _ := f.parentState(t, parent.ID); n != replies {
		t.Fatalf("reply_count = %d, want %d", n, replies)
	}
	if got := f.posted(t); got != replies+1 {
		t.Fatalf("chat.message_posted = %d, want %d", got, replies+1)
	}
}

func TestPostChatMessage_refusesBeforeWritingAnything(t *testing.T) {
	t.Parallel()
	f := newChatFixture(t)
	top := f.mustSend(t, f.member(0), "top", nil)
	reply := f.mustSend(t, f.member(1), "reply", &domain.Reply{Parent: top.ID})
	gone := f.mustSend(t, f.member(0), "gone", nil)
	if err := f.del.Handle(f.as(t, f.member(0)), app.DeleteChatMessage{
		CabalID: f.cabal.ID, MessageID: gone.ID, Caller: f.member(0),
	}); err != nil {
		t.Fatal(err)
	}
	elsewhere, err := f.post.Handle(f.as(t, f.other.Creator.ID), app.PostChatMessage{
		CabalID: f.other.ID, Author: f.other.Creator.ID, Body: "other cabal",
	})
	if err != nil {
		t.Fatal(err)
	}
	before := f.count(t, `SELECT count(*) FROM cabal_messages`)
	appended := f.posted(t)
	tests := []struct {
		name   string
		author ids.UserID
		reply  *domain.Reply
		want   errs.Code
	}{
		{"outsider", f.outsider, nil, errs.CodeNotCabalMember},
		{"outsider replying", f.outsider, &domain.Reply{Parent: top.ID}, errs.CodeNotCabalMember},
		{"unknown parent", f.member(0), &domain.Reply{Parent: ids.Real{}.NewV7()}, errs.CodeChatParentNotFound},
		{"parent in another cabal", f.member(0), &domain.Reply{Parent: elsewhere.ID}, errs.CodeChatParentNotFound},
		{"deleted parent", f.member(0), &domain.Reply{Parent: gone.ID}, errs.CodeChatParentNotFound},
		{"reply to a reply", f.member(2), &domain.Reply{Parent: reply.ID}, errs.CodeChatParentIsReply},
	}
	for _, tt := range tests {
		_, err := f.send(t, tt.author, "nope", tt.reply)
		if got := errs.CodeOf(err); err == nil || got != tt.want {
			t.Errorf("%s: err = %v (code %s), want %s", tt.name, err, got, tt.want)
		}
	}
	if after := f.count(t, `SELECT count(*) FROM cabal_messages`); after != before {
		t.Fatalf("messages = %d, want %d", after, before)
	}
	if n, _ := f.parentState(t, top.ID); n != 1 {
		t.Fatalf("top reply_count = %d, want 1", n)
	}
	if got := f.posted(t); got != appended {
		t.Fatalf("chat.message_posted = %d, want %d", got, appended)
	}
}

func (f chatFixture) lastPosted(t *testing.T) events.ChatMessagePosted {
	t.Helper()
	var payload []byte
	if err := f.pool.QueryRow(t.Context(),
		`SELECT payload FROM events WHERE type = $1 ORDER BY id DESC LIMIT 1`, string(events.TypeChatMessagePosted),
	).Scan(&payload); err != nil {
		t.Fatal(err)
	}
	var ev events.ChatMessagePosted
	if err := json.Unmarshal(payload, &ev); err != nil {
		t.Fatal(err)
	}
	return ev
}

func (f chatFixture) leave(t *testing.T, user ids.UserID) {
	t.Helper()
	if _, err := f.pool.Exec(t.Context(),
		`DELETE FROM cabal_members WHERE cabal_id = $1 AND user_id = $2`, f.cabal.ID.UUID(), user.UUID()); err != nil {
		t.Fatal(err)
	}
}

func TestPostChatMessage_MentionsResolved(t *testing.T) {
	t.Parallel()
	f := newChatFixture(t)
	f.mustSend(t, f.member(0), "@Member1 @outsider @nobody_here @member0 @member2 @member1 hi", nil)
	ev := f.lastPosted(t)
	want := []uuid.UUID{f.member(1).UUID(), f.member(2).UUID()}
	noThread := len(ev.ThreadParticipantIDs) == 0 && ev.ThreadParticipantIDs != nil
	if !reflect.DeepEqual(ev.MentionedUserIDs, want) || !noThread {
		t.Fatalf("mentioned = %v, participants = %v, want %v and []",
			ev.MentionedUserIDs, ev.ThreadParticipantIDs, want)
	}
}

func TestPostChatMessage_PlainMessageCarriesEmptyArrays(t *testing.T) {
	t.Parallel()
	f := newChatFixture(t)
	f.mustSend(t, f.member(0), "gm", nil)
	var raw string
	if err := f.pool.QueryRow(t.Context(),
		`SELECT (payload::jsonb->>'mentioned_user_ids') || (payload::jsonb->>'thread_participant_ids')
		 FROM events WHERE type = $1`,
		string(events.TypeChatMessagePosted)).Scan(&raw); err != nil || raw != "[][]" {
		t.Fatalf("arrays = %q, %v, want [][]", raw, err)
	}
}

func TestPostChatMessage_ThreadParticipants(t *testing.T) {
	t.Parallel()
	f := newChatFixture(t)
	top := f.mustSend(t, f.member(0), "top", nil)
	f.mustSend(t, f.member(1), "one", &domain.Reply{Parent: top.ID})
	f.mustSend(t, f.member(2), "two", &domain.Reply{Parent: top.ID})
	f.leave(t, f.member(2))
	f.mustSend(t, f.member(1), "again", &domain.Reply{Parent: top.ID})
	got := f.lastPosted(t).ThreadParticipantIDs
	want := []uuid.UUID{f.member(0).UUID()}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("participants = %v, want %v (parent author only; member 2 left, member 1 posted)", got, want)
	}
}

func TestPostChatMessage_IdentityDown(t *testing.T) {
	t.Parallel()
	f := newChatFixture(t)
	f.users.Fail("UserIDsByHandles", errs.New(errs.CodeUpstreamUnavailable, "test"))
	_, err := f.send(t, f.member(0), "@member1 look", nil)
	wantCode(t, err, errs.CodeUpstreamUnavailable)
	if n := f.count(t, `SELECT count(*) FROM cabal_messages`); n != 0 || f.posted(t) != 0 {
		t.Fatalf("rows = %d, events = %d, want none", n, f.posted(t))
	}
}

func TestPostChatMessage_MembersDown(t *testing.T) {
	t.Parallel()
	f := newChatFixture(t)
	deps := f.deps
	deps.Members = membersListDown{f.deps.Members}
	_, err := app.NewPostChatMessageHandler(deps).Handle(f.as(t, f.member(0)), app.PostChatMessage{
		CabalID: f.cabal.ID, Author: f.member(0), Body: mustBody(t, "@member1"),
	})
	wantCode(t, err, errs.CodeUpstreamUnavailable)
}

type membersListDown struct{ app.ChatMembers }

func (membersListDown) Members(context.Context, ids.CabalID) ([]cabalport.MemberView, error) {
	return nil, errs.New(errs.CodeUpstreamUnavailable, "test")
}

func mustBody(t *testing.T, raw string) domain.ChatBody {
	t.Helper()
	b, err := domain.ParseChatBody(raw)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestPostChatMessage_returnsTheMembershipLookupFailure(t *testing.T) {
	t.Parallel()
	f := newChatFixture(t)
	members := fakes.NewCabal(nil, nil)
	boom := errs.New(errs.CodeUpstreamUnavailable, "test")
	members.Fail("IsMember", boom)
	deps := f.deps
	deps.Members = members
	_, err := app.NewPostChatMessageHandler(deps).Handle(f.as(t, f.member(0)), app.PostChatMessage{
		CabalID: f.cabal.ID, Author: f.member(0), Body: "gm",
	})
	if !errors.Is(err, boom) || errs.CodeOf(err) != errs.CodeUpstreamUnavailable {
		t.Fatalf("err = %v, want the lookup error", err)
	}
}

func TestDeleteChatMessage_softDeletesTheAuthorsMessageOnce(t *testing.T) {
	t.Parallel()
	f := newChatFixture(t)
	m := f.mustSend(t, f.member(0), "oops", nil)
	appended := f.count(t, `SELECT count(*) FROM events`)
	f.clock.Advance(time.Minute)
	cmd := app.DeleteChatMessage{CabalID: f.cabal.ID, MessageID: m.ID, Caller: f.member(0)}
	for range 2 {
		if err := f.del.Handle(f.as(t, f.member(0)), cmd); err != nil {
			t.Fatal(err)
		}
		f.clock.Advance(time.Minute)
	}
	var deleted time.Time
	if err := f.pool.QueryRow(t.Context(), `SELECT deleted_at FROM cabal_messages WHERE id = $1`, m.ID).
		Scan(&deleted); err != nil {
		t.Fatal(err)
	}
	if want := f.now.Add(time.Minute); !deleted.Equal(want) {
		t.Fatalf("deleted_at = %s, want the first delete's clock reading %s", deleted, want)
	}
	if got := f.count(t, `SELECT count(*) FROM events`); got != appended {
		t.Fatalf("events = %d, want %d: a delete appends nothing", got, appended)
	}
}

func TestDeleteChatMessage_refusesWhenTheCallerCannotDeleteIt(t *testing.T) {
	t.Parallel()
	f := newChatFixture(t)
	m := f.mustSend(t, f.member(0), "mine", nil)
	tests := []struct {
		name string
		cmd  app.DeleteChatMessage
		want errs.Code
	}{
		{
			"another member's message",
			app.DeleteChatMessage{CabalID: f.cabal.ID, MessageID: m.ID, Caller: f.member(1)},
			errs.CodeChatMessageNotOwned,
		},
		{
			"unknown message",
			app.DeleteChatMessage{CabalID: f.cabal.ID, MessageID: ids.Real{}.NewV7(), Caller: f.member(0)},
			errs.CodeChatMessageNotFound,
		},
		{
			"message in another cabal",
			app.DeleteChatMessage{CabalID: f.other.ID, MessageID: m.ID, Caller: f.member(0)},
			errs.CodeChatMessageNotFound,
		},
	}
	for _, tt := range tests {
		err := f.del.Handle(f.as(t, tt.cmd.Caller), tt.cmd)
		if got := errs.CodeOf(err); err == nil || got != tt.want {
			t.Errorf("%s: err = %v (code %s), want %s", tt.name, err, got, tt.want)
		}
	}
	if n := f.count(t, `SELECT count(*) FROM cabal_messages WHERE deleted_at IS NOT NULL`); n != 0 {
		t.Fatalf("deleted messages = %d, want 0", n)
	}
}

func TestChatCommands_failWithInternalWhenTheStoreFails(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		breaker string
		run     func(t *testing.T, f chatFixture, parent app.ChatMessage) error
	}{
		{
			"post when the insert fails",
			`ALTER TABLE cabal_messages ADD CONSTRAINT no_more CHECK (false) NOT VALID`,
			func(t *testing.T, f chatFixture, _ app.ChatMessage) error {
				t.Helper()
				_, err := f.send(t, f.member(0), "gm", nil)
				return err
			},
		},
		{
			"reply when the parent bump fails",
			`ALTER TABLE cabal_messages ADD CONSTRAINT no_replies CHECK (reply_count = 0) NOT VALID`,
			func(t *testing.T, f chatFixture, parent app.ChatMessage) error {
				t.Helper()
				_, err := f.send(t, f.member(1), "reply", &domain.Reply{Parent: parent.ID})
				return err
			},
		},
		{
			"reply when the thread participants read fails",
			`CREATE FUNCTION arm_reads() RETURNS trigger LANGUAGE plpgsql AS $$
			   BEGIN PERFORM set_config('chat.reads', 'broken', true); RETURN NULL; END $$;
			 CREATE TRIGGER arm_reads AFTER INSERT ON cabal_messages FOR EACH ROW EXECUTE FUNCTION arm_reads();
			 ALTER TABLE cabal_messages ENABLE ROW LEVEL SECURITY;
			 ALTER TABLE cabal_messages FORCE ROW LEVEL SECURITY;
			 CREATE POLICY open ON cabal_messages USING (true) WITH CHECK (true);
			 CREATE POLICY broken_reads ON cabal_messages AS RESTRICTIVE FOR SELECT
			   USING (1 / (CASE WHEN current_setting('chat.reads', true) = 'broken' THEN 0 ELSE 1 END) = 1)`,
			func(t *testing.T, f chatFixture, parent app.ChatMessage) error {
				t.Helper()
				_, err := f.send(t, f.member(1), "reply", &domain.Reply{Parent: parent.ID})
				return err
			},
		},
		{
			"reply when the parent lock fails",
			`DROP TABLE cabal_messages`,
			func(t *testing.T, f chatFixture, parent app.ChatMessage) error {
				t.Helper()
				_, err := f.send(t, f.member(1), "reply", &domain.Reply{Parent: parent.ID})
				return err
			},
		},
		{
			"delete when the lock fails",
			`DROP TABLE cabal_messages`,
			func(t *testing.T, f chatFixture, parent app.ChatMessage) error {
				t.Helper()
				return f.del.Handle(f.as(t, f.member(0)), app.DeleteChatMessage{
					CabalID: f.cabal.ID, MessageID: parent.ID, Caller: f.member(0),
				})
			},
		},
		{
			"delete when the update fails",
			`ALTER TABLE cabal_messages ADD CONSTRAINT no_deletes CHECK (deleted_at IS NULL) NOT VALID`,
			func(t *testing.T, f chatFixture, parent app.ChatMessage) error {
				t.Helper()
				return f.del.Handle(f.as(t, f.member(0)), app.DeleteChatMessage{
					CabalID: f.cabal.ID, MessageID: parent.ID, Caller: f.member(0),
				})
			},
		},
		{
			"post when the event cannot be appended",
			`DROP TABLE events CASCADE`,
			func(t *testing.T, f chatFixture, _ app.ChatMessage) error {
				t.Helper()
				_, err := f.send(t, f.member(0), "gm", nil)
				return err
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			f := newChatFixture(t)
			parent := f.mustSend(t, f.member(0), "parent", nil)
			if _, err := f.pool.Exec(t.Context(), tt.breaker); err != nil {
				t.Fatal(err)
			}
			wantCode(t, tt.run(t, f, parent), errs.CodeInternal)
		})
	}
}
