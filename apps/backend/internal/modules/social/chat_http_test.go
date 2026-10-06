package social_test

import (
	"encoding/json"
	"reflect"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity"
	"github.com/monaco/monaco/apps/backend/internal/modules/social"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/adapters"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/app"
	api "github.com/monaco/monaco/apps/backend/internal/platform/httpx/api/socialapi"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
	"github.com/monaco/monaco/apps/backend/internal/testkit/fakes"
)

type chatRoutes struct {
	chatFixture
	users  *fakes.Identity
	routes adapters.HTTP
}

func newChatRoutes(t *testing.T) chatRoutes {
	t.Helper()
	f := chatRoutes{chatFixture: newChatFixture(t)}
	f.users = fakes.NewIdentity([]identity.UserCard{
		{ID: f.member(0), Handle: "kai", DisplayName: "Kai", PhotoURL: "https://cdn.example.com/kai.jpg"},
		{ID: f.member(1), Handle: "gone", DisplayName: "Gone", Deleted: true},
	}, nil)
	deps := module.Deps{Pool: f.pool, UoW: f.deps.UoW, IDs: f.deps.IDs, Clock: f.clock}
	f.routes = social.HTTPOf(social.New(deps, social.WithUsers(f.users), social.WithRealtime(f.rt)))
	return f
}

func (f chatRoutes) postAs(
	t *testing.T, author ids.UserID, body api.PostChatMessageRequest,
) (api.ChatMessage, error) {
	t.Helper()
	res, err := f.routes.PostChatMessage(asUser(t.Context(), author), api.PostChatMessageRequestObject{
		Id: f.cabal.ID.UUID(), Body: &body,
	})
	if err != nil {
		return api.ChatMessage{}, err
	}
	created, ok := res.(api.PostChatMessage201JSONResponse)
	if !ok {
		t.Fatalf("response = %#v", res)
	}
	return api.ChatMessage(created), nil
}

func (f chatRoutes) mustPost(t *testing.T, author ids.UserID, body api.PostChatMessageRequest) api.ChatMessage {
	t.Helper()
	m, err := f.postAs(t, author, body)
	if err != nil {
		t.Fatal(err)
	}
	f.clock.Advance(time.Second)
	return m
}

func (f chatRoutes) channel(t *testing.T, viewer ids.UserID, p api.GetChatMessagesParams) ([]api.ChatMessage, error) {
	t.Helper()
	res, err := f.routes.GetChatMessages(asUser(t.Context(), viewer), api.GetChatMessagesRequestObject{
		Id: f.cabal.ID.UUID(), Params: p,
	})
	if err != nil {
		return nil, err
	}
	page, ok := res.(api.GetChatMessages200JSONResponse)
	if !ok {
		t.Fatalf("response = %#v", res)
	}
	return page.Messages, nil
}

func (f chatRoutes) thread(
	t *testing.T, viewer ids.UserID, parent uuid.UUID, p api.GetChatThreadParams,
) (api.ChatThread, error) {
	t.Helper()
	res, err := f.routes.GetChatThread(asUser(t.Context(), viewer), api.GetChatThreadRequestObject{
		Id: f.cabal.ID.UUID(), MessageId: parent, Params: p,
	})
	if err != nil {
		return api.ChatThread{}, err
	}
	th, ok := res.(api.GetChatThread200JSONResponse)
	if !ok {
		t.Fatalf("response = %#v", res)
	}
	return api.ChatThread(th), nil
}

func (f chatRoutes) remove(t *testing.T, caller ids.UserID, message uuid.UUID) error {
	t.Helper()
	res, err := f.routes.DeleteChatMessage(asUser(t.Context(), caller), api.DeleteChatMessageRequestObject{
		Id: f.cabal.ID.UUID(), MessageId: message,
	})
	if err == nil {
		if _, ok := res.(api.DeleteChatMessage204Response); !ok {
			t.Fatalf("response = %#v", res)
		}
	}
	return err
}

