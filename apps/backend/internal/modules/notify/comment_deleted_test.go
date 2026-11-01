package notify_test

import (
	"errors"
	"slices"
	"testing"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/notify"
	"github.com/monaco/monaco/apps/backend/internal/modules/notify/app"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
)

func TestNotify_CommentKinds_NothingGoesOutForADeletedComment(t *testing.T) {
	t.Parallel()
	golden := goldenEvent(t, events.TypeCommentCreated).(events.CommentCreated)
	mentioned := ids.UserIDFrom(fixtureUUID(t, "b11"))
	golden.MentionedUserIDs = uuidsOf(mentioned)
	gone := comments{deleted: map[uuid.UUID]bool{golden.CommentID: true}}
	down := errs.New(errs.CodeDBUnavailable, "test.comments")
	for name, recipients := range map[string]func(c comments) ([]ids.UserID, error){
		"comment_reply": func(c comments) ([]ids.UserID, error) {
			return app.CommentReply{Cabals: cabalsOf(golden), Blocks: blocks{}, Comments: c}.Recipients(t.Context(), golden)
		},
		"post_reply": func(c comments) ([]ids.UserID, error) {
			return app.PostReply{Cabals: cabalsOf(golden), Blocks: blocks{}, Comments: c}.Recipients(t.Context(), golden)
		},
		"comment_mention": func(c comments) ([]ids.UserID, error) {
			return app.CommentMention{Blocks: blocks{}, Comments: c}.Recipients(t.Context(), golden)
		},
	} {
		live, err := recipients(comments{})
		if err != nil || len(live) == 0 {
			t.Fatalf("%s: a live comment gave %v, %v, want a recipient", name, live, err)
		}
		got, err := recipients(gone)
		if err != nil || !slices.Equal(got, nil) {
			t.Errorf("%s: a deleted comment gave %v, %v, want none", name, got, err)
		}
		if _, err := recipients(comments{err: down}); !errors.Is(err, down) {
			t.Errorf("%s: a failing lookup gave %v, want %v", name, err, down)
		}
	}
}

func TestNotify_CommentReply_ADeletedCommentPushesNothingThroughTheModule(t *testing.T) {
	t.Parallel()
	r := newPushRig(t)
	parent, replier := r.follower(t, "ada", "Ada"), r.follower(t, "bea", "Bea")
	r.device(t, parent, token('a'))
	r.device(t, replier, token('b'))
	deps := module.Deps{Pool: r.pool, UoW: r.uow, IDs: r.ids, Clock: r.clock}
	e := r.reply(t, parent, replier)
	e.ReplyRule = "anyone"
	if _, err := r.pool.Exec(
		t.Context(),
		`UPDATE feed_comments SET deleted_at = $2 WHERE id = $1`,
		e.CommentID,
		r.clock.Now(),
	); err != nil {
		t.Fatal(err)
	}

	r.dispatchTo(t, notify.New(deps, notify.WithSender(r.sender)), userActor(replier), e)

	if sent := r.sender.Sent(); len(sent) != 0 {
		t.Fatalf("sent %+v, want nothing for a deleted comment", sent)
	}
}
