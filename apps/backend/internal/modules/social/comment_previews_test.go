package social_test

import (
	"context"
	"reflect"
	"testing"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/domain/feed"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/port"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/testkit/feedtest"
)

func (f commentFixture) previews(
	t *testing.T, viewer ids.UserID, comments ...uuid.UUID,
) map[uuid.UUID]port.CommentPreview {
	t.Helper()
	got, err := app.NewCommentPreviews(f.pool, f.users).CommentPreviews(t.Context(), viewer, comments)
	if err != nil {
		t.Fatal(err)
	}
	return got
}

func (f commentFixture) proposalThread(t *testing.T, proposal uuid.UUID) uuid.UUID {
	t.Helper()
	return feedtest.Item(t, f.pool, f.clock, f.gen, func(it *feed.Item) {
		it.Kind, it.RefID, it.CabalID, it.ActorID = feed.KindProposal, proposal, f.cabal.ID, f.cabal.Creator.ID
		it.Payload = feed.Payload{CabalName: "Alpha Cabal", Symbol: "AAPLx", AssetName: "Apple", Action: feed.ActionBuy}
	})
}

func TestCommentPreviews_returnsTheCommentAndTheOneItAnswered(t *testing.T) {
	t.Parallel()
	f := newCommentFixture(t)
	item := f.item(t, feed.KindTrade)
	kai, lee := f.cabal.Members[0].ID, f.cabal.Members[1].ID
	top := f.say(t, item, kai, "Deliveries miss is priced in.", uuid.Nil)
	reply := f.say(t, item, lee, "Not so sure", top)
	f.say(t, item, kai, "unrelated", uuid.Nil)

	got := f.previews(t, kai, reply)

	if len(got) != 2 {
		t.Fatalf("previews = %d, want the reply and the comment it answers", len(got))
	}
	wantPreview(t, got[top], port.CommentPreview{
		FeedItemID: item, CommentID: top, RootCommentID: top,
		Author: port.CommentAuthor{ID: kai, Name: "Kai"}, Body: "Deliveries miss is priced in.",
	})
	wantPreview(t, got[reply], port.CommentPreview{
		FeedItemID: item, CommentID: reply, ReplyToID: &top, RootCommentID: reply,
		Author: port.CommentAuthor{ID: lee, Name: "lee"}, Body: "Not so sure",
	})
}

func TestCommentPreviews_aThreadAuthorsAnswerFoldsUnderItsRoot(t *testing.T) {
	t.Parallel()
	f := newCommentFixture(t)
	item := f.item(t, feed.KindTrade)
	kai, author := f.cabal.Members[0].ID, f.cabal.Creator.ID
	top := f.say(t, item, kai, "top", uuid.Nil)
	answer := f.say(t, item, author, "answer", top)

	got := f.previews(t, kai, answer)

	if got[answer].ParentCommentID == nil || *got[answer].ParentCommentID != top ||
		got[answer].ReplyToID == nil || *got[answer].ReplyToID != top || got[answer].RootCommentID != top {
		t.Fatalf("answer = %+v, want it folded under %v and answering it", got[answer], top)
	}
}

func wantPreview(t *testing.T, got, want port.CommentPreview) {
	t.Helper()
	if got.CreatedAt.IsZero() {
		t.Fatalf("preview %v has no creation time", got.CommentID)
	}
	want.CreatedAt = got.CreatedAt
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("preview = %+v, want %+v", got, want)
	}
}

func TestCommentPreviews_hidesDeletedRemovedAndBlockedAuthors(t *testing.T) {
	t.Parallel()
	f := newCommentFixture(t)
	item := f.item(t, feed.KindTrade)
	kai, lee := f.cabal.Members[0].ID, f.cabal.Members[1].ID
	kept := f.say(t, item, kai, "kept", uuid.Nil)
	gone := f.say(t, item, kai, "gone", uuid.Nil)
	removed := f.say(t, item, kai, "removed", uuid.Nil)
	blocked := f.say(t, item, lee, "blocked", uuid.Nil)
	if err := f.delete(t, gone, kai); err != nil {
		t.Fatal(err)
	}
	if _, err := f.pool.Exec(t.Context(),
		`UPDATE feed_comments SET deleted_at = $2, deleted_by_kind = 'admin' WHERE id = $1`,
		removed, f.clock.Now()); err != nil {
		t.Fatal(err)
	}
	insertBlock(t, f.pool, f.gen.NewV7(), kai, lee)

	got := f.previews(t, kai, kept, gone, removed, blocked)

	if len(got) != 1 || got[kept].Body != "kept" {
		t.Fatalf("previews = %+v, want only the kept comment", got)
	}
	if again := f.previews(t, f.outsider, blocked); again[blocked].Body != "blocked" {
		t.Fatalf("a viewer who blocked nobody lost the comment: %+v", again)
	}
}

