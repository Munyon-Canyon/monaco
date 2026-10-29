package social_test

import (
	"context"
	"reflect"
	"testing"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/cabal"
	"github.com/monaco/monaco/apps/backend/internal/modules/social"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/domain/feed"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/testkit/feedtest"
)

func (f commentFixture) proposalRef(t *testing.T, item uuid.UUID) uuid.UUID {
	t.Helper()
	var ref uuid.UUID
	err := f.pool.QueryRow(t.Context(), `SELECT ref_id FROM feed_objects WHERE id = $1`, item).Scan(&ref)
	if err != nil {
		t.Fatal(err)
	}
	return ref
}

func (f commentFixture) closing(t *testing.T, kind feed.Kind, shape feed.Shape, proposal uuid.UUID) uuid.UUID {
	t.Helper()
	return feedtest.Item(t, f.pool, f.clock, f.gen, func(it *feed.Item) {
		it.Kind, it.CabalID, it.ActorID = kind, f.cabal.ID, f.cabal.Creator.ID
		if kind == feed.KindThreshold {
			it.RefID = proposal
		}
		it.Payload = feed.Payload{
			CabalName: "Alpha Cabal", ActorName: "Ada", Symbol: "AAPLx", Action: feed.ActionBuy,
			ProposalID: proposal, Shape: shape,
		}
	})
}

func (f commentFixture) threadOf(t *testing.T, id uuid.UUID) uuid.UUID {
	t.Helper()
	q := sqlc.New(f.pool)
	item, err := q.GetCommentItem(t.Context(), id)
	if err != nil {
		t.Fatal(err)
	}
	thread, err := app.ThreadOf(t.Context(), q, item)
	if err != nil {
		t.Fatalf("ThreadOf(%s) = %v", item.Kind, err)
	}
	return thread.ID
}

func TestThreadOf_resolvesEveryKindToItsThreadItem(t *testing.T) {
	t.Parallel()
	f := newCommentFixture(t)
	proposal := f.item(t, feed.KindProposal)
	ref := f.proposalRef(t, proposal)
	for name, tt := range map[string]struct {
		item uuid.UUID
		want uuid.UUID
	}{
		"proposal":             {proposal, proposal},
		"trade":                {f.item(t, feed.KindTrade), uuid.Nil},
		"member joined":        {f.item(t, feed.KindMemberJoined), uuid.Nil},
		"cabal created":        {f.item(t, feed.KindCabalCreated), uuid.Nil},
		"price move":           {f.item(t, feed.KindPriceMove), uuid.Nil},
		"member-shaped result": {f.closing(t, feed.KindResult, feed.ShapeMember, ref), uuid.Nil},
		"cabal-shaped result":  {f.closing(t, feed.KindResult, feed.ShapeCabal, ref), proposal},
		"result with no shape": {f.closing(t, feed.KindResult, "", ref), proposal},
		"threshold":            {f.closing(t, feed.KindThreshold, "", ref), proposal},
		"reply":                {f.closing(t, feed.KindReply, "", ref), proposal},
	} {
		want := tt.want
		if want == uuid.Nil {
			want = tt.item
		}
		if got := f.threadOf(t, tt.item); got != want {
			t.Errorf("%s: thread = %s, want %s", name, got, want)
		}
	}
}

func TestThreadOf_aMissingProposalItemIsPending(t *testing.T) {
	t.Parallel()
	f := newCommentFixture(t)
	for _, kind := range []feed.Kind{feed.KindResult, feed.KindThreshold, feed.KindReply} {
		item := f.closing(t, kind, feed.ShapeCabal, f.gen.NewV7())
		_, err := f.comment(t, item, f.cabal.Members[0].ID, "hi", uuid.Nil)
		wantCode(t, err, errs.CodeFeedItemPending)
	}
}

func TestCanReply_membersOnAProposalAndAnyoneOnTheRest(t *testing.T) {
	t.Parallel()
	f := newCommentFixture(t)
	members := cabal.New(module.Deps{Pool: f.pool}).Queries()
	member := f.cabal.Members[0].ID
	for _, tt := range []struct {
		kind   feed.Kind
		user   ids.UserID
		want   bool
		reason app.ReplyRule
	}{
		{feed.KindProposal, member, true, app.ReplyRuleMembers},
		{feed.KindProposal, f.outsider, false, app.ReplyRuleMembers},
		{feed.KindTrade, f.outsider, true, app.ReplyRuleAnyone},
		{feed.KindMemberJoined, f.outsider, true, app.ReplyRuleAnyone},
		{feed.KindCabalCreated, f.outsider, true, app.ReplyRuleAnyone},
		{feed.KindPriceMove, f.outsider, true, app.ReplyRuleAnyone},
		{feed.KindResult, f.outsider, true, app.ReplyRuleAnyone},
		{feed.KindReply, member, true, app.ReplyRuleAnyone},
	} {
		got, reason, err := app.CanReply(t.Context(), members, tt.kind, f.cabal.ID, tt.user)
		if err != nil || got != tt.want || reason != tt.reason {
			t.Errorf("CanReply(%s) = %v, %s, %v, want %v, %s", tt.kind, got, reason, err, tt.want, tt.reason)
		}
	}
}

