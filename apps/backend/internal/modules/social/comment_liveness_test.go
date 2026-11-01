package social_test

import (
	"testing"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/domain/feed"
)

func TestCommentLiveness_aDeletedOrMissingCommentReadsAsDeleted(t *testing.T) {
	t.Parallel()
	f := newCommentFixture(t)
	item, author := f.item(t, feed.KindTrade), f.cabal.Members[0].ID
	live := f.mustComment(t, item, author, "stays", uuid.Nil)
	gone := f.mustComment(t, item, author, "goes", uuid.Nil)
	if err := f.delete(t, gone.ID, author); err != nil {
		t.Fatal(err)
	}
	liveness := app.NewCommentLiveness(f.pool)

	for name, tc := range map[string]struct {
		id   uuid.UUID
		want bool
	}{"a live comment": {live.ID, false}, "a deleted comment": {gone.ID, true}, "an unknown comment": {f.gen.NewV7(), true}} {
		if got, err := liveness.CommentDeleted(t.Context(), tc.id); err != nil || got != tc.want {
			t.Errorf("%s: CommentDeleted = %v, %v, want %v", name, got, err, tc.want)
		}
	}
}

func TestCommentLiveness_failsWithInternalWhenTheStoreFails(t *testing.T) {
	t.Parallel()
	f := newCommentFixture(t)
	if _, err := f.pool.Exec(t.Context(), `DROP TABLE feed_comments CASCADE`); err != nil {
		t.Fatal(err)
	}

	_, err := app.NewCommentLiveness(f.pool).CommentDeleted(t.Context(), f.gen.NewV7())

	wantCode(t, err, errs.CodeInternal)
}
