package social_test

import (
	"reflect"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/domain/feed"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

func (f commentFixture) say(t *testing.T, item uuid.UUID, author ids.UserID, body string, parent uuid.UUID) uuid.UUID {
	t.Helper()
	c := f.mustComment(t, item, author, body, parent)
	f.clock.Advance(time.Second)
	return c.ID
}

func (f commentFixture) list(t *testing.T, item uuid.UUID, limit int, after *domain.Keyset) app.CommentsPage {
	t.Helper()
	page, err := app.ListComments(
		t.Context(),
		f.pool,
		app.CommentsQuery{FeedObjectID: item, After: after, Limit: limit},
	)
	if err != nil {
		t.Fatal(err)
	}
	return page
}

func threadBodies(page app.CommentsPage) [][]string {
	out := make([][]string, len(page.Threads))
	for i, thread := range page.Threads {
		out[i] = []string{thread.Comment.Body}
		for _, reply := range thread.Replies {
			out[i] = append(out[i], reply.Body)
		}
	}
	return out
}

func TestListComments_returnsThreadsOldestFirstWithTheirRepliesOldestFirst(t *testing.T) {
	t.Parallel()
	f := newCommentFixture(t)
	item := f.item(t, feed.KindTrade)
	a, b := f.cabal.Members[0].ID, f.cabal.Members[1].ID
	first := f.say(t, item, a, "first", uuid.Nil)
	second := f.say(t, item, b, "second", uuid.Nil)
	r1 := f.say(t, item, b, "first-r1", first)
	f.say(t, item, a, "second-r1", second)
	f.say(t, item, a, "first-r2", r1)
	f.say(t, item, f.outsider, "third", uuid.Nil)
	f.say(t, f.item(t, feed.KindTrade), a, "elsewhere", uuid.Nil)
	page := f.list(t, item, 50, nil)
	want := [][]string{{"first", "first-r1", "first-r2"}, {"second", "second-r1"}, {"third"}}
	if got := threadBodies(page); !reflect.DeepEqual(got, want) || page.Next != nil {
		t.Fatalf("threads = %v, next %v, want %v and no cursor", got, page.Next, want)
	}
}

func TestListComments_pagesTopLevelCommentsWithACursor(t *testing.T) {
	t.Parallel()
	f := newCommentFixture(t)
	item := f.item(t, feed.KindTrade)
	a := f.cabal.Members[0].ID
	one := f.say(t, item, a, "one", uuid.Nil)
	f.say(t, item, a, "two", uuid.Nil)
	f.say(t, item, a, "three", uuid.Nil)
	f.say(t, item, a, "one-r1", one)
	first := f.list(t, item, 2, nil)
	if got, want := threadBodies(first), [][]string{{"one", "one-r1"}, {"two"}}; !reflect.DeepEqual(got, want) {
		t.Fatalf("first page = %v, want %v", got, want)
	}
	if first.Next == nil || first.Next.ID != first.Threads[1].Comment.ID {
		t.Fatalf("cursor = %+v, want the last thread", first.Next)
	}
	second := f.list(t, item, 2, first.Next)
	if got, want := threadBodies(second), [][]string{{"three"}}; !reflect.DeepEqual(got, want) || second.Next != nil {
		t.Fatalf("second page = %v, next %v, want %v and no cursor", got, second.Next, want)
	}
}

func TestListComments_keepsDeletedCommentsInPlace(t *testing.T) {
	t.Parallel()
	f := newCommentFixture(t)
	item := f.item(t, feed.KindTrade)
	a := f.cabal.Members[0].ID
	top := f.say(t, item, a, "top", uuid.Nil)
	f.say(t, item, f.outsider, "reply", top)
	if err := f.delete(t, top, a); err != nil {
		t.Fatal(err)
	}
	page := f.list(t, item, 50, nil)
	if len(page.Threads) != 1 || !page.Threads[0].Comment.Deleted || len(page.Threads[0].Replies) != 1 ||
		page.Threads[0].Replies[0].Deleted {
		t.Fatalf("threads = %+v, want a deleted top-level comment keeping its live reply", page.Threads)
	}
}

func TestListComments_aThreadlessItemHasNoThreads(t *testing.T) {
	t.Parallel()
	f := newCommentFixture(t)
	if page := f.list(t, f.item(t, feed.KindTrade), 50, nil); len(page.Threads) != 0 || page.Next != nil {
		t.Fatalf("page = %+v, want empty", page)
	}
}

func TestListComments_refusesABadLimitAndAnUnknownItem(t *testing.T) {
	t.Parallel()
	f := newCommentFixture(t)
	item := f.item(t, feed.KindTrade)
	for _, limit := range []int{0, app.CommentPageMax + 1} {
		_, err := app.ListComments(t.Context(), f.pool, app.CommentsQuery{FeedObjectID: item, Limit: limit})
		wantCode(t, err, errs.CodeInvalidInput)
	}
	_, err := app.ListComments(t.Context(), f.pool, app.CommentsQuery{FeedObjectID: f.gen.NewV7(), Limit: 1})
	wantCode(t, err, errs.CodeFeedItemNotFound)
}

func TestListComments_failsWithInternalWhenTheStoreFails(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name, breaker string
	}{
		{"the item lookup fails", `DROP TABLE feed_objects CASCADE`},
		{"the comment reads fail", `DROP TABLE feed_comments CASCADE`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			f := newCommentFixture(t)
			item := f.item(t, feed.KindTrade)
			if _, err := f.pool.Exec(t.Context(), tt.breaker); err != nil {
				t.Fatal(err)
			}
			_, err := app.ListComments(t.Context(), f.pool, app.CommentsQuery{FeedObjectID: item, Limit: 1})
			wantCode(t, err, errs.CodeInternal)
		})
	}
}