func messageIDs(messages []api.ChatMessage) []uuid.UUID {
	out := make([]uuid.UUID, len(messages))
	for i, m := range messages {
		out[i] = m.Id
	}
	return out
}

func wantIDs(t *testing.T, got []api.ChatMessage, want ...api.ChatMessage) {
	t.Helper()
	if g, w := messageIDs(got), messageIDs(want); !reflect.DeepEqual(g, w) {
		t.Fatalf("messages = %v, want %v", g, w)
	}
}

func TestChatRoutes_postRendersTheStoredMessageWithItsAuthor(t *testing.T) {
	t.Parallel()
	f := newChatRoutes(t)
	got := f.mustPost(t, f.member(0), api.PostChatMessageRequest{Body: " gm "})
	want := api.ChatMessage{
		Id: got.Id, Body: ptr("gm"), CreatedAt: f.now,
		Author: api.ChatAuthor{
			Id: f.member(0).UUID(), Handle: ptr("kai"), DisplayName: "Kai",
			PhotoUrl: ptr("https://cdn.example.com/kai.jpg"),
		},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("posted = %+v, want %+v", got, want)
	}
	reply := f.mustPost(t, f.member(2), api.PostChatMessageRequest{
		Body: "wagmi", ParentId: &got.Id, AlsoInChannel: ptr(true),
	})
	if reply.ParentId == nil || *reply.ParentId != got.Id || !reply.AlsoInChannel {
		t.Fatalf("reply = %+v, want a reply to %s shown in the channel", reply, got.Id)
	}
	if a := reply.Author; a.Id != f.member(2).UUID() || a.Handle != nil || a.DisplayName != "" {
		t.Fatalf("author with no card = %+v, want an empty name", a)
	}
}

func (f chatRoutes) postThread(t *testing.T) (top, quiet, loud api.ChatMessage) {
	t.Helper()
	top = f.mustPost(t, f.member(0), api.PostChatMessageRequest{Body: "buy AAPLx?"})
	quiet = f.mustPost(t, f.member(1), api.PostChatMessageRequest{Body: "yes", ParentId: &top.Id})
	loud = f.mustPost(t, f.member(2), api.PostChatMessageRequest{
		Body: "no", ParentId: &top.Id, AlsoInChannel: ptr(true),
	})
	return top, quiet, loud
}

func TestChatRoutes_channelShowsOnlyRepliesSentToTheChannel(t *testing.T) {
	t.Parallel()
	f := newChatRoutes(t)
	top, _, loud := f.postThread(t)
	channel, err := f.channel(t, f.member(1), api.GetChatMessagesParams{})
	if err != nil {
		t.Fatal(err)
	}
	wantIDs(t, channel, loud, top)
	gotTop := channel[1]
	if gotTop.ReplyCount != 2 || gotTop.LastReplyAt == nil || !gotTop.LastReplyAt.Equal(loud.CreatedAt) {
		t.Fatalf("top in channel = %+v, want 2 replies, the last at %s", gotTop, loud.CreatedAt)
	}
	if a := channel[0].Author; a.Handle != nil || a.DisplayName != "" || a.Id != f.member(2).UUID() {
		t.Fatalf("author with no card = %+v, want an empty name", a)
	}
}

