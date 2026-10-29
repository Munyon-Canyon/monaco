package social_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/domain/feed"
	"github.com/monaco/monaco/apps/backend/internal/platform/auth"
	apibase "github.com/monaco/monaco/apps/backend/internal/platform/httpx/api"
	api "github.com/monaco/monaco/apps/backend/internal/platform/httpx/api/socialapi"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

func moderatorID() uuid.UUID { return testkit.NewIDs(9).NewV7() }

func asAdmin(t *testing.T) context.Context {
	t.Helper()
	return auth.WithActor(t.Context(), auth.Actor{Kind: auth.ActorAdmin, ID: moderatorID().String(), Role: "moderator"})
}

func (f commentRoutes) removeAs(ctx context.Context, comment uuid.UUID, reason string) error {
	_, err := f.routes.RemoveComment(ctx, api.RemoveCommentRequestObject{
		Id: comment, Body: &apibase.ReasonBody{Reason: reason},
	})
	return err
}

func (f commentFixture) audits(t *testing.T) []events.AdminAction {
	t.Helper()
	rows, err := f.pool.Query(t.Context(), `SELECT payload FROM events WHERE type = $1 ORDER BY id`,
		string(events.TypeAdminAction))
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []events.AdminAction
	for rows.Next() {
		var raw []byte
		var a events.AdminAction
		if err := rows.Scan(&raw); err != nil || json.Unmarshal(raw, &a) != nil {
			t.Fatalf("scan admin.action: %v", err)
		}
		out = append(out, a)
	}
	return out
}

func TestAdminRemoveComment_RendersRemovedByMonaco(t *testing.T) {
	t.Parallel()
	f := newCommentRoutes(t)
	item := f.item(t, feed.KindTrade)
	kai := f.cabal.Members[0].ID
	top := f.say(t, item, f.outsider, "secret abuse", nil)
	f.say(t, item, kai, "reply", &top.Id)
	for range 2 {
		if err := f.removeAs(asAdmin(t), top.Id, "abuse"); err != nil {
			t.Fatalf("RemoveComment = %v, want nil", err)
		}
	}
	page := f.read(t, f.outsider, item, api.GetFeedCommentsParams{})
	gotTop := page.Items[0].Comment
	if gotTop.Body != nil || gotTop.BodyDisplay != "Removed by Monaco" || len(page.Items[0].Replies) != 1 {
		t.Fatalf("removed comment = %+v with %d replies, want a null body, Removed by Monaco and 1 reply",
			gotTop, len(page.Items[0].Replies))
	}
	if stored, counted := f.count(t, item); stored != 2 || counted != 1 {
		t.Errorf("stored %d, comment_count %d, want 2 and 1", stored, counted)
	}
	f.expectRemovalEvents(t, top.Id, item)
}

func TestAdminRemoveComment_deletesTheCommentsReplyItem(t *testing.T) {
	t.Parallel()
	f := newCommentRoutes(t)
	item := f.item(t, feed.KindProposal)
	kai := f.cabal.Members[0].ID
	top := f.say(t, item, kai, "secret abuse", nil)
	kept := f.say(t, item, kai, "fine", nil)
	if got := f.replyItems(t); len(got) != 2 {
		t.Fatalf("%d reply items before the removal, want 2", len(got))
	}
	if err := f.removeAs(asAdmin(t), top.Id, "abuse"); err != nil {
		t.Fatalf("RemoveComment = %v, want nil", err)
	}
	got := f.replyItems(t)
	if len(got) != 1 || got[0].RefID != kept.Id {
		t.Fatalf("reply items = %+v, want only the item of comment %s", got, kept.Id)
	}
}

func (f commentRoutes) expectRemovalEvents(t *testing.T, comment, item uuid.UUID) {
	t.Helper()
	got := f.deleted(t)
	want := events.CommentDeleted{
		V: 1, CommentID: comment, FeedObjectID: item, DeletedBy: moderatorID(),
		DeletedByKind: "admin",
	}
	if len(got) != 1 || got[0] != want {
		t.Fatalf("comment.deleted = %+v, want one %+v", got, want)
	}
	audits := f.audits(t)
	if len(audits) != 1 {
		t.Fatalf("%d admin.action events, want 1", len(audits))
	}
	f.expectAudit(t, audits[0], comment)
}

func (f commentRoutes) expectAudit(t *testing.T, a events.AdminAction, comment uuid.UUID) {
	t.Helper()
	if a.Action != events.AdminActionCommentRemove || a.TargetType != events.AdminTargetComment ||
		a.TargetID != comment.String() || a.AdminID != moderatorID() || a.Reason != "abuse" {
		t.Errorf("admin.action = %+v, want the moderator's comment_remove of the comment", a)
	}
	raw, _ := json.Marshal(a)
	if strings.Contains(string(raw), "secret abuse") {
		t.Errorf("admin.action %s holds the comment body", raw)
	}
	var before, after map[string]bool
	if json.Unmarshal(a.Before, &before) != nil || json.Unmarshal(a.After, &after) != nil ||
		before["deleted"] || !after["deleted"] {
		t.Errorf("admin.action before %s after %s, want deleted false then true", a.Before, a.After)
	}
}

