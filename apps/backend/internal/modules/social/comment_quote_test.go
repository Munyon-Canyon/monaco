package social_test

import (
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/modules/social/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/domain/feed"
	api "github.com/monaco/monaco/apps/backend/internal/platform/httpx/api/socialapi"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

type quoteThread struct {
	commentRoutes
	item   uuid.UUID
	author ids.UserID
	gone   ids.UserID
	nora   ids.UserID
}

func newQuoteThread(t *testing.T) quoteThread {
	t.Helper()
	f := newCommentRoutes(t)
	return quoteThread{
		commentRoutes: f, item: f.item(t, feed.KindTrade),
		author: f.cabal.Members[0].ID, gone: f.cabal.Members[1].ID, nora: f.outsider,
	}
}

func (f quoteThread) say(t *testing.T, who ids.UserID, body string, answering uuid.UUID) api.Comment {
	t.Helper()
	req := &api.CreateCommentRequest{Body: &body}
	if answering != uuid.Nil {
		req.ParentCommentId = &answering
	}
	c, err := f.post(t, f.item, who, req)
	if err != nil {
		t.Fatal(err)
	}
	f.clock.Advance(time.Second)
	return c
}

func (f quoteThread) page(t *testing.T, viewer ids.UserID) api.CommentPage {
	t.Helper()
	limit := 50
	res, err := f.routes.GetFeedComments(asUser(t.Context(), viewer), api.GetFeedCommentsRequestObject{
		Id: f.item, Params: api.GetFeedCommentsParams{Limit: &limit},
	})
	if err != nil {
		t.Fatal(err)
	}
	return api.CommentPage(res.(api.GetFeedComments200JSONResponse))
}

func TestCommentQuote_theAuthorsAnswerFoldsUnderTheRoot(t *testing.T) {
	t.Parallel()
	f := newQuoteThread(t)
	why := f.say(t, f.nora, "Why sell here?", uuid.Nil)
	answer := f.say(t, f.author, "Took the gain while it was there.", why.Id)
	if answer.ParentCommentId == nil || *answer.ParentCommentId != why.Id ||
		answer.RootCommentId == nil || *answer.RootCommentId != why.Id {
		t.Fatalf("answer = %+v, want folded under %s", answer, why.Id)
	}
	want := api.CommentQuote{
		CommentId: why.Id, RootCommentId: why.Id, AuthorDisplayName: "Mia", Excerpt: ptr("Why sell here?"),
	}
	if answer.ReplyTo == nil || !reflect.DeepEqual(*answer.ReplyTo, want) {
		t.Fatalf("quote = %+v, want %+v", answer.ReplyTo, want)
	}
	page := f.page(t, f.nora)
	if len(page.Items) != 1 || len(page.Items[0].Replies) != 1 || page.Items[0].Replies[0].Id != answer.Id ||
		page.Items[0].Replies[0].ReplyTo == nil || !reflect.DeepEqual(*page.Items[0].Replies[0].ReplyTo, want) {
		t.Fatalf("page = %+v, want the answer inside the root's replies with its quote", page)
	}
}

func TestCommentQuote_aReplyToAFoldedAnswerIsTopLevelAndQuotesIt(t *testing.T) {
	t.Parallel()
	f := newQuoteThread(t)
	why := f.say(t, f.nora, "Why sell here?", uuid.Nil)
	answer := f.say(t, f.author, "Took the gain while it was there.", why.Id)
	gif := f.say(t, f.gone, "Respect.", answer.Id)
	if gif.ParentCommentId != nil || gif.RootCommentId != nil {
		t.Fatalf("reply = %+v, want top-level", gif)
	}
	want := api.CommentQuote{
		CommentId: answer.Id, RootCommentId: why.Id, AuthorDisplayName: "Kai",
		Excerpt: ptr("Took the gain while it was there."), IsFolded: true,
	}
	if gif.ReplyTo == nil || !reflect.DeepEqual(*gif.ReplyTo, want) {
		t.Fatalf("quote = %+v, want %+v", gif.ReplyTo, want)
	}
	page := f.page(t, f.nora)
	if len(page.Items) != 2 || page.Items[1].Comment.Id != gif.Id || len(page.Items[1].Replies) != 0 ||
		page.Items[1].Comment.ReplyTo == nil || !reflect.DeepEqual(*page.Items[1].Comment.ReplyTo, want) {
		t.Fatalf("page = %+v, want the reply second at the top level with its quote", page)
	}
}

func TestCommentQuote_aReplyToATopLevelCommentByAnotherMemberQuotesItAsItsOwnRoot(t *testing.T) {
	t.Parallel()
	f := newQuoteThread(t)
	good := f.say(t, f.author, strings.Repeat("é", domain.CommentExcerptMax+10), uuid.Nil)
	called := f.say(t, f.nora, "He called the top again.", good.Id)
	if called.ParentCommentId != nil {
		t.Fatalf("reply = %+v, want top-level", called)
	}
	want := api.CommentQuote{
		CommentId: good.Id, RootCommentId: good.Id, AuthorDisplayName: "Kai",
		Excerpt: ptr(strings.Repeat("é", domain.CommentExcerptMax)),
	}
	if called.ReplyTo == nil || !reflect.DeepEqual(*called.ReplyTo, want) {
		t.Fatalf("quote = %+v, want %+v", called.ReplyTo, want)
	}
}

func TestCommentQuote_aCommentThatAnswersNoneHasNoQuote(t *testing.T) {
	t.Parallel()
	f := newQuoteThread(t)
	top := f.say(t, f.nora, "first", uuid.Nil)
	if top.ReplyTo != nil || top.RootCommentId != nil {
		t.Fatalf("top-level comment = %+v, want no quote and no root", top)
	}
	if got := f.page(t, f.nora).Items[0].Comment; got.ReplyTo != nil {
		t.Fatalf("listed comment = %+v, want no quote", got)
	}
}

func TestCommentQuote_aDeletedCommentOrAuthorIsQuotedWithoutText(t *testing.T) {
	t.Parallel()
	f := newQuoteThread(t)
	deleted := f.say(t, f.nora, "regret this", uuid.Nil)
	byGone := f.say(t, f.gone, "from a deleted account", uuid.Nil)
	f.say(t, f.author, "to the deleted comment", deleted.Id)
	f.say(t, f.author, "to the deleted account", byGone.Id)
	if err := f.delete(t, deleted.Id, f.nora); err != nil {
		t.Fatal(err)
	}
	items := f.page(t, f.author).Items
	if len(items) != 2 {
		t.Fatalf("items = %+v, want the two top-level comments", items)
	}
	quote := items[0].Replies[0].ReplyTo
	if quote == nil || !quote.IsDeleted || quote.Excerpt != nil || quote.CommentId != deleted.Id {
		t.Fatalf("quote of a deleted comment = %+v, want is_deleted and no excerpt", quote)
	}
	quote = items[1].Replies[0].ReplyTo
	if quote == nil || quote.AuthorDisplayName != "Someone" || quote.Excerpt == nil {
		t.Fatalf("quote of a deleted account = %+v, want the name Someone", quote)
	}
}

func TestCommentQuote_aBlockedAuthorsTextIsWithheld(t *testing.T) {
	t.Parallel()
	f := newQuoteThread(t)
	byGone := f.say(t, f.gone, "blocked text", uuid.Nil)
	reply := f.say(t, f.nora, "agreed", byGone.Id)
	insertBlock(t, f.pool, f.gen.NewV7(), f.author, f.gone)
	items := f.page(t, f.author).Items
	if len(items) != 1 || items[0].Comment.Id != reply.Id {
		t.Fatalf("items = %+v, want only the reply", items)
	}
	if quote := items[0].Comment.ReplyTo; quote == nil || quote.Excerpt != nil || quote.CommentId != byGone.Id {
		t.Fatalf("quote = %+v, want the blocked author's text withheld", quote)
	}
	quote := f.say(t, f.author, "mine", reply.Id).ReplyTo
	if quote == nil || quote.Excerpt == nil || *quote.Excerpt != "agreed" {
		t.Fatalf("quote of an unblocked author = %+v, want its text", quote)
	}
}
