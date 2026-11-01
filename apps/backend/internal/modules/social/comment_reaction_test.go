package social_test

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/social"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/domain/feed"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	baseapi "github.com/monaco/monaco/apps/backend/internal/platform/httpx/api"
	api "github.com/monaco/monaco/apps/backend/internal/platform/httpx/api/socialapi"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
)

type commentReactFixture struct {
	commentRoutes
	react   *app.ReactCommentHandler
	unreact *app.UnreactCommentHandler
}

func newCommentReactFixture(t *testing.T) commentReactFixture {
	t.Helper()
	f := commentReactFixture{commentRoutes: newCommentRoutes(t)}
	deps := app.ReactionDeps{UoW: f.uow, Reads: f.pool, Members: membersOn(f.pool), Clock: f.clock, Hints: f.hints}
	f.react, f.unreact = app.NewReactCommentHandler(deps), app.NewUnreactCommentHandler(deps)
	return f
}

func (f commentReactFixture) add(
	t *testing.T, comment uuid.UUID, viewer ids.UserID, emoji string,
) (app.ReactionSummary, error) {
	t.Helper()
	return f.react.Handle(
		asUser(t.Context(), viewer),
		app.ReactComment{CommentID: comment, Viewer: viewer, Emoji: emoji},
	)
}

func (f commentReactFixture) drop(
	t *testing.T, comment uuid.UUID, viewer ids.UserID, emoji string,
) (app.ReactionSummary, error) {
	t.Helper()
	return f.unreact.Handle(
		asUser(t.Context(), viewer), app.UnreactComment{CommentID: comment, Viewer: viewer, Emoji: emoji})
}