func TestAdminRemoveComment_Refusals(t *testing.T) {
	t.Parallel()
	f := newCommentRoutes(t)
	item := f.item(t, feed.KindTrade)
	top := f.say(t, item, f.cabal.Members[0].ID, "top", nil)
	user := &auth.Actor{Kind: auth.ActorUser, ID: f.outsider.String()}
	service := &auth.Actor{Kind: auth.ActorAdmin, ID: "ops-1"}
	admin := &auth.Actor{Kind: auth.ActorAdmin, ID: moderatorID().String(), Role: "moderator"}
	for name, tc := range map[string]struct {
		actor   *auth.Actor
		comment uuid.UUID
		reason  string
		want    errs.Code
	}{
		"no actor":     {nil, top.Id, "abuse", errs.CodeUnauthorized},
		"a user":       {user, top.Id, "abuse", errs.CodeForbidden},
		"a service id": {service, top.Id, "abuse", errs.CodeAdminForbidden},
		"unknown":      {admin, f.gen.NewV7(), "abuse", errs.CodeCommentNotFound},
		"short reason": {admin, top.Id, "no", errs.CodeReasonRequired},
	} {
		ctx := t.Context()
		if tc.actor != nil {
			ctx = auth.WithActor(ctx, *tc.actor)
		}
		if err := f.removeAs(ctx, tc.comment, tc.reason); errs.CodeOf(err) != tc.want {
			t.Errorf("%s: RemoveComment = %v, want %s", name, err, tc.want)
		}
	}
	if stored, counted := f.count(t, item); stored != 1 || counted != 1 {
		t.Errorf("stored %d, comment_count %d, want 1 and 1", stored, counted)
	}
	if n := len(f.deleted(t)) + len(f.audits(t)); n != 0 {
		t.Errorf("%d events after refusals, want 0", n)
	}
}

func TestAdminRemoveComment_NeedsAReason(t *testing.T) {
	t.Parallel()
	f := newCommentRoutes(t)
	top := f.say(t, f.item(t, feed.KindTrade), f.cabal.Members[0].ID, "top", nil)
	cmd := app.RemoveComment{CommentID: top.Id, Admin: ids.UserIDFrom(moderatorID())}
	err := app.NewRemoveCommentHandler(f.deps()).Handle(asAdmin(t), cmd)
	if errs.CodeOf(err) != errs.CodeReasonRequired {
		t.Fatalf("RemoveComment without a reason = %v, want %s", err, errs.CodeReasonRequired)
	}
	if n := len(f.deleted(t)); n != 0 {
		t.Fatalf("%d comment.deleted events, want 0", n)
	}
}

func (f commentFixture) deps() app.CommentDeps {
	return app.CommentDeps{UoW: f.uow, Reads: f.pool, Users: f.users, IDs: f.gen, Clock: f.clock, Hints: f.hints}
}

func TestAdminRemoveComment_RollsBackWhenAWriteFails(t *testing.T) {
	t.Parallel()
	for name, ddl := range map[string]string{
		"the lock":   `ALTER TABLE feed_comments RENAME COLUMN deleted_at TO gone_at`,
		"the update": `ALTER TABLE feed_comments ADD CONSTRAINT never_removed CHECK (deleted_by_kind <> 'admin') NOT VALID`,
		"the count":  `ALTER TABLE feed_objects RENAME COLUMN comment_count TO comments`,
		"the item":   `ALTER TABLE feed_likes RENAME TO feed_likes_gone`,
		"the event":  `ALTER TABLE events ADD CONSTRAINT no_event CHECK (type <> 'comment.deleted') NOT VALID`,
		"the audit":  `ALTER TABLE events ADD CONSTRAINT no_audit CHECK (type <> 'admin.action') NOT VALID`,
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			f := newCommentRoutes(t)
			top := f.say(t, f.item(t, feed.KindTrade), f.cabal.Members[0].ID, "top", nil)
			if _, err := f.pool.Exec(t.Context(), ddl); err != nil {
				t.Fatal(err)
			}
			if err := f.removeAs(asAdmin(t), top.Id, "abuse"); errs.CodeOf(err) == "" {
				t.Fatalf("RemoveComment = %v, want an error", err)
			}
			if n := len(f.deleted(t)) + len(f.audits(t)); n != 0 {
				t.Errorf("%d events after the failure, want 0", n)
			}
		})
	}
}