func TestCanReact_anyoneWhoCanSeeTheItem(t *testing.T) {
	t.Parallel()
	f := newCommentFixture(t)
	proposal := f.item(t, feed.KindProposal)
	item, err := app.CanReact(t.Context(), f.pool, f.outsider, proposal)
	if err != nil || item.ID != proposal {
		t.Errorf("CanReact on a proposal of another cabal = %+v, %v, want the item", item, err)
	}
	_, err = app.CanReact(t.Context(), f.pool, f.outsider, f.gen.NewV7())
	wantCode(t, err, errs.CodeFeedItemNotFound)
}

func TestCreateComment_anOutsiderCannotReplyToAProposalOrItsCabalResult(t *testing.T) {
	t.Parallel()
	f := newCommentFixture(t)
	proposal := f.item(t, feed.KindProposal)
	result := f.closing(t, feed.KindResult, feed.ShapeCabal, f.proposalRef(t, proposal))
	for _, item := range []uuid.UUID{proposal, result} {
		_, err := f.comment(t, item, f.outsider, "let me in", uuid.Nil)
		wantCode(t, err, errs.CodeCommentMembersOnly)
	}
	if stored, counted := f.count(t, proposal); stored != 0 || counted != 0 {
		t.Fatalf("proposal thread: stored %d, comment_count %d, want 0 and 0", stored, counted)
	}
}

func TestCreateComment_aMemberShapedResultTakesRepliesFromAnyoneOnItsOwnThread(t *testing.T) {
	t.Parallel()
	f := newCommentFixture(t)
	proposal := f.item(t, feed.KindProposal)
	sale := f.closing(t, feed.KindResult, feed.ShapeMember, f.proposalRef(t, proposal))
	f.mustComment(t, sale, f.outsider, "nice sale", uuid.Nil)
	if stored, counted := f.count(t, sale); stored != 1 || counted != 1 {
		t.Fatalf("sale thread: stored %d, comment_count %d, want 1 and 1", stored, counted)
	}
	if stored, counted := f.count(t, proposal); stored != 0 || counted != 0 {
		t.Fatalf("proposal thread: stored %d, comment_count %d, want 0 and 0", stored, counted)
	}
}

func TestCreateComment_anySignedInUserCanReplyToTheOtherKinds(t *testing.T) {
	t.Parallel()
	f := newCommentFixture(t)
	for _, kind := range []feed.Kind{feed.KindTrade, feed.KindMemberJoined, feed.KindCabalCreated, feed.KindPriceMove} {
		item := f.item(t, kind)
		f.mustComment(t, item, f.outsider, "hello", uuid.Nil)
		if stored, counted := f.count(t, item); stored != 1 || counted != 1 {
			t.Errorf("%s thread: stored %d, comment_count %d, want 1 and 1", kind, stored, counted)
		}
	}
}

func TestCreateComment_aCardOfTheProposalWritesToTheProposalThread(t *testing.T) {
	t.Parallel()
	f := newCommentFixture(t)
	member := f.cabal.Members[0].ID
	proposal := f.item(t, feed.KindProposal)
	ref := f.proposalRef(t, proposal)
	cards := []uuid.UUID{
		f.closing(t, feed.KindResult, feed.ShapeCabal, ref),
		f.closing(t, feed.KindThreshold, "", ref),
		f.closing(t, feed.KindReply, "", ref),
	}
	for _, card := range cards {
		got := f.mustComment(t, card, member, "through a card", uuid.Nil)
		if got.FeedObjectID != proposal {
			t.Errorf("comment landed on %s, want the proposal thread %s", got.FeedObjectID, proposal)
		}
		if stored, counted := f.count(t, card); stored != 0 || counted != 0 {
			t.Errorf("card: stored %d, comment_count %d, want 0 and 0", stored, counted)
		}
	}
	if stored, counted := f.count(t, proposal); stored != 3 || counted != 3 {
		t.Fatalf("proposal thread: stored %d, comment_count %d, want 3 and 3", stored, counted)
	}
	counts, err := social.New(module.Deps{Pool: f.pool}).
		ReplyCounts(t.Context(), []ids.ProposalID{ids.ProposalIDFrom(ref)})
	if err != nil || counts[ids.ProposalIDFrom(ref)] != 3 {
		t.Errorf("ReplyCounts = %v, %v, want 3 across the cards", counts, err)
	}
}

