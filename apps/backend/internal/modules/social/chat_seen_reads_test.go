package social_test

import (
	"reflect"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	api "github.com/monaco/monaco/apps/backend/internal/platform/httpx/api/socialapi"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

func (f chatRoutes) seenBy(t *testing.T, viewer ids.UserID, message uuid.UUID) (api.ChatSeenBy, error) {
	t.Helper()
	res, err := f.routes.GetChatSeenBy(asUser(t.Context(), viewer), api.GetChatSeenByRequestObject{
		Id: f.cabal.ID.UUID(), Params: api.GetChatSeenByParams{MessageId: message},
	})
	if err != nil {
		return api.ChatSeenBy{}, err
	}
	seen, ok := res.(api.GetChatSeenBy200JSONResponse)
	if !ok {
		t.Fatalf("response = %#v", res)
	}
	return api.ChatSeenBy(seen), nil
}

func (f chatRoutes) markAs(t *testing.T, user ids.UserID) {
	t.Helper()
	f.mustMarkSeen(t, user)
	f.clock.Advance(time.Second)
}

func TestSeenCount_ExcludesTheAuthorAndMembersBehindTheMessage(t *testing.T) {
	t.Parallel()
	f := newChatRoutes(t)
	f.markAs(t, f.member(2))
	msg := f.mustPost(t, f.member(0), api.PostChatMessageRequest{Body: "gm"})
	f.markAs(t, f.member(0))
	f.markAs(t, f.member(1))
	got, err := f.seenBy(t, f.member(2), msg.Id)
	if err != nil {
		t.Fatal(err)
	}
	want := api.ChatSeenBy{Count: 1, Members: []api.ChatSeenMember{
		{UserId: f.member(1).UUID(), Handle: nil, DisplayName: "", PhotoUrl: nil},
	}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("seen by = %+v, want %+v: the author and a watermark older than the message do not count", got, want)
	}
}

func TestSeenBy_listsNewestWatermarkFirstWithNames(t *testing.T) {
	t.Parallel()
	f := newChatRoutes(t)
	msg := f.mustPost(t, f.member(2), api.PostChatMessageRequest{Body: "gm"})
	f.markAs(t, f.member(1))
	f.markAs(t, f.member(0))
	got, err := f.seenBy(t, f.member(1), msg.Id)
	if err != nil {
		t.Fatal(err)
	}
	want := api.ChatSeenBy{Count: 2, Members: []api.ChatSeenMember{
		{
			UserId: f.member(0).UUID(), Handle: ptr("kai"), DisplayName: "Kai",
			PhotoUrl: ptr("https://cdn.example.com/kai.jpg"),
		},
		{UserId: f.member(1).UUID()},
	}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("seen by = %+v, want %+v: newest watermark first, a deleted account without a name", got, want)
	}
}

func TestSeenBy_errors(t *testing.T) {
	t.Parallel()
	f := newChatRoutes(t)
	msg := f.mustPost(t, f.member(0), api.PostChatMessageRequest{Body: "gm"})
	if _, err := f.seenBy(t, f.outsider, msg.Id); errs.CodeOf(err) != errs.CodeNotCabalMember {
		t.Fatalf("seen by for an outsider = %v, want not_cabal_member", err)
	}
	if _, err := f.seenBy(t, f.member(1), testkit.NewIDs(7).NewV7()); errs.CodeOf(err) != errs.CodeChatMessageNotFound {
		t.Fatalf("seen by for an unknown message = %v, want chat_message_not_found", err)
	}
	if _, err := f.routes.GetChatSeenBy(t.Context(), api.GetChatSeenByRequestObject{}); err == nil {
		t.Fatal("unauthenticated read succeeded")
	}
	f.users.Fail("UsersByID", errs.New(errs.CodeUpstreamUnavailable, "test"))
	f.mustMarkSeen(t, f.member(1))
	if _, err := f.seenBy(t, f.member(1), msg.Id); errs.CodeOf(err) != errs.CodeUpstreamUnavailable {
		t.Fatalf("seen by when identity is down = %v, want upstream_unavailable", err)
	}
}

func TestChatChannel_seenCountIsOnlyOnTheNewestMessageOfAFreshRead(t *testing.T) {
	t.Parallel()
	f := newChatRoutes(t)
	older := f.mustPost(t, f.member(0), api.PostChatMessageRequest{Body: "one"})
	newest := f.mustPost(t, f.member(0), api.PostChatMessageRequest{Body: "two"})
	f.markAs(t, f.member(1))
	f.markAs(t, f.member(2))
	counts := func(p api.GetChatMessagesParams) map[uuid.UUID]*int {
		t.Helper()
		page, err := f.channel(t, f.member(0), p)
		if err != nil {
			t.Fatal(err)
		}
		out := map[uuid.UUID]*int{}
		for _, m := range page {
			out[m.Id] = m.SeenCount
		}
		return out
	}
	tests := []struct {
		name string
		page api.GetChatMessagesParams
		want map[uuid.UUID]*int
	}{
		{"a fresh read", api.GetChatMessagesParams{}, map[uuid.UUID]*int{newest.Id: ptr(2), older.Id: nil}},
		{"a before page", api.GetChatMessagesParams{Before: &newest.Id}, map[uuid.UUID]*int{older.Id: nil}},
		{"an after page", api.GetChatMessagesParams{After: &older.Id}, map[uuid.UUID]*int{newest.Id: ptr(2)}},
	}
	for _, tt := range tests {
		if got := counts(tt.page); !reflect.DeepEqual(got, tt.want) {
			t.Fatalf("seen_count on %s = %v, want %v", tt.name, got, tt.want)
		}
	}
}

func TestChatChannel_seenCountSkipsADeletedNewestMessageAndAnEmptyChannel(t *testing.T) {
	t.Parallel()
	f := newChatRoutes(t)
	if page, err := f.channel(t, f.member(0), api.GetChatMessagesParams{}); err != nil || len(page) != 0 {
		t.Fatalf("empty channel = %v, %v, want no messages", page, err)
	}
	kept := f.mustPost(t, f.member(0), api.PostChatMessageRequest{Body: "kept"})
	gone := f.mustPost(t, f.member(0), api.PostChatMessageRequest{Body: "gone"})
	if err := f.remove(t, f.member(0), gone.Id); err != nil {
		t.Fatal(err)
	}
	f.markAs(t, f.member(1))
	page, err := f.channel(t, f.member(0), api.GetChatMessagesParams{})
	if err != nil || len(page) != 1 || page[0].Id != kept.Id || page[0].SeenCount == nil || *page[0].SeenCount != 1 {
		t.Fatalf("channel = %+v, %v, want the kept message carrying seen_count 1", page, err)
	}
}

func TestSeenReads_databaseFailures(t *testing.T) {
	t.Parallel()
	seenBy := func(t *testing.T, f chatRoutes, msg uuid.UUID) error {
		t.Helper()
		_, err := f.seenBy(t, f.member(1), msg)
		return err
	}
	channel := func(t *testing.T, f chatRoutes, _ uuid.UUID) error {
		t.Helper()
		_, err := f.channel(t, f.member(1), api.GetChatMessagesParams{})
		return err
	}
	tests := []struct {
		name, breaker string
		run           func(*testing.T, chatRoutes, uuid.UUID) error
	}{
		{"seen by when the message lookup fails", `DROP TABLE cabal_messages CASCADE`, seenBy},
		{"seen by when the list fails", `DROP TABLE chat_seen`, seenBy},
		{"channel when the seen count fails", `DROP TABLE chat_seen`, channel},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			f := newChatRoutes(t)
			msg := f.mustPost(t, f.member(0), api.PostChatMessageRequest{Body: "gm"})
			if _, err := f.pool.Exec(t.Context(), tt.breaker); err != nil {
				t.Fatal(err)
			}
			wantCode(t, tt.run(t, f, msg.Id), errs.CodeInternal)
		})
	}
}
