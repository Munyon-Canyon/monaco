package social_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/cabal"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/adapters"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/domain/feed"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
)

type poolMembers struct{ app.Members }

func membersOn(pool *pgxpool.Pool) poolMembers {
	return poolMembers{cabal.New(module.Deps{Pool: pool}).Queries()}
}

func TestCommentPoll_aMembersOnlyThreadTakesVotesOnlyFromMembers(t *testing.T) {
	t.Parallel()
	f := newPollThread(t)
	proposal := f.commentRoutes.item(t, feed.KindProposal)
	member, other := f.cabal.Members[0].ID, f.cabal.Members[1].ID
	poll := f.ask(t, member, proposal)
	_, err := f.vote(t, f.nora, poll.Id, 0)
	wantCode(t, err, errs.CodeCommentMembersOnly)
	if got := f.mustVote(t, other, poll.Id, 1); got.Total != 1 {
		t.Fatalf("poll after a member's vote = %+v, want total 1", got)
	}
	if got, err := f.withdraw(t, f.nora, poll.Id); err != nil || got.Total != 1 {
		t.Fatalf("an outsider's withdraw = %+v, %v, want the poll untouched", got, err)
	}
	var votes int
	if err := f.pool.QueryRow(t.Context(), `SELECT count(*) FROM comment_poll_votes WHERE comment_id = $1`,
		poll.Id).Scan(&votes); err != nil || votes != 1 {
		t.Fatalf("stored votes = %d, %v, want only the member's", votes, err)
	}
}

func TestReactComment_aMembersOnlyThreadTakesReactionsOnlyFromMembers(t *testing.T) {
	t.Parallel()
	f := newCommentReactFixture(t)
	proposal := f.item(t, feed.KindProposal)
	member, other := f.cabal.Members[0].ID, f.cabal.Members[1].ID
	reply := f.say(t, proposal, member, "ship it", nil)
	_, err := f.add(t, reply.Id, f.outsider, "🎉")
	wantCode(t, err, errs.CodeCommentMembersOnly)
	if _, err := f.add(t, reply.Id, other, "🎉"); err != nil {
		t.Fatal(err)
	}
	if got := f.rows(t, reply.Id); got != 1 {
		t.Fatalf("%d reaction rows, want only the member's", got)
	}
	if _, err := f.drop(t, reply.Id, f.outsider, "🎉"); err != nil {
		t.Fatalf("an outsider taking back a reaction = %v, want it allowed", err)
	}
}

func TestReactFeedItem_aMembersOnlyThreadTakesReactionsOnlyFromMembers(t *testing.T) {
	t.Parallel()
	f := newReactFixture(t)
	proposal := f.item(t, feed.KindProposal)
	outsider := f.outsider
	_, err := f.add(t, proposal, outsider, "🎉")
	wantCode(t, err, errs.CodeCommentMembersOnly)
	if _, err := f.add(t, proposal, f.cabal.Members[0].ID, "🎉"); err != nil {
		t.Fatal(err)
	}
	if got := f.held(t, proposal, outsider); got != 0 {
		t.Fatalf("outsider holds %d reactions, want 0", got)
	}
}

func TestDeletedCleanup_removesTheDeletedAccountsReactionsLikesAndPollVotesOnly(t *testing.T) {
	t.Parallel()
	f := newPollThread(t)
	kai, lee := f.cabal.Members[0].ID, f.cabal.Members[1].ID
	poll := f.ask(t, kai, f.item)
	reply := f.say(t, kai, "gm", uuid.Nil)
	for _, who := range []ids.UserID{kai, lee} {
		f.mustVote(t, who, poll.Id, 0)
		for _, q := range []string{
			`INSERT INTO feed_reactions (feed_object_id, user_id, emoji, created_at) VALUES ($1, $2, 'x', now())`,
			`INSERT INTO feed_likes (feed_object_id, user_id, created_at) VALUES ($1, $2, now())`,
		} {
			if _, err := f.pool.Exec(t.Context(), q, f.item, who.UUID()); err != nil {
				t.Fatal(err)
			}
		}
		if _, err := f.pool.Exec(t.Context(), `INSERT INTO comment_reactions
			(comment_id, feed_object_id, user_id, emoji, created_at) VALUES ($1, $2, $3, 'x', now())`,
			reply.Id, f.item, who.UUID()); err != nil {
			t.Fatal(err)
		}
	}
	ctx := observability.WithEventID(t.Context(), ids.EventIDFrom(f.gen.NewV7()))
	for range 2 {
		err := f.uow.Do(ctx, func(ctx context.Context, tx db.Tx) error {
			return adapters.DeletedCleanup{}.Handle(
				ctx,
				tx,
				events.UserDeleted{V: 1, UserID: kai.UUID()},
				f.clock.Now(),
			)
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	for table, want := range map[string]int{
		"feed_reactions": 1, "feed_likes": 1, "comment_reactions": 1, "comment_poll_votes": 1,
	} {
		var left, theirs int
		if err := f.pool.QueryRow(t.Context(),
			`SELECT count(*), count(*) FILTER (WHERE user_id = $1) FROM `+table, kai.UUID()).
			Scan(&left, &theirs); err != nil || left != want || theirs != 0 {
			t.Errorf(
				"%s: %d rows (%d of the deleted account), %v; want %d, none of theirs",
				table,
				left,
				theirs,
				err,
				want,
			)
		}
	}
}

func TestCanReact_aFailedMembershipReadAndAThreadStillArrivingAreRefused(t *testing.T) {
	t.Parallel()
	f := newCommentFixture(t)
	proposal := f.item(t, feed.KindProposal)
	boom := errs.New(errs.CodeUpstreamUnavailable, "test")
	_, err := app.CanReact(t.Context(), f.pool, failingMembers{boom}, f.outsider, proposal)
	wantCode(t, err, errs.CodeUpstreamUnavailable)
	orphan := f.closing(t, feed.KindReply, "", f.gen.NewV7())
	_, err = app.CanReact(t.Context(), f.pool, membersOn(f.pool), f.outsider, orphan)
	wantCode(t, err, errs.CodeFeedItemPending)
}

func TestDeletedCleanup_aDatabaseFailureIsInternal(t *testing.T) {
	t.Parallel()
	for _, table := range []string{"feed_reactions", "feed_likes", "comment_reactions", "comment_poll_votes"} {
		t.Run(table, func(t *testing.T) {
			t.Parallel()
			f := newCommentFixture(t)
			if _, err := f.pool.Exec(t.Context(), `DROP TABLE `+table+` CASCADE`); err != nil {
				t.Fatal(err)
			}
			ctx := observability.WithEventID(t.Context(), ids.EventIDFrom(f.gen.NewV7()))
			err := f.uow.Do(ctx, func(ctx context.Context, tx db.Tx) error {
				return adapters.DeletedCleanup{}.Handle(
					ctx,
					tx,
					events.UserDeleted{V: 1, UserID: f.gen.NewV7()},
					f.clock.Now(),
				)
			})
			wantCode(t, err, errs.CodeInternal)
		})
	}
}
