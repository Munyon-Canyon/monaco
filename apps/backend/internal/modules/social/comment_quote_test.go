package social_test

import (
	"encoding/json"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/modules/social/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/domain/feed"
	api "github.com/monaco/monaco/apps/backend/internal/platform/httpx/api/socialapi"
)

func replyingTo(id uuid.UUID) *uuid.UUID { return &id }

func TestCommentQuote_theAuthorsAnswerFoldsUnderTheRoot(t *testing.T) {
	t.Parallel()
	f := newCommentRoutes(t)
	item := f.item(t, feed.KindTrade)
	author := f.cabal.Members[0].ID
	top := f.say(t, item, f.outsider, "Why sell here?", nil)
	answer := f.say(t, item, author, "Took the gain while it was there.", replyingTo(top.Id))
	page := f.read(t, f.outsider, item, api.GetFeedCommentsParams{})
	if len(page.Items) != 1 || len(page.Items[0].Replies) != 1 || page.Items[0].Replies[0].Id != answer.Id {
		t.Fatalf("page = %+v, want the answer in the replies of %s", page, top.Id)
	}
	got := page.Items[0].Replies[0]
	want := &api.CommentQuote{
		CommentId: top.Id, RootCommentId: top.Id, AuthorDisplayName: "Mia", Excerpt: ptr("Why sell here?"),
	}
	if got.RootCommentId == nil || *got.RootCommentId != top.Id || !reflect.DeepEqual(got.ReplyTo, want) {
		t.Fatalf("answer = %+v, want root %s and quote %+v", got, top.Id, want)
	}
}

func TestCommentQuote_anotherMembersReplyToAFoldedAnswerIsTopLevel(t *testing.T) {
	t.Parallel()
	f := newCommentRoutes(t)
	item := f.item(t, feed.KindTrade)
	author := f.cabal.Members[0].ID
	top := f.say(t, item, f.outsider, "Why sell here?", nil)
	answer := f.say(t, item, author, "Took the gain.", replyingTo(top.Id))
	reply := f.say(t, item, f.cabal.Members[1].ID, "Same.", replyingTo(answer.Id))
	page := f.read(t, f.outsider, item, api.GetFeedCommentsParams{})
	if len(page.Items) != 2 || page.Items[1].Comment.Id != reply.Id || len(page.Items[0].Replies) != 1 {
		t.Fatalf("page = %+v, want the reply as the second top-level comment", page)
	}
	got := page.Items[1].Comment
	want := &api.CommentQuote{
		CommentId: answer.Id, RootCommentId: top.Id, AuthorDisplayName: "Kai", Excerpt: ptr("Took the gain."),
		IsFolded: true,
	}
	if got.ParentCommentId != nil || got.RootCommentId != nil || !reflect.DeepEqual(got.ReplyTo, want) {
		t.Fatalf("reply = %+v, want top level and quote %+v", got, want)
	}
}

func TestCommentQuote_aNonAuthorsReplyToATopLevelCommentQuotesIt(t *testing.T) {
	t.Parallel()
	f := newCommentRoutes(t)
	item := f.item(t, feed.KindTrade)
	author := f.cabal.Members[0].ID
	top := f.say(t, item, author, "Good timing on this one.", nil)
	reply := f.say(t, item, f.outsider, "He called the top again.", replyingTo(top.Id))
	page := f.read(t, author, item, api.GetFeedCommentsParams{})
	if len(page.Items) != 2 || len(page.Items[0].Replies) != 0 || page.Items[1].Comment.Id != reply.Id {
		t.Fatalf("page = %+v, want the reply at the top level, not under %s", page, top.Id)
	}
	want := &api.CommentQuote{
		CommentId: top.Id, RootCommentId: top.Id, AuthorDisplayName: "Kai", Excerpt: ptr("Good timing on this one."),
	}
	if got := page.Items[1].Comment.ReplyTo; !reflect.DeepEqual(got, want) {
		t.Fatalf("quote = %+v, want %+v", got, want)
	}
}

