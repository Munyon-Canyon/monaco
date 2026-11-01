package social_test

import (
	"slices"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/domain/feed"
)

func TestReactFeedItem_hintsOnlyTheCabalNotEveryClient(t *testing.T) {
	t.Parallel()
	f := newReactFixture(t)
	item, me := f.item(t, feed.KindTrade), f.cabal.Members[0].ID

	if _, err := f.add(t, item, me, "🎉"); err != nil {
		t.Fatal(err)
	}
	if _, err := f.drop(t, item, me, "🎉"); err != nil {
		t.Fatal(err)
	}

	key := events.CabalFeedItemChangedHint(f.cabal.ID)
	if got := f.hints.seen(); !slices.Equal(got, []string{key, key}) {
		t.Fatalf("hints = %v, want two %s and no global.feed", got, key)
	}
}

func TestReactComment_hintsOnlyTheCabalNotEveryClient(t *testing.T) {
	t.Parallel()
	f := newCommentReactFixture(t)
	item := f.item(t, feed.KindTrade)
	kai, lee := f.cabal.Members[0].ID, f.cabal.Members[1].ID
	reply := f.say(t, item, lee, "nice", nil)
	before := len(f.hints.seen())

	if _, err := f.add(t, reply.Id, kai, "🎉"); err != nil {
		t.Fatal(err)
	}
	if _, err := f.drop(t, reply.Id, kai, "🎉"); err != nil {
		t.Fatal(err)
	}

	key := events.CabalFeedItemChangedHint(f.cabal.ID)
	if got := f.hints.seen()[before:]; !slices.Equal(got, []string{key, key}) {
		t.Fatalf("hints = %v, want two %s and no global.feed", got, key)
	}
}

func TestCommentPoll_aVoteAndAWithdrawalHintOnlyTheCabal(t *testing.T) {
	t.Parallel()
	f := newPollThread(t)
	poll := f.ask(t, f.author, f.commentRoutes.item(t, feed.KindTrade))
	deps := app.PollDeps{
		UoW: f.uow, Reads: f.pool, Members: membersOn(f.pool), Clock: f.clock, Hints: f.hints,
	}
	ctx := asUser(t.Context(), f.nora)
	if _, err := app.NewVoteCommentPollHandler(deps).Handle(
		ctx, app.VoteCommentPoll{CommentID: poll.Id, Viewer: f.nora, Position: 1},
	); err != nil {
		t.Fatal(err)
	}
	if _, err := app.NewWithdrawCommentPollVoteHandler(deps).Handle(
		ctx, app.WithdrawCommentPollVote{CommentID: poll.Id, Viewer: f.nora},
	); err != nil {
		t.Fatal(err)
	}

	key := events.CabalFeedItemChangedHint(f.cabal.ID)
	if got := f.hints.seen(); !slices.Equal(got, []string{key, key}) {
		t.Fatalf("hints = %v, want two %s and no global.feed", got, key)
	}
}
