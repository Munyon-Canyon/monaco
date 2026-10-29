package social_test

import (
	"testing"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/modules/social/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/domain/feed"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

func (f commentFixture) feedMembers(t *testing.T) {
	t.Helper()
	q := sqlc.New(f.pool)
	if err := q.UpsertFeedCabal(t.Context(), sqlc.UpsertFeedCabalParams{
		CabalID: f.cabal.ID.UUID(), Name: "Alpha Cabal", At: f.clock.Now(),
	}); err != nil {
		t.Fatal(err)
	}
	for _, m := range f.cabal.Members {
		_, err := q.UpsertFeedMembership(t.Context(), sqlc.UpsertFeedMembershipParams{
			CabalID: f.cabal.ID.UUID(), UserID: m.ID.UUID(), JoinedAt: f.clock.Now(), EventID: f.gen.NewV7(),
		})
		if err != nil {
			t.Fatal(err)
		}
	}
}

func (f commentFixture) setCommentCount(t *testing.T, item uuid.UUID, n int) {
	t.Helper()
	if _, err := f.pool.Exec(
		t.Context(),
		`UPDATE feed_objects SET comment_count = $2 WHERE id = $1`,
		item,
		n,
	); err != nil {
		t.Fatal(err)
	}
}

func (f commentFixture) feedOf(t *testing.T, db sqlc.DBTX, viewer ids.UserID, limit int) map[uuid.UUID]app.FeedItem {
	t.Helper()
	page, err := app.ListFeed(t.Context(), db, app.FeedQuery{Viewer: viewer, Limit: limit, Now: f.clock.Now()})
	if err != nil {
		t.Fatal(err)
	}
	out := make(map[uuid.UUID]app.FeedItem, len(page.Items))
	for _, it := range page.Items {
		out[it.ID] = it
	}
	return out
}

func (f commentFixture) wantThreadFields(t *testing.T, name, who string, id uuid.UUID, member bool, it app.FeedItem) {
	t.Helper()
	q := sqlc.New(f.pool)
	row, err := q.GetCommentItem(t.Context(), id)
	if err != nil {
		t.Fatal(err)
	}
	thread, err := app.ThreadOf(t.Context(), q, row)
	if err != nil {
		t.Fatal(err)
	}
	var count int32
	err = f.pool.QueryRow(t.Context(), `SELECT comment_count FROM feed_objects WHERE id = $1`, thread.ID).Scan(&count)
	if err != nil {
		t.Fatal(err)
	}
	wantReply, wantRule := app.ReplyRights(feed.Kind(thread.Kind), member)
	wantName := ""
	if wantRule == app.ReplyRuleMembers {
		wantName = "Alpha Cabal"
	}
	if it.ThreadItemID != thread.ID || it.ReplyCount != count || it.CanReply != wantReply ||
		it.ReplyRule != wantRule || !it.CanReact || it.ReplyRuleCabalName != wantName {
		t.Errorf(
			"%s as %s: thread %s count %d reply %v/%s react %v name %q, want thread %s count %d reply %v/%s name %q",
			name,
			who,
			it.ThreadItemID,
			it.ReplyCount,
			it.CanReply,
			it.ReplyRule,
			it.CanReact,
			it.ReplyRuleCabalName,
			thread.ID,
			count,
			wantReply,
			wantRule,
			wantName,
		)
	}
}

func TestFeedReads_threadFieldsAgreeWithThreadOfForEveryKindAndShape(t *testing.T) {
	t.Parallel()
	f := newCommentFixture(t)
	f.feedMembers(t)
	proposal := f.item(t, feed.KindProposal)
	ref := f.proposalRef(t, proposal)
	f.setCommentCount(t, proposal, 6)
	items := map[string]uuid.UUID{
		"proposal":             proposal,
		"trade":                f.item(t, feed.KindTrade),
		"member joined":        f.item(t, feed.KindMemberJoined),
		"cabal created":        f.item(t, feed.KindCabalCreated),
		"price move":           f.item(t, feed.KindPriceMove),
		"member-shaped result": f.closing(t, feed.KindResult, feed.ShapeMember, ref),
		"cabal-shaped result":  f.closing(t, feed.KindResult, feed.ShapeCabal, ref),
		"result with no shape": f.closing(t, feed.KindResult, "", ref),
		"threshold":            f.closing(t, feed.KindThreshold, "", ref),
		"reply":                f.closing(t, feed.KindReply, "", ref),
	}
	for who, viewer := range map[string]ids.UserID{"member": f.cabal.Creator.ID, "outsider": f.outsider} {
		got := f.feedOf(t, f.pool, viewer, app.FeedPageMax)
		for name, id := range items {
			f.wantThreadFields(t, name, who, id, who == "member", got[id])
		}
	}
}

func TestFeedReads_aThresholdAndItsProposalShareOneThreadAndCount(t *testing.T) {
	t.Parallel()
	f := newCommentFixture(t)
	f.feedMembers(t)
	proposal := f.item(t, feed.KindProposal)
	threshold := f.closing(t, feed.KindThreshold, "", f.proposalRef(t, proposal))
	f.setCommentCount(t, proposal, 5)
	got := f.feedOf(t, f.pool, f.outsider, app.FeedPageMax)
	a, b := got[proposal], got[threshold]
	if a.ThreadItemID != proposal || b.ThreadItemID != proposal || a.ReplyCount != 5 || b.ReplyCount != 5 {
		t.Fatalf("proposal thread %s/%d, threshold thread %s/%d, want %s/5 for both",
			a.ThreadItemID, a.ReplyCount, b.ThreadItemID, b.ReplyCount, proposal)
	}
	if a.CanReply || b.CanReply || !a.CanReact || a.ReplyRule != app.ReplyRuleMembers {
		t.Fatalf("outsider can reply %v/%v, react %v, rule %s", a.CanReply, b.CanReply, a.CanReact, a.ReplyRule)
	}
}

func TestGetFeedItem_carriesTheSameFieldsAsTheList(t *testing.T) {
	t.Parallel()
	f := newCommentFixture(t)
	f.feedMembers(t)
	proposal := f.item(t, feed.KindProposal)
	view, err := app.GetFeedItem(t.Context(), f.pool, f.outsider, proposal, app.FeedFilter{})
	if err != nil {
		t.Fatal(err)
	}
	it := view.Item
	if it.CanReply || !it.CanReact || it.ReplyRule != app.ReplyRuleMembers || it.ReplyRuleCabalName != "Alpha Cabal" ||
		it.ThreadItemID != proposal {
		t.Fatalf("item = %+v", it)
	}
	view, err = app.GetFeedItem(
		t.Context(),
		f.pool,
		f.outsider,
		proposal,
		app.FeedFilter{Kinds: []feed.Kind{feed.KindTrade}},
	)
	if err != nil || view.Visible || view.Item.CanReact {
		t.Fatalf("filtered out item: visible %v, can react %v, err %v", view.Visible, view.Item.CanReact, err)
	}
}

func TestListFeed_computesCanReplyForAFullPageWithoutAMembershipCallPerRow(t *testing.T) {
	t.Parallel()
	f := newCommentFixture(t)
	f.feedMembers(t)
	for range app.FeedPageDefault {
		f.item(t, feed.KindProposal)
	}
	testkit.AssertQueries(t, "list feed with thread fields", func() {
		if got := f.feedOf(t, f.pool, f.outsider, app.FeedPageDefault); len(got) != app.FeedPageDefault {
			t.Errorf("items = %d, want %d", len(got), app.FeedPageDefault)
		}
	})
}
