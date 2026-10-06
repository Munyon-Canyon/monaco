package social_test

import (
	"reflect"
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

func (f chatFixture) unread(t *testing.T, user ids.UserID, cabals ...ids.CabalID) map[ids.CabalID]int {
	t.Helper()
	counts, err := app.NewUnread(f.pool).UnreadCounts(t.Context(), user, cabals)
	if err != nil {
		t.Fatal(err)
	}
	return counts
}

func TestUnreadCounts_CapAndThreadRepliesExcluded(t *testing.T) {
	t.Parallel()
	f := newChatFixture(t)
	top := f.mustSend(t, f.member(0), "top", nil)
	f.clock.Advance(time.Second)
	f.mustSend(t, f.member(0), "thread only", &domain.Reply{Parent: top.ID})
	f.clock.Advance(time.Second)
	f.mustSend(t, f.member(0), "thread and channel", &domain.Reply{Parent: top.ID, AlsoInChannel: true})
	f.clock.Advance(time.Second)
	f.mustSend(t, f.member(1), "mine", nil)
	gone := f.mustSend(t, f.member(0), "gone", nil)
	if err := f.del.Handle(f.as(t, f.member(0)), app.DeleteChatMessage{
		CabalID: f.cabal.ID, MessageID: gone.ID, Caller: f.member(0),
	}); err != nil {
		t.Fatal(err)
	}
	want := map[ids.CabalID]int{f.cabal.ID: 2, f.other.ID: 0}
	if got := f.unread(t, f.member(1), f.cabal.ID, f.other.ID); !reflect.DeepEqual(got, want) {
		t.Fatalf("unread = %v, want %v: the top message and the reply sent to the channel", got, want)
	}
	if _, err := f.pool.Exec(t.Context(), `INSERT INTO cabal_messages (id, cabal_id, author_id, body, created_at)
		SELECT gen_random_uuid(), $1, $2, 'gm', $3::timestamptz + n * interval '1 second'
		FROM generate_series(1, 150) AS n`, f.cabal.ID.UUID(), f.member(0).UUID(), f.now); err != nil {
		t.Fatal(err)
	}
	if got := f.unread(t, f.member(1), f.cabal.ID)[f.cabal.ID]; got != app.UnreadCap {
		t.Fatalf("unread with 150 more messages = %d, want the cap %d", got, app.UnreadCap)
	}
}

func TestUnreadCounts_followTheSeenWatermark(t *testing.T) {
	t.Parallel()
	f := newChatFixture(t)
	f.mustSend(t, f.member(0), "one", nil)
	f.clock.Advance(time.Second)
	f.mustSend(t, f.member(0), "two", nil)
	f.clock.Advance(time.Second)
	f.mustMarkSeen(t, f.member(1))
	f.clock.Advance(time.Second)
	f.mustSend(t, f.member(0), "three", nil)
	if got := f.unread(t, f.member(1), f.cabal.ID)[f.cabal.ID]; got != 1 {
		t.Fatalf("unread after the mark = %d, want 1", got)
	}
	if got := f.unread(t, f.member(2), f.cabal.ID)[f.cabal.ID]; got != 3 {
		t.Fatalf("unread for a member who never marked = %d, want 3", got)
	}
	if got := f.unread(t, f.member(1)); len(got) != 0 {
		t.Fatalf("unread for no cabals = %v, want none", got)
	}
}

func TestUnreadCounts_aDatabaseFailureIsInternal(t *testing.T) {
	t.Parallel()
	f := newChatFixture(t)
	if _, err := f.pool.Exec(t.Context(), `DROP TABLE chat_seen`); err != nil {
		t.Fatal(err)
	}
	_, err := app.NewUnread(f.pool).UnreadCounts(t.Context(), f.member(1), []ids.CabalID{f.cabal.ID})
	wantCode(t, err, errs.CodeInternal)
}