func TestChatRoutes_threadPagesEveryReplyOldestFirst(t *testing.T) {
	t.Parallel()
	f := newChatRoutes(t)
	top, quiet, loud := f.postThread(t)
	th, err := f.thread(t, f.member(2), top.Id, api.GetChatThreadParams{})
	if err != nil {
		t.Fatal(err)
	}
	if th.Parent.Id != top.Id {
		t.Fatalf("thread parent = %s, want %s", th.Parent.Id, top.Id)
	}
	wantIDs(t, th.Replies, quiet, loud)
	if a := th.Replies[0].Author; a.Handle != nil || a.DisplayName != "" || a.PhotoUrl != nil {
		t.Fatalf("deleted author = %+v, want an empty name", a)
	}
	older, err := f.thread(t, f.member(2), top.Id, api.GetChatThreadParams{Before: &loud.Id, Limit: ptr(5)})
	if err != nil {
		t.Fatal(err)
	}
	wantIDs(t, older.Replies, quiet)
	last, err := f.thread(t, f.member(2), top.Id, api.GetChatThreadParams{Limit: ptr(1)})
	if err != nil {
		t.Fatal(err)
	}
	wantIDs(t, last.Replies, loud)
	if _, err := f.thread(t, f.member(0), loud.Id, api.GetChatThreadParams{}); errs.CodeOf(err) !=
		errs.CodeChatParentIsReply {
		t.Fatalf("thread of a reply: err = %v, want chat_parent_is_reply", err)
	}
}

func TestChatChannel_DeletedParentPlaceholder(t *testing.T) {
	t.Parallel()
	f := newChatRoutes(t)
	parent := f.mustPost(t, f.member(0), api.PostChatMessageRequest{Body: "thread starter"})
	reply := f.mustPost(t, f.member(1), api.PostChatMessageRequest{Body: "reply", ParentId: &parent.Id})
	lonely := f.mustPost(t, f.member(0), api.PostChatMessageRequest{Body: "no replies"})
	echoed := f.mustPost(t, f.member(1), api.PostChatMessageRequest{
		Body: "echo", ParentId: &parent.Id, AlsoInChannel: ptr(true),
	})
	for _, m := range []api.ChatMessage{parent, lonely} {
		if err := f.remove(t, f.member(0), m.Id); err != nil {
			t.Fatal(err)
		}
	}
	if err := f.remove(t, f.member(1), echoed.Id); err != nil {
		t.Fatal(err)
	}
	channel, err := f.channel(t, f.member(2), api.GetChatMessagesParams{})
	if err != nil {
		t.Fatal(err)
	}
	wantIDs(t, channel, parent)
	if p := channel[0]; !p.Deleted || p.Body != nil || p.ReplyCount != 2 {
		t.Fatalf("placeholder = %+v, want deleted with no body and 2 replies", p)
	}
	th, err := f.thread(t, f.member(2), parent.Id, api.GetChatThreadParams{})
	if err != nil {
		t.Fatal(err)
	}
	if !th.Parent.Deleted || th.Parent.Body != nil {
		t.Fatalf("thread parent = %+v, want the placeholder", th.Parent)
	}
	wantIDs(t, th.Replies, reply)
	if _, err := f.thread(t, f.member(2), lonely.Id, api.GetChatThreadParams{}); errs.CodeOf(err) !=
		errs.CodeChatMessageNotFound {
		t.Fatalf("thread of a deleted message with no replies: err = %v, want chat_message_not_found", err)
	}
}

func TestChatChannel_AfterCursorCatchUp(t *testing.T) {
	t.Parallel()
	f := newChatRoutes(t)
	sent := make([]api.ChatMessage, 0, 4)
	for _, body := range []string{"one", "two", "three", "four"} {
		sent = append(sent, f.mustPost(t, f.member(0), api.PostChatMessageRequest{Body: body}))
	}
	after, err := f.channel(t, f.member(1), api.GetChatMessagesParams{After: &sent[1].Id})
	if err != nil {
		t.Fatal(err)
	}
	wantIDs(t, after, sent[2], sent[3])
	firstAfter, err := f.channel(t, f.member(1), api.GetChatMessagesParams{After: &sent[0].Id, Limit: ptr(1)})
	if err != nil {
		t.Fatal(err)
	}
	wantIDs(t, firstAfter, sent[1])
	before, err := f.channel(t, f.member(1), api.GetChatMessagesParams{Before: &sent[2].Id, Limit: ptr(1)})
	if err != nil {
		t.Fatal(err)
	}
	wantIDs(t, before, sent[1])
	newest, err := f.channel(t, f.member(1), api.GetChatMessagesParams{Limit: ptr(2)})
	if err != nil {
		t.Fatal(err)
	}
	wantIDs(t, newest, sent[3], sent[2])
}