func TestCommentQuote_aQuoteOfADeletedCommentCarriesNoText(t *testing.T) {
	t.Parallel()
	f := newCommentRoutes(t)
	item := f.item(t, feed.KindTrade)
	top := f.say(t, item, f.outsider, "something regrettable", nil)
	f.say(t, item, f.cabal.Members[1].ID, "agreed", replyingTo(top.Id))
	if err := f.delete(t, top.Id, f.outsider); err != nil {
		t.Fatal(err)
	}
	page := f.read(t, f.cabal.Members[0].ID, item, api.GetFeedCommentsParams{})
	got := page.Items[1].Comment.ReplyTo
	if got == nil || !got.IsDeleted || got.Excerpt != nil || got.CommentId != top.Id || got.AuthorDisplayName != "Mia" {
		t.Fatalf("quote = %+v, want a deleted quote of %s with no excerpt", got, top.Id)
	}
}

func TestCommentQuote_aBlockedAuthorsTextIsWithheld(t *testing.T) {
	t.Parallel()
	f := newCommentRoutes(t)
	item := f.item(t, feed.KindTrade)
	viewer, third := f.cabal.Members[0].ID, f.cabal.Members[1].ID
	top := f.say(t, item, f.outsider, "private opinion", nil)
	f.say(t, item, third, "noted", replyingTo(top.Id))
	insertBlock(t, f.pool, f.gen.NewV7(), viewer, f.outsider)
	page := f.read(t, viewer, item, api.GetFeedCommentsParams{})
	if len(page.Items) != 1 {
		t.Fatalf("page = %+v, want only the reply, the blocked author's thread being hidden", page)
	}
	got := page.Items[0].Comment.ReplyTo
	if got == nil || got.Excerpt != nil || got.IsDeleted || got.CommentId != top.Id {
		t.Fatalf("quote = %+v, want a quote of %s with the excerpt withheld", got, top.Id)
	}
	raw, err := json.Marshal(page)
	if err != nil || strings.Contains(string(raw), "private opinion") {
		t.Fatalf("page leaks the blocked author's text: %s, %v", raw, err)
	}
}

func TestCommentQuote_cutsTheExcerptAt120Scalars(t *testing.T) {
	t.Parallel()
	f := newCommentRoutes(t)
	item := f.item(t, feed.KindTrade)
	top := f.say(t, item, f.outsider, strings.Repeat("é", domain.CommentExcerptMax+30), nil)
	reply, err := f.post(
		t,
		item,
		f.cabal.Members[1].ID,
		&api.CreateCommentRequest{Body: "ok", ParentCommentId: &top.Id},
	)
	if err != nil {
		t.Fatal(err)
	}
	if reply.ReplyTo == nil || reply.ReplyTo.Excerpt == nil ||
		len([]rune(*reply.ReplyTo.Excerpt)) != domain.CommentExcerptMax {
		t.Fatalf("quote = %+v, want an excerpt of %d scalars", reply.ReplyTo, domain.CommentExcerptMax)
	}
}

func TestCommentQuote_theMigrationBackfillsExistingRepliesWithTheirParent(t *testing.T) {
	t.Parallel()
	f := newCommentFixture(t)
	item := f.item(t, feed.KindTrade)
	top := f.mustComment(t, item, f.outsider, "top", uuid.Nil)
	reply := f.mustComment(t, item, f.cabal.Members[0].ID, "reply", top.ID)
	if _, err := f.pool.Exec(t.Context(), `UPDATE feed_comments SET reply_to_comment_id = NULL`); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile("../../../migrations/20261029045313_social_comment_reply_to.sql")
	if err != nil {
		t.Fatal(err)
	}
	var backfill string
	for _, statement := range strings.Split(string(raw), ";") {
		if strings.Contains(statement, "UPDATE feed_comments") {
			backfill = statement
		}
	}
	if backfill == "" {
		t.Fatal("the migration has no backfill statement")
	}
	if _, err := f.pool.Exec(t.Context(), backfill); err != nil {
		t.Fatal(err)
	}
	var topTo, replyTo *uuid.UUID
	if err := f.pool.QueryRow(t.Context(),
		`SELECT (SELECT reply_to_comment_id FROM feed_comments WHERE id = $1),
		        (SELECT reply_to_comment_id FROM feed_comments WHERE id = $2)`, top.ID, reply.ID,
	).Scan(&topTo, &replyTo); err != nil {
		t.Fatal(err)
	}
	if topTo != nil || replyTo == nil || *replyTo != top.ID {
		t.Fatalf("reply_to_comment_id = %v on the top-level comment and %v on the reply, want nil and %s",
			topTo, replyTo, top.ID)
	}
}
