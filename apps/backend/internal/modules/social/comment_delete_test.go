package social_test

import (
	"encoding/json"
	"reflect"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/domain/feed"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
)

func (f commentFixture) delete(t *testing.T, id uuid.UUID, caller ids.UserID) error {
	t.Helper()
	ctx := observability.WithActor(t.Context(), "user:"+caller.String())
	return f.remove.Handle(ctx, app.DeleteComment{CommentID: id, Caller: caller})
}

func (f commentFixture) deleted(t *testing.T) []events.CommentDeleted {
	t.Helper()
	rows, err := f.pool.Query(t.Context(), `SELECT payload FROM events WHERE type = $1 ORDER BY id`,
		string(events.TypeCommentDeleted))
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []events.CommentDeleted
	for rows.Next() {
		var e events.CommentDeleted
		var raw []byte
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

func TestDeleteComment_softDeletesDecrementsTheCountAndAppendsTheEvent(t *testing.T) {
	t.Parallel()
	f := newCommentFixture(t)
	item := f.item(t, feed.KindTrade)
	author := f.cabal.Members[0].ID
	top := f.mustComment(t, item, author, "top", uuid.Nil)
	f.mustComment(t, item, f.outsider, "reply", top.ID)
	f.clock.Advance(1)
	if err := f.delete(t, top.ID, author); err != nil {
		t.Fatal(err)
	}
	var deletedAt *time.Time
	var deletedBy *uuid.UUID
	if err := f.pool.QueryRow(t.Context(), `SELECT deleted_at, deleted_by FROM feed_comments WHERE id = $1`, top.ID).
		Scan(&deletedAt, &deletedBy); err != nil || deletedAt == nil || deletedBy == nil || *deletedBy != author.UUID() {
		t.Fatalf("deleted_at %v, deleted_by %v, err %v, want %s", deletedAt, deletedBy, err, author)
	}
	if stored, counted := f.count(t, item); stored != 2 || counted != 1 {
		t.Fatalf("stored %d, comment_count %d, want 2 and 1", stored, counted)
	}
	want := []events.CommentDeleted{{V: 1, CommentID: top.ID, FeedObjectID: item, DeletedBy: author.UUID()}}
	if got := f.deleted(t); !reflect.DeepEqual(got, want) {
		t.Fatalf("events = %+v, want %+v", got, want)
	}
	if got := f.hints.seen(); len(got) != 3 || got[2] != "global.feed" {
		t.Fatalf("hints = %v, want one after each of the 2 creates and the delete", got)
	}
}

func TestDeleteComment_aSecondDeleteChangesNothing(t *testing.T) {
	t.Parallel()
	f := newCommentFixture(t)
	item := f.item(t, feed.KindTrade)
	author := f.cabal.Members[0].ID
	top := f.mustComment(t, item, author, "top", uuid.Nil)
	for range 2 {
		if err := f.delete(t, top.ID, author); err != nil {
			t.Fatal(err)
		}
	}
	if stored, counted := f.count(t, item); stored != 1 || counted != 0 {
		t.Fatalf("stored %d, comment_count %d, want 1 and 0", stored, counted)
	}
	if got := len(f.deleted(t)); got != 1 {
		t.Fatalf("%d comment.deleted events, want 1", got)
	}
	if got := len(f.hints.seen()); got != 2 {
		t.Fatalf("%d hints, want 2 (create and the first delete)", got)
	}
}

func TestDeleteComment_refusesAnotherUsersCommentAndAnUnknownOne(t *testing.T) {
	t.Parallel()
	f := newCommentFixture(t)
	item := f.item(t, feed.KindTrade)
	top := f.mustComment(t, item, f.cabal.Members[0].ID, "top", uuid.Nil)
	wantCode(t, f.delete(t, top.ID, f.outsider), errs.CodeCommentNotAuthor)
	wantCode(t, f.delete(t, f.gen.NewV7(), f.outsider), errs.CodeCommentNotFound)
	if stored, counted := f.count(t, item); stored != 1 || counted != 1 {
		t.Fatalf("stored %d, comment_count %d, want 1 and 1", stored, counted)
	}
	if got := len(f.deleted(t)); got != 0 {
		t.Fatalf("%d comment.deleted events after two refusals, want 0", got)
	}
}

func TestDeleteComment_failsWithInternalWhenTheStoreFails(t *testing.T) {
	t.Parallel()
	tests := []struct{ name, breaker string }{
		{"the lock fails", `DROP TABLE feed_comments CASCADE`},
		{
			"the update fails",
			`ALTER TABLE feed_comments ADD CONSTRAINT no_deletes CHECK (deleted_at IS NULL) NOT VALID`,
		},
		{
			"the count bump fails",
			`ALTER TABLE feed_objects ADD CONSTRAINT no_drops CHECK (comment_count > 0) NOT VALID`,
		},
		{"the event cannot be appended", `DROP TABLE events CASCADE`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			f := newCommentFixture(t)
			item := f.item(t, feed.KindTrade)
			author := f.cabal.Members[0].ID
			top := f.mustComment(t, item, author, "top", uuid.Nil)
			if _, err := f.pool.Exec(t.Context(), tt.breaker); err != nil {
				t.Fatal(err)
			}
			wantCode(t, f.delete(t, top.ID, author), errs.CodeInternal)
		})
	}
}