func TestChatReads_refuseBadRequests(t *testing.T) {
	t.Parallel()
	f := newChatRoutes(t)
	m := f.mustPost(t, f.member(0), api.PostChatMessageRequest{Body: "gm"})
	elsewhere, err := f.post.Handle(f.as(t, f.other.Creator.ID), app.PostChatMessage{
		CabalID: f.other.ID, Author: f.other.Creator.ID, Body: "elsewhere",
	})
	if err != nil {
		t.Fatal(err)
	}
	unknown := ids.Real{}.NewV7()
	tests := []struct {
		name   string
		viewer ids.UserID
		p      api.GetChatMessagesParams
		want   errs.Code
	}{
		{"outsider", f.outsider, api.GetChatMessagesParams{}, errs.CodeNotCabalMember},
		{"limit 0", f.member(0), api.GetChatMessagesParams{Limit: ptr(0)}, errs.CodeInvalidInput},
		{"limit 101", f.member(0), api.GetChatMessagesParams{Limit: ptr(101)}, errs.CodeInvalidInput},
		{
			"before and after", f.member(0),
			api.GetChatMessagesParams{Before: &m.Id, After: &m.Id},
			errs.CodeInvalidInput,
		},
		{"unknown before", f.member(0), api.GetChatMessagesParams{Before: &unknown}, errs.CodeChatMessageNotFound},
		{"unknown after", f.member(0), api.GetChatMessagesParams{After: &unknown}, errs.CodeChatMessageNotFound},
		{
			"cursor in another cabal", f.member(0),
			api.GetChatMessagesParams{Before: &elsewhere.ID},
			errs.CodeChatMessageNotFound,
		},
	}
	for _, tt := range tests {
		if _, err := f.channel(t, tt.viewer, tt.p); errs.CodeOf(err) != tt.want {
			t.Errorf("%s: err = %v, want %s", tt.name, err, tt.want)
		}
	}
	threads := []struct {
		name   string
		viewer ids.UserID
		parent uuid.UUID
		p      api.GetChatThreadParams
		want   errs.Code
	}{
		{"outsider", f.outsider, m.Id, api.GetChatThreadParams{}, errs.CodeNotCabalMember},
		{"limit 101", f.member(0), m.Id, api.GetChatThreadParams{Limit: ptr(101)}, errs.CodeInvalidInput},
		{"unknown parent", f.member(0), unknown, api.GetChatThreadParams{}, errs.CodeChatMessageNotFound},
		{"parent in another cabal", f.member(0), elsewhere.ID, api.GetChatThreadParams{}, errs.CodeChatMessageNotFound},
		{
			"unknown before", f.member(0), m.Id,
			api.GetChatThreadParams{Before: &unknown},
			errs.CodeChatMessageNotFound,
		},
	}
	for _, tt := range threads {
		if _, err := f.thread(t, tt.viewer, tt.parent, tt.p); errs.CodeOf(err) != tt.want {
			t.Errorf("thread %s: err = %v, want %s", tt.name, err, tt.want)
		}
	}
}

func TestPostChatMessage_refusesABadRequestBeforeTheCommand(t *testing.T) {
	t.Parallel()
	f := newChatRoutes(t)
	tests := []struct {
		name string
		body *api.PostChatMessageRequest
		want errs.Code
	}{
		{"no body", nil, errs.CodeInvalidInput},
		{"blank text", &api.PostChatMessageRequest{Body: "  "}, errs.CodeChatBodyInvalid},
		{
			"also in channel without a parent",
			&api.PostChatMessageRequest{Body: "gm", AlsoInChannel: ptr(true)},
			errs.CodeInvalidInput,
		},
	}
	for _, tt := range tests {
		_, err := f.routes.PostChatMessage(asUser(t.Context(), f.member(0)), api.PostChatMessageRequestObject{
			Id: f.cabal.ID.UUID(), Body: tt.body,
		})
		if errs.CodeOf(err) != tt.want {
			t.Errorf("%s: err = %v, want %s", tt.name, err, tt.want)
		}
	}
	if _, err := f.postAs(t, f.outsider, api.PostChatMessageRequest{Body: "gm"}); errs.CodeOf(err) !=
		errs.CodeNotCabalMember {
		t.Fatalf("outsider: err = %v, want not_cabal_member", err)
	}
	if n := f.count(t, `SELECT count(*) FROM cabal_messages`); n != 0 {
		t.Fatalf("messages = %d, want 0", n)
	}
}

