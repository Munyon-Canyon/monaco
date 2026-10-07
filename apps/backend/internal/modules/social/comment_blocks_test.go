package social_test

import (
	"reflect"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/modules/social/domain/feed"
	api "github.com/monaco/monaco/apps/backend/internal/platform/httpx/api/socialapi"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

type blockedComments struct {
	commentRoutes
	feedItem                 uuid.UUID
	blocker, blocked, third  ids.UserID
	byBlocked, byThird, mine uuid.UUID
}

func (f blockedComments) sayAsThird(t *testing.T, body string, parent uuid.UUID) uuid.UUID {
	t.Helper()
	id := f.gen.NewV7()
	var parentID any
	if parent != uuid.Nil {
		parentID = parent
	}
	if _, err := f.pool.Exec(t.Context(),
		`INSERT INTO feed_comments (id, feed_object_id, author_id, parent_comment_id, body, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6)`, id, f.feedItem, f.third.UUID(), parentID, body, f.clock.Now()); err != nil {
		t.Fatal(err)
	}
	f.clock.Advance(time.Second)
	return id
}

func newBlockedComments(t *testing.T) blockedComments {
	t.Helper()
	f := blockedComments{commentRoutes: newCommentRoutes(t)}
	f.blocker, f.blocked, f.third = f.cabal.Members[0].ID, f.cabal.Members[1].ID, f.outsider
	f.feedItem = f.item(t, feed.KindTrade)
	f.byBlocked = f.commentFixture.say(t, f.feedItem, f.blocked, "top by the blocked", uuid.Nil)
	f.commentFixture.say(t, f.feedItem, f.blocker, "reply under the blocked", f.byBlocked)
	f.sayAsThird(t, "third under the blocked", f.byBlocked)
	f.byThird = f.sayAsThird(t, "top by the third", uuid.Nil)
	f.commentFixture.say(t, f.feedItem, f.blocked, "blocked reply under the third", f.byThird)
	f.commentFixture.say(t, f.feedItem, f.blocker, "blocker reply under the third", f.byThird)
	f.mine = f.commentFixture.say(t, f.feedItem, f.blocker, "top by the blocker", uuid.Nil)
	insertBlock(t, f.pool, f.gen.NewV7(), f.blocker, f.blocked)
	return f
}

func (f blockedComments) page(t *testing.T, viewer ids.UserID, limit int, cursor *string) api.CommentPage {
	t.Helper()
	res, err := f.routes.GetFeedComments(asUser(t.Context(), viewer), api.GetFeedCommentsRequestObject{
		Id: f.feedItem, Params: api.GetFeedCommentsParams{Limit: &limit, Cursor: cursor},
	})
	if err != nil {
		t.Fatal(err)
	}
	return api.CommentPage(res.(api.GetFeedComments200JSONResponse))
}

func bodiesOf(page api.CommentPage) [][]string {
	out := make([][]string, len(page.Items))
	for i, thread := range page.Items {
		out[i] = []string{thread.Comment.BodyDisplay}
		for _, reply := range thread.Replies {
			out[i] = append(out[i], reply.BodyDisplay)
		}
	}
	return out
}

func TestComments_HidesBlockedAuthor(t *testing.T) {
	t.Parallel()
	f := newBlockedComments(t)
	got := bodiesOf(f.page(t, f.blocker, 50, nil))
	want := [][]string{
		{"top by the third", "blocker reply under the third"},
		{"top by the blocker"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("the blocker reads %q, want %q", got, want)
	}
	got = bodiesOf(f.page(t, f.third, 50, nil))
	want = [][]string{
		{"top by the blocked", "reply under the blocked", "third under the blocked"},
		{"top by the third", "blocked reply under the third", "blocker reply under the third"},
		{"top by the blocker"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("a third member reads %q, want %q", got, want)
	}
}

func TestComments_PagesExactlyPastHiddenThreads(t *testing.T) {
	t.Parallel()
	f := newBlockedComments(t)
	first := f.page(t, f.blocker, 1, nil)
	if got := bodiesOf(first); len(got) != 1 || got[0][0] != "top by the third" || first.NextCursor == nil {
		t.Fatalf("first page = %q, cursor %v; want the third's thread and a cursor", got, first.NextCursor)
	}
	second := f.page(t, f.blocker, 1, first.NextCursor)
	if got := bodiesOf(second); len(got) != 1 || got[0][0] != "top by the blocker" || second.NextCursor != nil {
		t.Fatalf("second page = %q, cursor %v; want the blocker's thread and no cursor", got, second.NextCursor)
	}
}