func (f commentReactFixture) rows(t *testing.T, comment uuid.UUID) int {
	t.Helper()
	var n int
	if err := f.pool.QueryRow(t.Context(),
		`SELECT count(*) FROM comment_reactions WHERE comment_id = $1`, comment).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func (f commentReactFixture) count(t *testing.T, query string, args ...any) int {
	t.Helper()
	var n int
	if err := f.pool.QueryRow(t.Context(), query, args...).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestReactComment_storesOneRowAndAnEventNamingTheCommentAndNoNotification(t *testing.T) {
	t.Parallel()
	f := newCommentReactFixture(t)
	item := f.item(t, feed.KindTrade)
	kai, lee := f.cabal.Members[0].ID, f.cabal.Members[1].ID
	reply := f.say(t, item, lee, "nice", nil)
	for range 2 {
		got, err := f.add(t, reply.Id, kai, "🎉")
		if err != nil {
			t.Fatal(err)
		}
		wantSummary(t, got, 1, app.Reaction{Emoji: "🎉", Count: 1, Mine: true})
	}
	if got := f.rows(t, reply.Id); got != 1 {
		t.Fatalf("%d rows after two identical reactions, want 1", got)
	}
	if got := f.count(t,
		`SELECT count(*) FROM events WHERE type = $1 AND payload->>'comment_id' = $2`,
		string(events.TypeReactionAdded), reply.Id.String()); got != 1 {
		t.Fatalf("%d reaction.added events naming the comment, want 1", got)
	}
	if got := f.count(t, `SELECT count(*) FROM notifications`); got != 0 {
		t.Fatalf("%d notifications, want 0", got)
	}
	if got := f.count(t, `SELECT count(*) FROM feed_reactions`); got != 0 {
		t.Fatalf("%d feed reactions, want 0", got)
	}
}

func TestReactComment_refusesTheEleventhEmojiButNotARepeat(t *testing.T) {
	t.Parallel()
	f := newCommentReactFixture(t)
	kai := f.cabal.Members[0].ID
	reply := f.say(t, f.item(t, feed.KindTrade), kai, "nice", nil)
	for _, emoji := range []string{"😀", "😂", "😍", "😎", "😭", "😡", "🤔", "👍", "👀", "🎉"} {
		if _, err := f.add(t, reply.Id, kai, emoji); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := f.add(t, reply.Id, kai, "🎉"); err != nil {
		t.Fatalf("repeating a held emoji at the cap: %v", err)
	}
	_, err := f.add(t, reply.Id, kai, "🔥")
	wantCode(t, err, errs.CodeInvalidInput)
	if got := f.rows(t, reply.Id); got != 10 {
		t.Fatalf("%d rows after the refused 11th, want 10", got)
	}
}

func TestReactComment_refusesBadEmojiAnUnknownOrDeletedCommentAndAnInvisibleItem(t *testing.T) {
	t.Parallel()
	f := newCommentReactFixture(t)
	item := f.item(t, feed.KindTrade)
	kai, lee := f.cabal.Members[0].ID, f.cabal.Members[1].ID
	reply := f.say(t, item, lee, "nice", nil)
	_, err := f.add(t, reply.Id, kai, "nope")
	wantCode(t, err, errs.CodeInvalidInput)
	_, err = f.drop(t, reply.Id, kai, "nope")
	wantCode(t, err, errs.CodeInvalidInput)
	_, err = f.add(t, f.gen.NewV7(), kai, "🎉")
	wantCode(t, err, errs.CodeCommentNotFound)
	_, err = f.drop(t, f.gen.NewV7(), kai, "🎉")
	wantCode(t, err, errs.CodeCommentNotFound)
	if _, err := f.pool.Exec(t.Context(),
		`INSERT INTO feed_mutes (user_id, target_type, target_id, created_at) VALUES ($1, 'item', $2, $3)`,
		f.outsider.UUID(), item.String(), f.clock.Now()); err != nil {
		t.Fatal(err)
	}
	_, err = f.add(t, reply.Id, f.outsider, "🎉")
	wantCode(t, err, errs.CodeFeedItemNotFound)
	_, err = f.drop(t, reply.Id, f.outsider, "🎉")
	wantCode(t, err, errs.CodeFeedItemNotFound)
}

func TestReactComment_refusesADeletedAndARemovedComment(t *testing.T) {
	t.Parallel()
	f := newCommentReactFixture(t)
	item := f.item(t, feed.KindTrade)
	kai, lee := f.cabal.Members[0].ID, f.cabal.Members[1].ID
	own := f.say(t, item, lee, "taken back", nil)
	removed := f.say(t, item, lee, "against the rules", nil)
	if err := f.delete(t, own.Id, lee); err != nil {
		t.Fatal(err)
	}
	if err := f.removeAs(asAdmin(t), removed.Id, "abuse"); err != nil {
		t.Fatal(err)
	}
	for _, id := range []uuid.UUID{own.Id, removed.Id} {
		_, err := f.add(t, id, kai, "🎉")
		wantCode(t, err, errs.CodeCommentNotFound)
		_, err = f.drop(t, id, kai, "🎉")
		wantCode(t, err, errs.CodeCommentNotFound)
	}
}

func TestUnreactComment_removesOnlyTheCallersEmojiAndAnEventOnlyWhenARowWent(t *testing.T) {
	t.Parallel()
	f := newCommentReactFixture(t)
	kai, lee := f.cabal.Members[0].ID, f.cabal.Members[1].ID
	reply := f.say(t, f.item(t, feed.KindTrade), lee, "nice", nil)
	for _, r := range []struct {
		user  ids.UserID
		emoji string
	}{{kai, "🎉"}, {kai, "👀"}, {lee, "🎉"}} {
		if _, err := f.add(t, reply.Id, r.user, r.emoji); err != nil {
			t.Fatal(err)
		}
	}
	got, err := f.drop(t, reply.Id, kai, "🎉")
	if err != nil {
		t.Fatal(err)
	}
	wantSummary(t, got, 2, app.Reaction{Emoji: "👀", Count: 1, Mine: true}, app.Reaction{Emoji: "🎉", Count: 1})
	if _, err := f.drop(t, reply.Id, kai, "🎉"); err != nil {
		t.Fatal(err)
	}
	if got := f.count(t,
		`SELECT count(*) FROM events WHERE type = $1 AND payload->>'comment_id' = $2`,
		string(events.TypeReactionRemoved), reply.Id.String()); got != 1 {
		t.Fatalf("%d reaction.removed events after two removals of one row, want 1", got)
	}
}

func TestDeleteAndRemoveComment_clearTheCommentsReactions(t *testing.T) {
	t.Parallel()
	f := newCommentReactFixture(t)
	item := f.item(t, feed.KindTrade)
	kai, lee := f.cabal.Members[0].ID, f.cabal.Members[1].ID
	own := f.say(t, item, lee, "taken back", nil)
	removed := f.say(t, item, lee, "against the rules", nil)
	kept := f.say(t, item, lee, "kept", nil)
	for _, id := range []uuid.UUID{own.Id, removed.Id, kept.Id} {
		if _, err := f.add(t, id, kai, "🎉"); err != nil {
			t.Fatal(err)
		}
	}
	if err := f.delete(t, own.Id, lee); err != nil {
		t.Fatal(err)
	}
	if err := f.removeAs(asAdmin(t), removed.Id, "abuse"); err != nil {
		t.Fatal(err)
	}
	if f.rows(t, own.Id) != 0 || f.rows(t, removed.Id) != 0 || f.rows(t, kept.Id) != 1 {
		t.Fatalf("rows after delete and removal = %d %d %d, want 0 0 1",
			f.rows(t, own.Id), f.rows(t, removed.Id), f.rows(t, kept.Id))
	}
	page := f.read(t, kai, item, api.GetFeedCommentsParams{})
	for _, thread := range page.Items {
		if thread.Comment.Id != kept.Id && (len(thread.Comment.Reactions) != 0 || thread.Comment.ReactionCount != 0) {
			t.Fatalf("a deleted comment shows %+v", thread.Comment)
		}
	}
}

func (f commentReactFixture) commentsByID(t *testing.T, viewer ids.UserID, item uuid.UUID) map[uuid.UUID]api.Comment {
	t.Helper()
	byID := map[uuid.UUID]api.Comment{}
	for _, thread := range f.read(t, viewer, item, api.GetFeedCommentsParams{}).Items {
		byID[thread.Comment.Id] = thread.Comment
		for _, reply := range thread.Replies {
			byID[reply.Id] = reply
		}
	}
	return byID
}

func TestGetFeedComments_carriesReactionsForTopsAndAnswersWithMine(t *testing.T) {
	t.Parallel()
	f := newCommentReactFixture(t)
	item := f.item(t, feed.KindTrade)
	kai, lee := f.cabal.Members[0].ID, f.cabal.Members[1].ID
	top := f.say(t, item, lee, "top", nil)
	answer := f.say(t, item, kai, "answer", &top.Id)
	plain := f.say(t, item, lee, "plain", nil)
	for _, r := range []struct {
		comment uuid.UUID
		user    ids.UserID
		emoji   string
	}{{top.Id, kai, "❤️"}, {top.Id, lee, "😂"}, {top.Id, kai, "😂"}, {answer.Id, lee, "👀"}} {
		if _, err := f.add(t, r.comment, r.user, r.emoji); err != nil {
			t.Fatal(err)
		}
	}
	byID := f.commentsByID(t, kai, item)
	wantTop := []baseapi.Reaction{{Emoji: "😂", Count: 2, Mine: true}, {Emoji: "❤️", Count: 1, Mine: true}}
	if got := byID[top.Id]; !reflect.DeepEqual(got.Reactions, wantTop) || got.ReactionCount != 3 {
		t.Fatalf("top = %+v %d, want %+v and 3", got.Reactions, got.ReactionCount, wantTop)
	}
	if got := byID[answer.Id]; len(got.Reactions) != 1 || got.Reactions[0].Mine || got.ReactionCount != 1 {
		t.Fatalf("answer = %+v %d, want one reaction not mine", got.Reactions, got.ReactionCount)
	}
	if got := byID[plain.Id]; got.Reactions == nil || len(got.Reactions) != 0 || got.ReactionCount != 0 {
		t.Fatalf("plain = %#v %d, want an empty list and 0", got.Reactions, got.ReactionCount)
	}
}

func TestCommentReactionRoutes_throughTheHandlers(t *testing.T) {
	t.Parallel()
	f := newCommentReactFixture(t)
	kai, lee := f.cabal.Members[0].ID, f.cabal.Members[1].ID
	reply := f.say(t, f.item(t, feed.KindTrade), lee, "nice", nil)
	ctx := asUser(t.Context(), kai)
	put, err := f.routes.PutCommentReaction(ctx, api.PutCommentReactionRequestObject{CommentId: reply.Id, Emoji: "🎉"})
	if err != nil {
		t.Fatal(err)
	}
	if got := put.(api.PutCommentReaction200JSONResponse); got.Total != 1 || len(got.Reactions) != 1 {
		t.Fatalf("put = %+v, want one reaction", got)
	}
	del, err := f.routes.DeleteCommentReaction(
		ctx,
		api.DeleteCommentReactionRequestObject{CommentId: reply.Id, Emoji: "🎉"},
	)
	if err != nil {
		t.Fatal(err)
	}
	if got := del.(api.DeleteCommentReaction200JSONResponse); got.Total != 0 || len(got.Reactions) != 0 {
		t.Fatalf("delete = %+v, want none", got)
	}
	missing := f.gen.NewV7()
	if _, err := f.routes.PutCommentReaction(ctx, api.PutCommentReactionRequestObject{
		CommentId: missing, Emoji: "🎉",
	}); errs.CodeOf(err) != errs.CodeCommentNotFound {
		t.Fatalf("put on a missing comment err = %v", err)
	}
	if _, err := f.routes.DeleteCommentReaction(ctx, api.DeleteCommentReactionRequestObject{
		CommentId: missing, Emoji: "🎉",
	}); errs.CodeOf(err) != errs.CodeCommentNotFound {
		t.Fatalf("delete on a missing comment err = %v", err)
	}
	anon := t.Context()
	if _, err := f.routes.PutCommentReaction(anon, api.PutCommentReactionRequestObject{
		CommentId: reply.Id, Emoji: "🎉",
	}); err == nil {
		t.Fatal("put without a caller = nil error")
	}
	if _, err := f.routes.DeleteCommentReaction(anon, api.DeleteCommentReactionRequestObject{
		CommentId: reply.Id, Emoji: "🎉",
	}); err == nil {
		t.Fatal("delete without a caller = nil error")
	}
}

type brokenStore struct {
	t        *testing.T
	down     *pgxpool.Pool
	react    app.ReactionDeps
	comments app.CommentDeps
	item     uuid.UUID
	proposal ids.ProposalID
	kai, lee ids.UserID
	reply    uuid.UUID
	start    time.Time
}

func (b brokenStore) reactAgain() error {
	_, err := app.NewReactCommentHandler(b.react).Handle(
		asUser(b.t.Context(), b.kai), app.ReactComment{CommentID: b.reply, Viewer: b.kai, Emoji: "👀"})
	return err
}

func (b brokenStore) unreact() error {
	_, err := app.NewUnreactCommentHandler(b.react).Handle(
		asUser(b.t.Context(), b.kai), app.UnreactComment{CommentID: b.reply, Viewer: b.kai, Emoji: "🎉"})
	return err
}

func (b brokenStore) deleteReply() error {
	return app.NewDeleteCommentHandler(b.comments).
		Handle(asUser(b.t.Context(), b.kai), app.DeleteComment{CommentID: b.reply, Caller: b.lee})
}

func (b brokenStore) removeReply() error {
	reason, err := events.NewReason("abuse")
	if err != nil {
		return err
	}
	return app.NewRemoveCommentHandler(b.comments).Handle(
		asUser(
			b.t.Context(),
			b.kai,
		), app.RemoveComment{CommentID: b.reply, Reason: reason, Admin: ids.UserIDFrom(moderatorID())})
}

func (b brokenStore) listPage() error {
	_, err := app.ListComments(
		asUser(b.t.Context(), b.kai),
		b.down,
		app.CommentsQuery{FeedObjectID: b.item, Viewer: b.kai, Limit: 10},
	)
	return err
}

func (b brokenStore) topReply() error {
	_, _, err := app.FindTopReply(
		asUser(b.t.Context(), b.kai),
		b.down,
		[]ids.ProposalID{b.proposal},
		b.start,
		b.start.Add(2*time.Hour),
	)
	return err
}

func TestCommentReactions_failWhenAStoreStepFails(t *testing.T) {
	t.Parallel()
	for name, tt := range map[string]struct {
		query string
		run   func(brokenStore) error
	}{
		"the comment lookup":            {"GetCommentRef", brokenStore.reactAgain},
		"the cap count":                 {"CountUserCommentReactions", brokenStore.reactAgain},
		"the add":                       {"AddCommentReaction", brokenStore.reactAgain},
		"the remove":                    {"RemoveCommentReaction", brokenStore.unreact},
		"the summary after a reaction":  {"CommentReactionSummaries", brokenStore.reactAgain},
		"the clearing on delete":        {"DeleteCommentReactions", brokenStore.deleteReply},
		"the clearing on admin removal": {"DeleteCommentReactions", brokenStore.removeReply},
		"the summaries of a page":       {"CommentReactionSummaries", brokenStore.listPage},
		"the summary of the top reply":  {"CommentReactionSummaries", brokenStore.topReply},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			f := newCommentReactFixture(t)
			item, proposal := f.proposalThread(t)
			kai, lee := f.cabal.Members[0].ID, f.cabal.Members[1].ID
			reply := f.say(t, item, lee, "nice", nil)
			if _, err := f.add(t, reply.Id, kai, "🎉"); err != nil {
				t.Fatal(err)
			}
			cfg := f.pool.Config().Copy()
			cfg.ConnConfig.Tracer = &breakOnQuery{name: tt.query}
			down, err := pgxpool.NewWithConfig(t.Context(), cfg)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(down.Close)
			uow := db.New(down, f.gen, f.clock)
			broken := brokenStore{
				t:        t,
				down:     down,
				item:     item,
				proposal: proposal,
				kai:      kai,
				lee:      lee,
				reply:    reply.Id,
				start:    f.clock.Now().Add(-time.Hour),
				react: app.ReactionDeps{
					UoW:     uow,
					Reads:   down,
					Members: membersOn(down),
					Clock:   f.clock,
					Hints:   f.hints,
				},
				comments: app.CommentDeps{
					UoW: uow, Reads: down, Users: f.users, IDs: f.gen, Clock: f.clock, Hints: f.hints,
				},
			}
			if err := tt.run(broken); err == nil {
				t.Errorf("%s: err = nil, want the failed step", name)
			}
		})
	}
}

func TestTopReply_picksTheMostReactedLiveReplyInTheWindow(t *testing.T) {
	t.Parallel()
	f := newCommentReactFixture(t)
	kai, lee := f.cabal.Members[0].ID, f.cabal.Members[1].ID
	item, proposal := f.proposalThread(t)
	reads := social.New(module.Deps{Pool: f.pool})
	start := f.clock.Now()
	none, err := reads.TopReply(t.Context(), []ids.ProposalID{proposal}, start, start.Add(time.Hour))
	if err != nil || none != nil {
		t.Fatalf("TopReply with no reactions = %+v, %v, want nil", none, err)
	}
	if got, err := reads.TopReply(t.Context(), nil, start, start.Add(time.Hour)); err != nil || got != nil {
		t.Fatalf("TopReply with no proposals = %+v, %v, want nil", got, err)
	}
	first := f.say(t, item, kai, "first", nil)
	second := f.say(t, item, lee, "second", nil)
	taken := f.say(t, item, lee, "taken back", nil)
	for _, r := range []struct {
		comment uuid.UUID
		user    ids.UserID
		emoji   string
	}{
		{first.Id, kai, "🎉"},
		{second.Id, kai, "🎉"},
		{second.Id, lee, "🎉"},
		{taken.Id, kai, "🎉"},
		{taken.Id, lee, "🎉"},
		{taken.Id, kai, "👀"},
	} {
		if _, err := f.add(t, r.comment, r.user, r.emoji); err != nil {
			t.Fatal(err)
		}
	}
	if err := f.delete(t, taken.Id, lee); err != nil {
		t.Fatal(err)
	}
	got, err := reads.TopReply(t.Context(), []ids.ProposalID{proposal}, start, start.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	want := &social.TopReply{
		CommentID: second.Id, ProposalID: proposal.UUID(), AuthorID: lee, Excerpt: "second",
		ReactionCount: 2, TopEmoji: "🎉", TopEmojiCount: 2,
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("TopReply = %+v, want %+v", got, want)
	}
	late, err := reads.TopReply(t.Context(), []ids.ProposalID{proposal}, start.Add(time.Hour), start.Add(2*time.Hour))
	if err != nil || late != nil {
		t.Fatalf("TopReply after the window = %+v, %v, want nil", late, err)
	}
}

func TestTopReply_breaksATieForTheEarliestReplyAndFailsOnAClosedContext(t *testing.T) {
	t.Parallel()
	f := newCommentReactFixture(t)
	kai := f.cabal.Members[0].ID
	item, proposal := f.proposalThread(t)
	start := f.clock.Now()
	first := f.say(t, item, kai, "first", nil)
	second := f.say(t, item, kai, "second", nil)
	for _, id := range []uuid.UUID{second.Id, first.Id} {
		if _, err := f.add(t, id, kai, "🎉"); err != nil {
			t.Fatal(err)
		}
	}
	reads := social.New(module.Deps{Pool: f.pool})
	got, err := reads.TopReply(t.Context(), []ids.ProposalID{proposal}, start, start.Add(time.Hour))
	if err != nil || got == nil || got.CommentID != first.Id {
		t.Fatalf("TopReply on a tie = %+v, %v, want the earliest reply %s", got, err, first.Id)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := reads.TopReply(
		ctx,
		[]ids.ProposalID{proposal},
		start,
		start.Add(time.Hour),
	); errs.CodeOf(
		err,
	) != errs.CodeInternal {
		t.Fatalf("TopReply on a cancelled context err = %v, want internal", err)
	}
}
