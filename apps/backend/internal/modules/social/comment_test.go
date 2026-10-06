package social_test

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/cabal"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/domain/feed"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
	"github.com/monaco/monaco/apps/backend/internal/testkit/feedtest"
)

type commentFixture struct {
	pool     *pgxpool.Pool
	gen      *testkit.IDs
	clock    *testkit.Clock
	uow      *db.UnitOfWork
	create   *app.CreateCommentHandler
	cabal    testkit.SeededCabal
	outsider ids.UserID
}

func newCommentFixture(t *testing.T) commentFixture {
	t.Helper()
	pool := testkit.DB(t)
	g := testkit.NewIDs(622)
	clk := testkit.NewClock(clock.Real{}.Now().UTC().Truncate(time.Microsecond))
	uow := db.New(pool, g, clk)
	return commentFixture{
		pool: pool, gen: g, clock: clk, uow: uow,
		create: app.NewCreateCommentHandler(app.CommentDeps{
			UoW: uow, Reads: pool, Members: cabal.New(module.Deps{Pool: pool}).Queries(),
			IDs: g, Clock: clk,
		}),
		cabal:    testkit.NewCabal(t, pool, testkit.WithMembers(2)),
		outsider: testkit.SeedUser(t, pool, testkit.UserOpts{}).ID,
	}
}

func (f commentFixture) item(t *testing.T, kind feed.Kind) uuid.UUID {
	t.Helper()
	return feedtest.Item(t, f.pool, f.clock, f.gen, func(it *feed.Item) {
		it.Kind, it.CabalID, it.ActorID = kind, f.cabal.ID, f.cabal.Creator.ID
		if kind == feed.KindProposal {
			it.Payload = feed.Payload{
				CabalName: "Alpha Cabal", Symbol: "AAPLx", AssetName: "Apple", Action: feed.ActionBuy,
			}
		}
	})
}

func (f commentFixture) comment(
	t *testing.T, item uuid.UUID, author ids.UserID, body string, parent uuid.UUID,
) (app.Comment, error) {
	t.Helper()
	parsed, err := domain.ParseCommentBody(body)
	if err != nil {
		t.Fatal(err)
	}
	ctx := observability.WithActor(t.Context(), "user:"+author.String())
	return f.create.Handle(ctx, app.CreateComment{FeedObjectID: item, Author: author, Body: parsed, ParentID: parent})
}

