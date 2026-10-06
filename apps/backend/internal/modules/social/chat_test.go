package social_test

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/cabal"
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
	post     *app.PostChatMessageHandler
	del      *app.DeleteChatMessageHandler
	cabal    testkit.SeededCabal
	other    testkit.SeededCabal
	outsider ids.UserID
}

func newChatFixture(t *testing.T) chatFixture {
	t.Helper()
	pool := testkit.DB(t)
	g := testkit.NewIDs(591)
	now := clock.Real{}.Now().UTC().Truncate(time.Microsecond)
	clk := testkit.NewClock(now)
	uow := db.New(pool, g, clk)
	deps := app.ChatDeps{UoW: uow, Members: cabal.New(module.Deps{Pool: pool}).Queries(), IDs: g, Clock: clk}
	return chatFixture{
		now: now, pool: pool, clock: clk, deps: deps,
		post: app.NewPostChatMessageHandler(deps), del: app.NewDeleteChatMessageHandler(uow, clk),
		cabal: testkit.NewCabal(t, pool, testkit.WithMembers(3)), other: testkit.NewCabal(t, pool),
		outsider: testkit.SeedUser(t, pool, testkit.UserOpts{}).ID,
	}
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