func TestCreateComment_theEventNamesThePostAndItsThread(t *testing.T) {
	t.Parallel()
	f := newCommentFixture(t)
	proposal := f.item(t, feed.KindProposal)
	result := f.closing(t, feed.KindResult, feed.ShapeCabal, f.proposalRef(t, proposal))
	sale := f.feedSale(t)
	f.mustComment(t, result, f.cabal.Members[0].ID, "on the result", uuid.Nil)
	f.mustComment(t, sale, f.outsider, "on the sale", uuid.Nil)
	evs := f.created(t)
	if len(evs) != 2 {
		t.Fatalf("%d comment.created events, want 2", len(evs))
	}
	cabalName := "Alpha Cabal"
	for name, tt := range map[string]struct {
		got, want events.CommentCreated
	}{
		"cabal result": {evs[0], events.CommentCreated{
			ItemKind: "result", ThreadFeedObjectID: proposal, ItemActorName: "Ada", ItemLabel: "Buy AAPLx",
			ItemNoun: "proposal", CabalName: &cabalName, ReplyRule: "members",
		}},
		"member sale": {evs[1], events.CommentCreated{
			ItemKind: "result", ThreadFeedObjectID: sale, ItemActorName: "Ada", ItemLabel: "AAPLx sale",
			ItemNoun: "sale", CabalName: &cabalName, ReplyRule: "anyone",
		}},
	} {
		got := events.CommentCreated{
			ItemKind:           tt.got.ItemKind,
			ThreadFeedObjectID: tt.got.ThreadFeedObjectID,
			ItemActorName:      tt.got.ItemActorName,
			ItemLabel:          tt.got.ItemLabel,
			ItemNoun:           tt.got.ItemNoun,
			CabalName:          tt.got.CabalName,
			ReplyRule:          tt.got.ReplyRule,
		}
		if !reflect.DeepEqual(got, tt.want) {
			t.Errorf("%s: event fields = %+v, want %+v", name, got, tt.want)
		}
	}
}

func (f commentFixture) feedSale(t *testing.T) uuid.UUID {
	t.Helper()
	return feedtest.Item(t, f.pool, f.clock, f.gen, func(it *feed.Item) {
		it.Kind, it.CabalID, it.ActorID = feed.KindResult, f.cabal.ID, f.cabal.Creator.ID
		it.Payload = feed.Payload{
			CabalName: "Alpha Cabal",
			ActorName: "Ada",
			Symbol:    "AAPLx",
			Action:    feed.ActionSell,
			Shape:     feed.ShapeMember,
		}
	})
}

func TestThreadOf_aThresholdWithNoProposalInItsPayloadUsesItsRef(t *testing.T) {
	t.Parallel()
	f := newCommentFixture(t)
	proposal := f.item(t, feed.KindProposal)
	row := sqlc.GetCommentItemRow{
		ID: f.gen.NewV7(), Kind: string(feed.KindThreshold), RefType: string(feed.RefProposals),
		RefID: f.proposalRef(t, proposal), Payload: []byte("{}"),
	}
	thread, err := app.ThreadOf(t.Context(), sqlc.New(f.pool), row)
	if err != nil || thread.ID != proposal {
		t.Errorf("ThreadOf = %s, %v, want the proposal item %s", thread.ID, err, proposal)
	}
}

func TestThreadOf_failsOnAPayloadItCannotRead(t *testing.T) {
	t.Parallel()
	f := newCommentFixture(t)
	row := sqlc.GetCommentItemRow{Kind: string(feed.KindResult), Payload: []byte("{")}
	_, err := app.ThreadOf(t.Context(), sqlc.New(f.pool), row)
	wantCode(t, err, errs.CodeInternal)
}

func TestResolveThread_failsWhenTheReadFails(t *testing.T) {
	t.Parallel()
	f := newCommentFixture(t)
	proposal := f.item(t, feed.KindProposal)
	result := f.closing(t, feed.KindResult, feed.ShapeCabal, f.proposalRef(t, proposal))
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	for _, id := range []uuid.UUID{proposal, result} {
		_, _, err := app.ResolveThread(ctx, sqlc.New(f.pool), id)
		wantCode(t, err, errs.CodeInternal)
	}
}

func TestResolveThread_namesAMissingItemAndAMissingProposal(t *testing.T) {
	t.Parallel()
	f := newCommentFixture(t)
	_, _, err := app.ResolveThread(t.Context(), sqlc.New(f.pool), f.gen.NewV7())
	wantCode(t, err, errs.CodeFeedItemNotFound)
	orphan := f.closing(t, feed.KindResult, feed.ShapeCabal, f.gen.NewV7())
	_, err = app.ListComments(t.Context(), f.pool, app.CommentsQuery{FeedObjectID: orphan, Limit: 10})
	wantCode(t, err, errs.CodeFeedItemPending)
}

func TestThreadOf_failsWhenTheProposalReadFails(t *testing.T) {
	t.Parallel()
	f := newCommentFixture(t)
	proposal := f.item(t, feed.KindProposal)
	row := sqlc.GetCommentItemRow{
		Kind: string(feed.KindThreshold), RefType: string(feed.RefProposals), RefID: f.proposalRef(t, proposal),
		Payload: []byte("{}"),
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	_, err := app.ThreadOf(ctx, sqlc.New(f.pool), row)
	wantCode(t, err, errs.CodeInternal)
}