func TestChatRoutes_requireASignedInUser(t *testing.T) {
	t.Parallel()
	f := newChatRoutes(t)
	ctx := t.Context()
	id := f.cabal.ID.UUID()
	_, postErr := f.routes.PostChatMessage(ctx, api.PostChatMessageRequestObject{Id: id})
	_, listErr := f.routes.GetChatMessages(ctx, api.GetChatMessagesRequestObject{Id: id})
	_, threadErr := f.routes.GetChatThread(ctx, api.GetChatThreadRequestObject{Id: id})
	_, deleteErr := f.routes.DeleteChatMessage(ctx, api.DeleteChatMessageRequestObject{Id: id})
	for _, err := range []error{postErr, listErr, threadErr, deleteErr} {
		if errs.CodeOf(err) != errs.CodeUnauthorized {
			t.Errorf("err = %v, want unauthorized", err)
		}
	}
}

func TestChatRoutes_failWhenADependencyFails(t *testing.T) {
	t.Parallel()
	f := newChatRoutes(t)
	m := f.mustPost(t, f.member(0), api.PostChatMessageRequest{Body: "gm"})
	if err := f.remove(t, f.member(1), m.Id); errs.CodeOf(err) != errs.CodeChatMessageNotOwned {
		t.Fatalf("delete by another member: err = %v, want chat_message_not_owned", err)
	}
	f.users.Fail("UsersByID", errs.New(errs.CodeUpstreamUnavailable, "test"))
	if _, err := f.postAs(t, f.member(0), api.PostChatMessageRequest{Body: "gm"}); errs.CodeOf(err) !=
		errs.CodeUpstreamUnavailable {
		t.Fatalf("post: err = %v, want the author lookup failure", err)
	}
	if _, err := f.channel(t, f.member(0), api.GetChatMessagesParams{}); errs.CodeOf(err) !=
		errs.CodeUpstreamUnavailable {
		t.Fatalf("channel: err = %v, want the author lookup failure", err)
	}
	if _, err := f.thread(t, f.member(0), m.Id, api.GetChatThreadParams{}); errs.CodeOf(err) !=
		errs.CodeUpstreamUnavailable {
		t.Fatalf("thread: err = %v, want the author lookup failure", err)
	}
}

func TestChatReads_failWithInternalWhenTheStoreFails(t *testing.T) {
	t.Parallel()
	const (
		renameCount  = `ALTER TABLE cabal_messages RENAME COLUMN reply_count TO replies`
		renameCreate = `ALTER TABLE cabal_messages RENAME COLUMN created_at TO at`
		textParent   = `ALTER TABLE cabal_messages DROP CONSTRAINT cabal_messages_cabal_id_parent_id_fkey;
			ALTER TABLE cabal_messages ALTER COLUMN parent_id TYPE text`
	)
	channel := func(p func(m api.ChatMessage) api.GetChatMessagesParams) func(*testing.T, chatRoutes, api.ChatMessage) error {
		return func(t *testing.T, f chatRoutes, m api.ChatMessage) error {
			t.Helper()
			_, err := f.channel(t, f.member(0), p(m))
			return err
		}
	}
	thread := func(t *testing.T, f chatRoutes, m api.ChatMessage) error {
		t.Helper()
		_, err := f.thread(t, f.member(0), m.Id, api.GetChatThreadParams{})
		return err
	}
	tests := []struct {
		name    string
		breaker string
		read    func(*testing.T, chatRoutes, api.ChatMessage) error
	}{
		{"channel", renameCount, channel(func(api.ChatMessage) api.GetChatMessagesParams {
			return api.GetChatMessagesParams{}
		})},
		{"channel after", renameCount, channel(func(m api.ChatMessage) api.GetChatMessagesParams {
			return api.GetChatMessagesParams{After: &m.Id}
		})},
		{"cursor", renameCreate, channel(func(m api.ChatMessage) api.GetChatMessagesParams {
			return api.GetChatMessagesParams{Before: &m.Id}
		})},
		{"thread parent", renameCount, thread},
		{"thread replies", textParent, thread},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			f := newChatRoutes(t)
			m := f.mustPost(t, f.member(0), api.PostChatMessageRequest{Body: "gm"})
			if _, err := f.pool.Exec(t.Context(), tt.breaker); err != nil {
				t.Fatal(err)
			}
			wantCode(t, tt.read(t, f, m), errs.CodeInternal)
		})
	}
}

