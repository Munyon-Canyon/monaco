package social_test

import (
	"context"
	"reflect"
	"testing"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/cabal"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/domain/feed"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
)

type replyRow struct {
	ID, RefID, ActorID, CabalID uuid.UUID
	RefType, Title, Body        string
	Symbol, CabalName           string
	Payload                     feed.Payload
}

func (f commentFixture) replyItems(t *testing.T) []replyRow {
	t.Helper()
	rows, err := f.pool.Query(t.Context(), `SELECT id, ref_id, actor_id, cabal_id, ref_type, title,
		coalesce(body, ''), coalesce(symbol, ''), coalesce(cabal_name, ''), payload
		FROM feed_objects WHERE kind = 'reply' ORDER BY created_at, id`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []replyRow
	for rows.Next() {
		var (
			r   replyRow
			raw []byte
		)
		err := rows.Scan(&r.ID, &r.RefID, &r.ActorID, &r.CabalID, &r.RefType, &r.Title, &r.Body, &r.Symbol,
			&r.CabalName, &raw)
		if err != nil {
			t.Fatal(err)
		}
		if r.Payload, err = feed.ParsePayload(raw); err != nil {
			t.Fatal(err)
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}

func (f commentFixture) refOf(t *testing.T, item uuid.UUID) uuid.UUID {
	t.Helper()
	var ref uuid.UUID
	err := f.pool.QueryRow(t.Context(), `SELECT ref_id FROM feed_objects WHERE id = $1`, item).Scan(&ref)
	if err != nil {
		t.Fatal(err)
	}
	return ref
}

func TestCreateComment_onAProposalMakesOneReplyItemWithTheProposalsFigures(t *testing.T) {
	t.Parallel()
	f := newCommentFixture(t)
	item := f.item(t, feed.KindProposal)
	author := f.cabal.Members[0].ID
	comment := f.mustComment(t, item, author, "  buy it before earnings  ", uuid.Nil)
	got := f.replyItems(t)
	if len(got) != 1 {
		t.Fatalf("%d reply items, want 1", len(got))
	}
	want := replyRow{
		ID: got[0].ID, RefID: comment.ID, ActorID: author.UUID(), CabalID: f.cabal.ID.UUID(),
		RefType: "feed_comments", Title: "Kai replied to Ada's proposal", Body: "buy it before earnings",
		Symbol: "AAPLx", CabalName: "Alpha Cabal",
		Payload: feed.Payload{
			CabalName: "Alpha Cabal", ActorName: "Kai", ProposerName: "Ada", Symbol: "AAPLx", AssetName: "Apple",
			Action: feed.ActionBuy, USDCMicros: money.MicrosFromUint64(250_000_000), TokenAmount: 1_500_000,
			TokenDecimals: 6, ShareOfPotBps: 1000, ProposalID: f.refOf(t, item),
		},
	}
	if !reflect.DeepEqual(got[0], want) {
		t.Fatalf("reply item = %+v, want %+v", got[0], want)
	}
	if stored, counted := f.count(t, item); stored != 1 || counted != 1 {
		t.Fatalf("stored %d, comment_count %d, want 1 and 1", stored, counted)
	}
}

func TestCreateComment_onAnotherKindMakesNoReplyItem(t *testing.T) {
	t.Parallel()
	f := newCommentFixture(t)
	f.mustComment(t, f.item(t, feed.KindTrade), f.cabal.Members[0].ID, "nice", uuid.Nil)
	f.mustComment(t, f.item(t, feed.KindPriceMove), f.outsider, "wow", uuid.Nil)
	if got := f.replyItems(t); len(got) != 0 {
		t.Fatalf("%d reply items, want 0", len(got))
	}
}

func TestCreateComment_aReplyToACommentMakesItsOwnReplyItem(t *testing.T) {
	t.Parallel()
	f := newCommentFixture(t)
	item := f.item(t, feed.KindProposal)
	a, b := f.cabal.Members[0].ID, f.cabal.Members[1].ID
	top := f.mustComment(t, item, a, "top", uuid.Nil)
	f.clock.Advance(1)
	reply := f.mustComment(t, item, b, "reply", top.ID)
	f.clock.Advance(1)
	nested := f.mustComment(t, item, a, "nested", reply.ID)
	got := f.replyItems(t)
	if len(got) != 3 {
		t.Fatalf("%d reply items, want 3", len(got))
	}
	for i, want := range []struct {
		comment uuid.UUID
		actor   ids.UserID
		title   string
		body    string
	}{
		{top.ID, a, "Kai replied to Ada's proposal", "top"},
		{reply.ID, b, "lee replied to Ada's proposal", "reply"},
		{nested.ID, a, "Kai replied to Ada's proposal", "nested"},
	} {
		if got[i].RefID != want.comment || got[i].ActorID != want.actor.UUID() ||
			got[i].Title != want.title || got[i].Body != want.body {
			t.Errorf("reply item %d = %+v, want comment %s by %s %q %q",
				i, got[i], want.comment, want.actor, want.title, want.body)
		}
	}
}

func TestCreateComment_refusedCommentMakesNoReplyItem(t *testing.T) {
	t.Parallel()
	f := newCommentFixture(t)
	_, err := f.comment(t, f.item(t, feed.KindProposal), f.outsider, "hi", uuid.Nil)
	wantCode(t, err, errs.CodeCommentMembersOnly)
	if got := f.replyItems(t); len(got) != 0 {
		t.Fatalf("%d reply items after a refusal, want 0", len(got))
	}
}

func TestCreateComment_rollsBackWhenTheReplyItemOrTheCardFails(t *testing.T) {
	t.Parallel()
	boom := errs.New(errs.CodeUpstreamUnavailable, "test")
	tests := []struct {
		name string
		arm  func(*testing.T, commentFixture)
		want errs.Code
	}{
		{"the reply item cannot be stored", func(t *testing.T, f commentFixture) {
			t.Helper()
			if _, err := f.pool.Exec(t.Context(),
				`ALTER TABLE feed_objects ADD CONSTRAINT no_reply CHECK (kind <> 'reply') NOT VALID`); err != nil {
				t.Fatal(err)
			}
		}, errs.CodeInternal},
		{"the proposal's payload cannot be read", func(t *testing.T, f commentFixture) {
			t.Helper()
			if _, err := f.pool.Exec(t.Context(),
				`UPDATE feed_objects SET payload = '{"action": 5}' WHERE kind = 'proposal'`); err != nil {
				t.Fatal(err)
			}
		}, errs.CodeInternal},
		{"the commenter's card cannot be read", func(t *testing.T, f commentFixture) {
			t.Helper()
			f.users.Fail("UsersByID", boom)
		}, errs.CodeUpstreamUnavailable},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			f := newCommentFixture(t)
			item := f.item(t, feed.KindProposal)
			tt.arm(t, f)
			_, err := f.comment(t, item, f.cabal.Members[0].ID, "hi", uuid.Nil)
			wantCode(t, err, tt.want)
			if stored, counted := f.count(t, item); stored != 0 || counted != 0 {
				t.Fatalf("stored %d, comment_count %d, want 0 and 0", stored, counted)
			}
		})
	}
}

func (f commentFixture) like(t *testing.T, item uuid.UUID, user ids.UserID) {
	t.Helper()
	if _, err := f.pool.Exec(t.Context(),
		`INSERT INTO feed_likes (feed_object_id, user_id) VALUES ($1, $2)`, item, user.UUID()); err != nil {
		t.Fatal(err)
	}
}

func TestDeleteComment_deletesItsReplyItemAndItsLikesOnly(t *testing.T) {
	t.Parallel()
	f := newCommentFixture(t)
	item := f.item(t, feed.KindProposal)
	a, b := f.cabal.Members[0].ID, f.cabal.Members[1].ID
	doomed := f.mustComment(t, item, a, "doomed", uuid.Nil)
	f.clock.Advance(1)
	kept := f.mustComment(t, item, b, "kept", uuid.Nil)
	for _, r := range f.replyItems(t) {
		f.like(t, r.ID, b)
	}
	ctx := observability.WithActor(t.Context(), "user:"+a.String())
	for range 2 {
		if err := f.remove.Handle(ctx, app.DeleteComment{CommentID: doomed.ID, Caller: a}); err != nil {
			t.Fatal(err)
		}
	}
	got := f.replyItems(t)
	if len(got) != 1 || got[0].RefID != kept.ID {
		t.Fatalf("reply items = %+v, want only the item of comment %s", got, kept.ID)
	}
	var likes int
	if err := f.pool.QueryRow(t.Context(), `SELECT count(*) FROM feed_likes`).Scan(&likes); err != nil || likes != 1 {
		t.Fatalf("%d likes, %v, want the one on the kept item", likes, err)
	}
}

func TestListFeed_filtersByTheReplyAndThresholdKinds(t *testing.T) {
	t.Parallel()
	f := newFeedFixture(t)
	viewer := ids.NewUserID(f.gen)
	trade := f.item(t)
	reply := f.item(t, func(it *feed.Item) { it.Kind = feed.KindReply })
	threshold := f.item(t, func(it *feed.Item) { it.Kind = feed.KindThreshold })
	for name, tt := range map[string]struct {
		kinds []feed.Kind
		want  []uuid.UUID
	}{
		"reply":     {[]feed.Kind{feed.KindReply}, []uuid.UUID{reply}},
		"threshold": {[]feed.Kind{feed.KindThreshold}, []uuid.UUID{threshold}},
		"both":      {[]feed.Kind{feed.KindReply, feed.KindThreshold}, []uuid.UUID{reply, threshold}},
		"all":       {nil, []uuid.UUID{trade, reply, threshold}},
	} {
		page := f.list(t, app.FeedQuery{Viewer: viewer, Filter: app.FeedFilter{Kinds: tt.kinds}})
		if !sameSet(idsOf(page.Items), tt.want) {
			t.Errorf("%s: got %v, want %v", name, idsOf(page.Items), tt.want)
		}
	}
}

func TestListFeed_sendsTheProposalOfAReplyAndAThresholdItem(t *testing.T) {
	t.Parallel()
	f := newFeedFixture(t)
	viewer, proposal := ids.NewUserID(f.gen), f.gen.NewV7()
	payload := feed.Payload{
		CabalName: "Alpha Cabal", ActorName: "Kai", ProposerName: "Ada", Symbol: "AAPLx", AssetName: "Apple",
		Action: feed.ActionBuy, USDCMicros: money.MicrosFromUint64(250_000_000), TokenAmount: 1_500_000,
		TokenDecimals: 6, ShareOfPotBps: 1000, ProposalID: proposal,
	}
	reply := f.item(t, func(it *feed.Item) { it.Kind, it.Payload = feed.KindReply, payload })
	threshold := f.item(t, func(it *feed.Item) {
		it.Kind, it.RefID, it.Payload = feed.KindThreshold, proposal, payload
	})
	page := f.list(t, app.FeedQuery{Viewer: viewer})
	if len(page.Items) != 2 {
		t.Fatalf("%d items, want 2", len(page.Items))
	}
	for _, item := range page.Items {
		if item.ID != reply && item.ID != threshold {
			t.Fatalf("unexpected item %s", item.ID)
		}
		if item.ProposalID != proposal || item.ShareOfPotBps != 1000 || item.TokenAmount != 1_500_000 ||
			item.USDCMicros != payload.USDCMicros || item.Action != feed.ActionBuy {
			t.Errorf("%s item = %+v, want the proposal %s and its figures", item.Kind, item, proposal)
		}
	}
}

type handleOnlyUsers struct {
	app.Users
	handle string
}

func (u handleOnlyUsers) UsersByID(_ context.Context, users []ids.UserID) (map[ids.UserID]app.UserCard, error) {
	return map[ids.UserID]app.UserCard{users[0]: {Handle: u.handle}}, nil
}

func TestCreateComment_namesACommenterWithoutADisplayNameByHandle(t *testing.T) {
	t.Parallel()
	f := newCommentFixture(t)
	h := app.NewCreateCommentHandler(app.CommentDeps{
		UoW: f.uow, Reads: f.pool, Members: cabal.New(module.Deps{Pool: f.pool}).Queries(),
		Users: handleOnlyUsers{handle: "lee"}, IDs: f.gen, Clock: f.clock, Hints: f.hints,
	})
	body, _ := domain.ParseCommentBody("hi")
	author := f.cabal.Members[0].ID
	ctx := observability.WithActor(t.Context(), "user:"+author.String())
	_, err := h.Handle(ctx, app.CreateComment{
		FeedObjectID: f.item(t, feed.KindProposal), Author: author, Input: domain.CommentInput{Body: &body},
	})
	if err != nil {
		t.Fatal(err)
	}
	got := f.replyItems(t)
	if len(got) != 1 || got[0].Payload.ActorName != "lee" {
		t.Fatalf("reply items = %+v, want one by lee", got)
	}
}