func TestCommentPreviews_aReplyToAReplyReadsThatReplyAsTheOneItAnswered(t *testing.T) {
	t.Parallel()
	f := newCommentFixture(t)
	item := f.item(t, feed.KindTrade)
	kai, lee := f.cabal.Members[0].ID, f.cabal.Members[1].ID
	top := f.say(t, item, kai, "top", uuid.Nil)
	mid := f.say(t, item, lee, "mid", top)
	deep := f.say(t, item, kai, "deep", mid)

	got := f.previews(t, kai, deep)

	if len(got) != 2 || got[mid].Body != "mid" {
		t.Fatalf("previews = %+v, want the reply and the reply it answers", got)
	}
	if got[deep].ReplyToID == nil || *got[deep].ReplyToID != mid {
		t.Fatalf("deep answers %v, want %v", got[deep].ReplyToID, mid)
	}
}

func TestCommentPreviews_followsAChainOfParentsToItsRoot(t *testing.T) {
	t.Parallel()
	f := newCommentFixture(t)
	item := f.item(t, feed.KindTrade)
	kai := f.cabal.Members[0].ID
	top := f.say(t, item, kai, "top", uuid.Nil)
	mid, deep := f.gen.NewV7(), f.gen.NewV7()
	for _, row := range [][2]uuid.UUID{{mid, top}, {deep, mid}} {
		if _, err := f.pool.Exec(t.Context(),
			`INSERT INTO feed_comments (id, feed_object_id, author_id, parent_comment_id, body, created_at)
			 VALUES ($1, $2, $3, $4, 'chain', $5)`,
			row[0], item, kai.UUID(), row[1], f.clock.Now()); err != nil {
			t.Fatal(err)
		}
	}

	got := f.previews(t, kai, deep)

	if got[deep].RootCommentID != top {
		t.Fatalf("root = %v, want %v", got[deep].RootCommentID, top)
	}
}

func TestCommentPreviews_namesTheProposalAndCabalOnAProposalThread(t *testing.T) {
	t.Parallel()
	f := newCommentFixture(t)
	proposal := f.gen.NewV7()
	item := f.proposalThread(t, proposal)
	trade := f.item(t, feed.KindTrade)
	kai := f.cabal.Members[0].ID
	onProposal := f.say(t, item, kai, "on the proposal", uuid.Nil)
	onTrade := f.say(t, trade, kai, "on the trade", uuid.Nil)

	got := f.previews(t, kai, onProposal, onTrade)

	p := got[onProposal]
	if p.ProposalID == nil || p.ProposalID.UUID() != proposal || p.CabalID == nil || *p.CabalID != f.cabal.ID {
		t.Fatalf("proposal preview = %+v", p)
	}
	if o := got[onTrade]; o.ProposalID != nil || o.CabalID != nil {
		t.Fatalf("trade preview = %+v, want no proposal or cabal", o)
	}
}

type failingUsers struct{ app.Users }

func (failingUsers) UsersByID(context.Context, []ids.UserID) (map[ids.UserID]app.UserCard, error) {
	return nil, errs.New(errs.CodeDBUnavailable, "test.UsersByID")
}

func TestCommentPreviews_failsWhenTheCommentsCannotBeRead(t *testing.T) {
	t.Parallel()
	f := newCommentFixture(t)
	item := f.item(t, feed.KindTrade)
	id := f.say(t, item, f.cabal.Members[0].ID, "hello", uuid.Nil)

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	got, err := app.NewCommentPreviews(f.pool, f.users).
		CommentPreviews(ctx, f.cabal.Members[0].ID, []uuid.UUID{id})

	wantCode(t, err, errs.CodeInternal)
	if got != nil {
		t.Fatalf("previews = %+v, want none", got)
	}
}

func TestCommentPreviews_failsWhenTheAuthorsCannotBeRead(t *testing.T) {
	t.Parallel()
	f := newCommentFixture(t)
	item := f.item(t, feed.KindTrade)
	id := f.say(t, item, f.cabal.Members[0].ID, "hello", uuid.Nil)

	got, err := app.NewCommentPreviews(f.pool, failingUsers{f.users}).
		CommentPreviews(t.Context(), f.cabal.Members[0].ID, []uuid.UUID{id})

	wantCode(t, err, errs.CodeDBUnavailable)
	if got != nil {
		t.Fatalf("previews = %+v, want none", got)
	}
}

func TestCommentPreviews_answersNothingForNoComments(t *testing.T) {
	t.Parallel()
	f := newCommentFixture(t)

	if got := f.previews(t, f.cabal.Members[0].ID); len(got) != 0 {
		t.Fatalf("previews = %+v, want none", got)
	}
}