func TestChatRoutes_deleteSoftDeletesTheCallersMessage(t *testing.T) {
	t.Parallel()
	f := newChatRoutes(t)
	m := f.mustPost(t, f.member(0), api.PostChatMessageRequest{Body: "gm"})
	for range 2 {
		if err := f.remove(t, f.member(0), m.Id); err != nil {
			t.Fatal(err)
		}
	}
	if n := f.count(t, `SELECT count(*) FROM cabal_messages WHERE id = $1 AND deleted_at IS NOT NULL`, m.Id); n != 1 {
		t.Fatalf("deleted rows = %d, want 1", n)
	}
	if err := f.remove(t, f.member(0), ids.Real{}.NewV7()); errs.CodeOf(err) != errs.CodeChatMessageNotFound {
		t.Fatalf("unknown message: err = %v, want chat_message_not_found", err)
	}
}

func TestChatRoutes_publishedMessagesAreTheSameJSONAsTheChannelRows(t *testing.T) {
	t.Parallel()
	f := newChatRoutes(t)
	top := f.mustPost(t, f.member(0), api.PostChatMessageRequest{Body: "gm"})
	assertPublishedIsRow := func(published testkit.RealtimePublish, id uuid.UUID) {
		t.Helper()
		var got api.ChatMessage
		if err := json.Unmarshal(published.Data, &got); err != nil {
			t.Fatal(err)
		}
		rows, err := f.channel(t, f.member(0), api.GetChatMessagesParams{})
		if err != nil {
			t.Fatal(err)
		}
		for _, row := range rows {
			row.SeenCount = nil
			if row.Id == id && reflect.DeepEqual(got, row) {
				return
			}
		}
		t.Fatalf("published %+v is not a channel row in %+v", got, rows)
	}
	assertPublishedIsRow(f.rt.Published()[0], top.Id)
	reply := f.mustPost(t, f.member(1), api.PostChatMessageRequest{
		Body: "wagmi", ParentId: &top.Id, AlsoInChannel: ptr(true),
	})
	published := f.rt.Published()
	if len(published) != 3 || published[2].Name != app.EventThreadUpdated {
		t.Fatalf("published = %+v, want top, reply and thread update", published)
	}
	assertPublishedIsRow(published[1], reply.Id)
}

func TestChatRoutes_deleteAnnouncesTheDeletion(t *testing.T) {
	t.Parallel()
	f := newChatRoutes(t)
	m := f.mustPost(t, f.member(0), api.PostChatMessageRequest{Body: "oops"})
	if err := f.remove(t, f.member(0), m.Id); err != nil {
		t.Fatal(err)
	}
	published := f.rt.Published()
	if len(published) != 2 || published[1].Name != app.EventMessageDeleted ||
		string(published[1].Data) != `{"id":"`+m.Id.String()+`"}` {
		t.Fatalf("published = %+v, want the post then message.deleted for %s", published, m.Id)
	}
}