func (f commentFixture) mustComment(
	t *testing.T, item uuid.UUID, author ids.UserID, body string, parent uuid.UUID,
) app.Comment {
	t.Helper()
	c, err := f.comment(t, item, author, body, parent)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func (f commentFixture) count(t *testing.T, item uuid.UUID) (stored, counted int) {
	t.Helper()
	if err := f.pool.QueryRow(t.Context(),
		`SELECT (SELECT count(*) FROM feed_comments WHERE feed_object_id = $1), comment_count
		 FROM feed_objects WHERE id = $1`, item).Scan(&stored, &counted); err != nil {
		t.Fatal(err)
	}
	return stored, counted
}

func (f commentFixture) created(t *testing.T) []events.CommentCreated {
	t.Helper()
	rows, err := f.pool.Query(t.Context(), `SELECT payload FROM events WHERE type = $1 ORDER BY id`,
		string(events.TypeCommentCreated))
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []events.CommentCreated
	for rows.Next() {
		var raw []byte
		var e events.CommentCreated
		if err := rows.Scan(&raw); err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(raw, &e); err != nil {
			t.Fatal(err)
		}
		out = append(out, e)
	}
	return out
}

func TestCreateComment_storesTheCommentCountsItAndAppendsTheEvent(t *testing.T) {
	t.Parallel()
	f := newCommentFixture(t)
	item := f.item(t, feed.KindProposal)
	author := f.cabal.Members[1].ID
	got := f.mustComment(t, item, author, "  buy it before earnings  ", uuid.Nil)
	want := app.Comment{
		ID: got.ID, FeedObjectID: item, AuthorID: author, Body: "buy it before earnings", CreatedAt: f.clock.Now(),
	}
	if got != want {
		t.Fatalf("comment = %+v, want %+v", got, want)
	}
	if stored, counted := f.count(t, item); stored != 1 || counted != 1 {
		t.Fatalf("stored %d, comment_count %d, want 1 and 1", stored, counted)
	}
	evs := f.created(t)
	if len(evs) != 1 {
		t.Fatalf("%d comment.created events, want 1", len(evs))
	}
	cabalID, actor, ref := f.cabal.ID.UUID(), f.cabal.Creator.ID.UUID(), evs[0].RefID
	wantEvent := events.CommentCreated{
		V: 1, CommentID: got.ID, FeedObjectID: item, FeedKind: "proposal", RefType: "proposals", RefID: ref,
		CabalID: &cabalID, AuthorID: author.UUID(), ItemActorID: &actor, ProposalID: &ref,
		Excerpt: "buy it before earnings",
	}
	if !reflect.DeepEqual(evs[0], wantEvent) {
		t.Fatalf("event = %+v, want %+v", evs[0], wantEvent)
	}
}

func TestCreateComment_cutsTheExcerptAt120Scalars(t *testing.T) {
	t.Parallel()
	f := newCommentFixture(t)
	item := f.item(t, feed.KindTrade)
	long := strings.Repeat("é", 130)
	f.mustComment(t, item, f.outsider, long, uuid.Nil)
	if got := f.created(t)[0].Excerpt; len([]rune(got)) != domain.CommentExcerptMax {
		t.Fatalf("excerpt has %d scalars, want %d", len([]rune(got)), domain.CommentExcerptMax)
	}
}

func TestCreateComment_aReplyToAReplyLandsUnderTheTopLevelComment(t *testing.T) {
	t.Parallel()
	f := newCommentFixture(t)
	item := f.item(t, feed.KindTrade)
	a, b := f.cabal.Members[0].ID, f.cabal.Members[1].ID
	top := f.mustComment(t, item, a, "top", uuid.Nil)
	reply := f.mustComment(t, item, b, "reply", top.ID)
	nested := f.mustComment(t, item, a, "nested", reply.ID)
	if reply.ParentID != top.ID || reply.ReplyToUserID != (ids.UserID{}) {
		t.Fatalf("reply to a top-level comment = %+v", reply)
	}
	if nested.ParentID != top.ID || nested.ReplyToUserID != b {
		t.Fatalf("reply to a reply = %+v, want parent %s and reply-to %s", nested, top.ID, b)
	}
	if stored, counted := f.count(t, item); stored != 3 || counted != 3 {
		t.Fatalf("stored %d, comment_count %d, want 3 and 3", stored, counted)
	}
}

func TestCreateComment_theEventNamesWhoTheReplyAnswers(t *testing.T) {
	t.Parallel()
	f := newCommentFixture(t)
	item := f.item(t, feed.KindTrade)
	a, b := f.cabal.Members[0].ID.UUID(), f.cabal.Members[1].ID.UUID()
	top := f.mustComment(t, item, f.cabal.Members[0].ID, "top", uuid.Nil)
	reply := f.mustComment(t, item, f.cabal.Members[1].ID, "reply", top.ID)
	f.mustComment(t, item, f.cabal.Members[0].ID, "nested", reply.ID)
	evs := f.created(t)
	type answers struct{ parent, parentAuthor, replyTo *uuid.UUID }
	got := []answers{
		{evs[0].ParentCommentID, evs[0].ParentAuthorID, evs[0].ReplyToUserID},
		{evs[1].ParentCommentID, evs[1].ParentAuthorID, evs[1].ReplyToUserID},
		{evs[2].ParentCommentID, evs[2].ParentAuthorID, evs[2].ReplyToUserID},
	}
	want := []answers{{}, {&top.ID, &a, nil}, {&top.ID, &b, &b}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("answers = %+v, want %+v", got, want)
	}
}

func TestCreateComment_aReplyToADeletedCommentSaysSoInTheEvent(t *testing.T) {
	t.Parallel()
	f := newCommentFixture(t)
	item := f.item(t, feed.KindTrade)
	top := f.mustComment(t, item, f.cabal.Members[0].ID, "top", uuid.Nil)
	if _, err := f.pool.Exec(t.Context(),
		`UPDATE feed_comments SET deleted_at = now(), deleted_by = author_id WHERE id = $1`, top.ID); err != nil {
		t.Fatal(err)
	}
	f.mustComment(t, item, f.outsider, "still here", top.ID)
	if got := f.created(t)[1]; !got.ParentDeleted {
		t.Fatalf("event = %+v, want parent_deleted", got)
	}
}

func TestCreateComment_refusesAParentOnAnotherItemOrNoItem(t *testing.T) {
	t.Parallel()
	f := newCommentFixture(t)
	one, two := f.item(t, feed.KindTrade), f.item(t, feed.KindTrade)
	elsewhere := f.mustComment(t, two, f.outsider, "elsewhere", uuid.Nil)
	_, err := f.comment(t, one, f.outsider, "hi", elsewhere.ID)
	wantCode(t, err, errs.CodeCommentParentMismatch)
	_, err = f.comment(t, one, f.outsider, "hi", f.gen.NewV7())
	wantCode(t, err, errs.CodeCommentParentMismatch)
	if stored, counted := f.count(t, one); stored != 0 || counted != 0 {
		t.Fatalf("stored %d, comment_count %d, want 0 and 0", stored, counted)
	}
}

func TestCreateComment_refusesAnUnknownItem(t *testing.T) {
	t.Parallel()
	f := newCommentFixture(t)
	_, err := f.comment(t, f.gen.NewV7(), f.outsider, "hi", uuid.Nil)
	wantCode(t, err, errs.CodeFeedItemNotFound)
}

func TestCreateComment_keepsProposalsToMembersAndOpensTheOtherKinds(t *testing.T) {
	t.Parallel()
	f := newCommentFixture(t)
	proposal, trade := f.item(t, feed.KindProposal), f.item(t, feed.KindTrade)
	_, err := f.comment(t, proposal, f.outsider, "hi", uuid.Nil)
	wantCode(t, err, errs.CodeCommentMembersOnly)
	if stored, counted := f.count(t, proposal); stored != 0 || counted != 0 {
		t.Fatalf("refused comment left %d rows and comment_count %d", stored, counted)
	}
	f.mustComment(t, trade, f.outsider, "hi", uuid.Nil)
}

func TestCreateComment_passesAMembershipLookupFailureThrough(t *testing.T) {
	t.Parallel()
	f := newCommentFixture(t)
	boom := errs.New(errs.CodeUpstreamUnavailable, "test")
	h := app.NewCreateCommentHandler(app.CommentDeps{
		UoW: f.uow, Reads: f.pool, Members: failingMembers{boom}, IDs: f.gen, Clock: f.clock,
	})
	body, _ := domain.ParseCommentBody("hi")
	_, err := h.Handle(t.Context(), app.CreateComment{
		FeedObjectID: f.item(t, feed.KindProposal), Author: f.outsider, Body: body,
	})
	wantCode(t, err, errs.CodeUpstreamUnavailable)
}

type failingMembers struct{ err error }

func (m failingMembers) IsMember(context.Context, ids.CabalID, ids.UserID) (bool, error) {
	return false, m.err
}

func (m failingMembers) CabalsOf(context.Context, ids.UserID) ([]ids.CabalID, error) {
	return nil, m.err
}

func TestCreateComment_failsWithInternalWhenTheStoreFails(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		breaker string
		reply   bool
	}{
		{"the item lookup fails", `DROP TABLE feed_objects CASCADE`, false},
		{"the parent lookup fails", `DROP TABLE feed_comments CASCADE`, true},
		{"the insert fails", `ALTER TABLE feed_comments ADD CONSTRAINT no_more CHECK (false) NOT VALID`, false},
		{
			"the count bump fails",
			`ALTER TABLE feed_objects ADD CONSTRAINT no_more CHECK (comment_count = 0) NOT VALID`, false,
		},
		{"the event cannot be appended", `DROP TABLE events CASCADE`, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			f := newCommentFixture(t)
			item := f.item(t, feed.KindTrade)
			parent := uuid.Nil
			if tt.reply {
				parent = f.mustComment(t, item, f.outsider, "top", uuid.Nil).ID
			}
			if _, err := f.pool.Exec(t.Context(), tt.breaker); err != nil {
				t.Fatal(err)
			}
			_, err := f.comment(t, item, f.outsider, "hi", parent)
			wantCode(t, err, errs.CodeInternal)
		})
	}
}
