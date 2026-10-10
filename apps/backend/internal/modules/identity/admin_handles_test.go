package identity_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity/adapters"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity/app"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
	"github.com/monaco/monaco/apps/backend/internal/testkit/fakes"
)

func (f adminFixture) handleOf(t *testing.T, user ids.UserID) *string {
	t.Helper()
	var handle *string
	row := f.pool.QueryRow(t.Context(), `SELECT handle FROM users WHERE id = $1`, user.UUID())
	if err := row.Scan(&handle); err != nil {
		t.Fatal(err)
	}
	return handle
}

func (f adminFixture) profileUpdates(t *testing.T) map[uuid.UUID]string {
	t.Helper()
	out := map[uuid.UUID]string{}
	for _, raw := range f.payloads(t, events.TypeUserProfileUpdated) {
		var e events.UserProfileUpdated
		if err := json.Unmarshal(raw, &e); err != nil {
			t.Fatal(err)
		}
		if len(e.Fields) != 1 || e.Fields[0] != "handle" {
			t.Fatalf("user.profile_updated fields = %v, want [handle]", e.Fields)
		}
		out[e.UserID] = e.Handle
	}
	return out
}

func (f adminFixture) revoke(t *testing.T, user ids.UserID, key, reason string) *httptest.ResponseRecorder {
	t.Helper()
	return f.moderate(t, "handle/revoke", user, key, reason)
}

func (f adminFixture) reassign(
	t *testing.T, handle string, to ids.UserID, key, reason string,
) *httptest.ResponseRecorder {
	t.Helper()
	body := `{"to_user_id":"` + to.String() + `","reason":"` + reason + `"}`
	return f.call(t, moderatorToken, http.MethodPost, "/v1/admin/handles/"+handle+"/reassign", key, body)
}

func (f adminFixture) clearHandle(t *testing.T, user ids.UserID) {
	t.Helper()
	if _, err := f.pool.Exec(t.Context(), `UPDATE users SET handle = NULL WHERE id = $1`, user.UUID()); err != nil {
		t.Fatal(err)
	}
}

func TestAdminRevokeHandle_NullsHandle(t *testing.T) {
	t.Parallel()
	f := newAdminFixture(t)
	user := f.seed(t, portSeed{handle: "mallow", wallet: true})
	other := f.seed(t, portSeed{handle: "someone", wallet: true})
	if rec := f.revoke(t, user.ID, "r1", "impersonation"); rec.Code != http.StatusNoContent {
		t.Fatalf("revoke = %d %s, want 204", rec.Code, rec.Body)
	}
	if got := f.handleOf(t, user.ID); got != nil {
		t.Fatalf("handle = %q, want null", *got)
	}
	if got := f.profileUpdates(t); len(got) != 1 || got[user.ID.UUID()] != "" {
		t.Fatalf("user.profile_updated = %v, want one clearing the handle", got)
	}
	audit := f.audits(t, events.AdminActionHandleRevoke)
	if len(audit) != 1 || audit[0].TargetID != user.ID.String() || audit[0].TargetType != events.AdminTargetUser ||
		audit[0].Reason != "impersonation" || !sameJSON(audit[0].Before, `{"handle":"mallow"}`) ||
		!sameJSON(audit[0].After, `{"handle":null}`) {
		t.Fatalf("admin.action = %+v, want one handle_revoke of mallow", audit)
	}
	f.claim(t, other, "claim-1", "mallow")
	f.claim(t, user, "claim-2", "fresh_start")
}

func (f adminFixture) claim(t *testing.T, user testkit.SeededUser, key, handle string) {
	t.Helper()
	token := fakes.PrivyAccessToken(privyAppID, user.PrivyUserID, f.clock.Now(), time.Hour)
	rec := f.call(t, token, http.MethodPut, "/v1/me/handle", key, `{"handle":"`+handle+`"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("claiming %s = %d %s, want 200", handle, rec.Code, rec.Body)
	}
}

func TestAdminRevokeHandle_Refusals(t *testing.T) {
	t.Parallel()
	f := newAdminFixture(t)
	held := f.seed(t, portSeed{handle: "held"})
	bare := f.seed(t, portSeed{handle: "bare"})
	f.clearHandle(t, bare.ID)
	deleted := f.seed(t, portSeed{handle: "gone", status: "deleted", softDeleted: true})
	missing := ids.UserIDFrom(testkit.NewIDs(7).NewV7())
	for name, tc := range map[string]struct {
		user   ids.UserID
		reason string
		status int
		code   errs.Code
	}{
		"deleted":      {deleted.ID, "cleanup", http.StatusConflict, errs.CodeUserDeleted},
		"unknown":      {missing, "cleanup", http.StatusNotFound, errs.CodeUserNotFound},
		"short reason": {held.ID, "no", http.StatusBadRequest, errs.CodeReasonRequired},
	} {
		rec := f.revoke(t, tc.user, "k-"+name, tc.reason)
		if rec.Code != tc.status || problemCode(t, rec) != apiCode(tc.code) {
			t.Errorf("%s: %d %s, want %d %s", name, rec.Code, rec.Body, tc.status, tc.code)
		}
	}
	if rec := f.revoke(t, bare.ID, "k-bare", "cleanup"); rec.Code != http.StatusNoContent {
		t.Errorf("revoke of a user with no handle = %d %s, want 204", rec.Code, rec.Body)
	}
	if n := len(f.payloads(t, events.TypeAdminAction)) + len(f.payloads(t, events.TypeUserProfileUpdated)); n != 0 {
		t.Errorf("%d events after refusals and a no-op, want 0", n)
	}
	path := "/v1/admin/users/" + held.ID.String() + "/handle/revoke"
	rec := f.call(t, viewerToken, http.MethodPost, path, "k-viewer", `{"reason":"cleanup"}`)
	if rec.Code != http.StatusForbidden {
		t.Errorf("revoke as a viewer = %d %s, want 403", rec.Code, rec.Body)
	}
	if got := f.handleOf(t, held.ID); got == nil || *got != "held" {
		t.Errorf("handle = %v after refusals, want held", got)
	}
}

func TestAdminReassignHandle_MovesHandle(t *testing.T) {
	t.Parallel()
	f := newAdminFixture(t)
	holder := f.seed(t, portSeed{handle: "kai_one"})
	owner := f.seed(t, portSeed{handle: "kai_real"})
	if rec := f.reassign(t, "KAI_ONE", owner.ID, "m1", "verified owner"); rec.Code != http.StatusNoContent {
		t.Fatalf("reassign of a held handle = %d %s, want 204", rec.Code, rec.Body)
	}
	if got := f.handleOf(t, holder.ID); got != nil {
		t.Errorf("former holder handle = %q, want null", *got)
	}
	if got := f.handleOf(t, owner.ID); got == nil || *got != "kai_one" {
		t.Errorf("owner handle = %v, want kai_one", got)
	}
	f.expectMoved(t, holder.ID, owner.ID)
	if rec := f.reassign(t, "kai_one", owner.ID, "m2", "verified owner"); rec.Code != http.StatusNoContent {
		t.Fatalf("second reassign = %d %s, want 204", rec.Code, rec.Body)
	}
	if n := len(f.audits(t, events.AdminActionHandleReassign)); n != 1 {
		t.Errorf("%d handle_reassign events after reassigning to the holder, want 1", n)
	}
}

func (f adminFixture) expectMoved(t *testing.T, holder, owner ids.UserID) {
	t.Helper()
	updates := f.profileUpdates(t)
	if len(updates) != 2 || updates[holder.UUID()] != "" || updates[owner.UUID()] != "kai_one" {
		t.Errorf("user.profile_updated = %v, want the holder cleared and the owner set", updates)
	}
	audit := f.audits(t, events.AdminActionHandleReassign)
	if len(audit) != 1 || audit[0].TargetID != "kai_one" || audit[0].TargetType != events.AdminTargetHandle ||
		!sameJSON(audit[0].Before, `{"from_user_id":"`+holder.String()+`"}`) ||
		!sameJSON(audit[0].After, `{"to_user_id":"`+owner.String()+`"}`) {
		t.Fatalf("admin.action = %+v, want one handle_reassign from the holder to the owner", audit)
	}
}

func TestAdminReassignHandle_FreeHandle(t *testing.T) {
	t.Parallel()
	f := newAdminFixture(t)
	owner := f.seed(t, portSeed{handle: "oldname"})
	if rec := f.reassign(t, "brand_new", owner.ID, "m1", "verified owner"); rec.Code != http.StatusNoContent {
		t.Fatalf("reassign of a free handle = %d %s, want 204", rec.Code, rec.Body)
	}
	if got := f.handleOf(t, owner.ID); got == nil || *got != "brand_new" {
		t.Errorf("owner handle = %v, want brand_new", got)
	}
	if got := f.profileUpdates(t); len(got) != 1 || got[owner.ID.UUID()] != "brand_new" {
		t.Errorf("user.profile_updated = %v, want one for the owner", got)
	}
	audit := f.audits(t, events.AdminActionHandleReassign)
	if len(audit) != 1 || !sameJSON(audit[0].Before, `{}`) {
		t.Fatalf("admin.action = %+v, want one handle_reassign with no former holder", audit)
	}
	var taken bool
	row := f.pool.QueryRow(t.Context(), `SELECT EXISTS (SELECT 1 FROM users WHERE handle = 'oldname')`)
	if err := row.Scan(&taken); err != nil || taken {
		t.Errorf("the owner's old handle is still held (%v, %v), want it freed", taken, err)
	}
}

func TestAdminReassignHandle_Refusals(t *testing.T) {
	t.Parallel()
	f := newAdminFixture(t)
	owner := f.seed(t, portSeed{handle: "owner"})
	deleted := f.seed(t, portSeed{handle: "gone", status: "deleted", softDeleted: true})
	missing := ids.UserIDFrom(testkit.NewIDs(7).NewV7())
	for name, tc := range map[string]struct {
		handle string
		to     ids.UserID
		reason string
		status int
		code   errs.Code
	}{
		"deleted target": {"fresh", deleted.ID, "verified", http.StatusConflict, errs.CodeUserDeleted},
		"unknown target": {"fresh", missing, "verified", http.StatusNotFound, errs.CodeUserNotFound},
		"bad handle":     {"bad-handle", owner.ID, "verified", http.StatusBadRequest, errs.CodeHandleInvalid},
		"short handle":   {"ab", owner.ID, "verified", http.StatusBadRequest, errs.CodeHandleInvalid},
		"short reason":   {"fresh", owner.ID, "no", http.StatusBadRequest, errs.CodeReasonRequired},
	} {
		rec := f.reassign(t, tc.handle, tc.to, "k-"+name, tc.reason)
		if rec.Code != tc.status || problemCode(t, rec) != apiCode(tc.code) {
			t.Errorf("%s: %d %s, want %d %s", name, rec.Code, rec.Body, tc.status, tc.code)
		}
	}
	path := "/v1/admin/handles/fresh/reassign"
	body := `{"to_user_id":"` + owner.ID.String() + `","reason":"verified"}`
	if rec := f.call(t, viewerToken, http.MethodPost, path, "k-viewer", body); rec.Code != http.StatusForbidden {
		t.Errorf("reassign as a viewer = %d %s, want 403", rec.Code, rec.Body)
	}
	if n := len(f.payloads(t, events.TypeAdminAction)) + len(f.payloads(t, events.TypeUserProfileUpdated)); n != 0 {
		t.Errorf("%d events after refusals, want 0", n)
	}
}

func TestAdminHandles_NeedAnAdminActor(t *testing.T) {
	t.Parallel()
	f := newAdminFixture(t)
	user := f.seed(t, portSeed{handle: "target"})
	handles := f.handles()
	reason, err := events.NewReason("cleanup")
	if err != nil {
		t.Fatal(err)
	}
	for name, ctx := range map[string]context.Context{"no actor": t.Context(), "a user": userCtx(t, user.ID)} {
		if err := handles.Revoke(ctx, app.RevokeHandle{UserID: user.ID, Reason: reason}); err == nil {
			t.Errorf("%s: Revoke = nil, want an error", name)
		}
		cmd := app.ReassignHandle{Handle: "abc", ToUserID: user.ID, Reason: reason}
		if err := handles.Reassign(ctx, cmd); err == nil {
			t.Errorf("%s: Reassign = nil, want an error", name)
		}
	}
}

func (f adminFixture) handles() *app.AdminHandles {
	return app.NewAdminHandles(app.AdminHandlesDeps{
		Users: adapters.Users{}, UoW: db.New(f.pool, f.ids, f.clock), IDs: f.ids, Clock: f.clock,
		Hints: &recordedHints{},
	})
}

func TestAdminHandles_RollBackWhenAWriteFails(t *testing.T) {
	t.Parallel()
	trigger := func(cond, code string) string {
		return `CREATE FUNCTION fail_write() RETURNS trigger LANGUAGE plpgsql AS
			$$ BEGIN IF ` + cond + ` THEN RAISE EXCEPTION 'no' USING ERRCODE = '` + code + `'; END IF; RETURN NEW; END $$;
			CREATE TRIGGER fail_write BEFORE UPDATE ON users FOR EACH ROW EXECUTE FUNCTION fail_write()`
	}
	for name, ddl := range map[string]string{
		"the clear":         trigger("NEW.handle IS NULL", "P0001"),
		"the assignment":    trigger("NEW.handle IS NOT NULL", "P0001"),
		"a lost race":       trigger("NEW.handle IS NOT NULL", "23505"),
		"the profile event": `ALTER TABLE events ADD CONSTRAINT no_event CHECK (type <> 'user.profile_updated') NOT VALID`,
		"the audit":         `ALTER TABLE events ADD CONSTRAINT no_audit CHECK (type <> 'admin.action') NOT VALID`,
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			f := newAdminFixture(t)
			holder := f.seed(t, portSeed{handle: "holder"})
			owner := f.seed(t, portSeed{handle: "owner"})
			if _, err := f.pool.Exec(t.Context(), ddl); err != nil {
				t.Fatal(err)
			}
			reason, _ := events.NewReason("cleanup")
			handles := f.handles()
			cmd := app.ReassignHandle{Handle: "holder", ToUserID: owner.ID, Reason: reason}
			err := handles.Reassign(adminCtx(t), cmd)
			if err == nil || (name == "a lost race" && errs.CodeOf(err) != errs.CodeHandleTaken) {
				t.Errorf("Reassign = %v, want an error (handle_taken on a lost race)", err)
			}
			if handles.Revoke(adminCtx(t), app.RevokeHandle{UserID: holder.ID, Reason: reason}) == nil {
				return
			}
			f.expectHandle(t, holder.ID, "holder")
			f.expectHandle(t, owner.ID, "owner")
		})
	}
}

func (f adminFixture) expectHandle(t *testing.T, user ids.UserID, want string) {
	t.Helper()
	if _, err := f.pool.Exec(t.Context(), `DROP TRIGGER IF EXISTS fail_write ON users`); err != nil {
		t.Fatal(err)
	}
	if got := f.handleOf(t, user); got == nil || *got != want {
		t.Errorf("handle = %v after the failures, want %s", got, want)
	}
}

func TestAdminHandles_RefuseAMissingReasonAndAnUnreadableTable(t *testing.T) {
	t.Parallel()
	f := newAdminFixture(t)
	holder := f.seed(t, portSeed{handle: "holder"})
	owner := f.seed(t, portSeed{handle: "owner"})
	handles := f.handles()
	err := handles.Revoke(adminCtx(t), app.RevokeHandle{UserID: holder.ID})
	if errs.CodeOf(err) != errs.CodeReasonRequired {
		t.Errorf("Revoke without a reason = %v, want %s", err, errs.CodeReasonRequired)
	}
	err = handles.Reassign(adminCtx(t), app.ReassignHandle{Handle: "free_one", ToUserID: owner.ID})
	if errs.CodeOf(err) != errs.CodeReasonRequired {
		t.Errorf("Reassign without a reason = %v, want %s", err, errs.CodeReasonRequired)
	}
	reason, _ := events.NewReason("cleanup")
	ddl := `ALTER TABLE events ADD CONSTRAINT no_event CHECK (type <> 'user.profile_updated') NOT VALID`
	if _, err := f.pool.Exec(t.Context(), ddl); err != nil {
		t.Fatal(err)
	}
	err = handles.Reassign(adminCtx(t), app.ReassignHandle{Handle: "free_one", ToUserID: owner.ID, Reason: reason})
	if err == nil {
		t.Error("Reassign of a free handle with the event refused = nil, want an error")
	}
	if got := f.handleOf(t, owner.ID); got == nil || *got != "owner" {
		t.Errorf("owner handle = %v after the failure, want owner", got)
	}
	if _, err := f.pool.Exec(t.Context(), `ALTER TABLE users RENAME COLUMN handle TO nickname`); err != nil {
		t.Fatal(err)
	}
	err = handles.Reassign(adminCtx(t), app.ReassignHandle{Handle: "free_one", ToUserID: owner.ID, Reason: reason})
	if errs.CodeOf(err) != errs.CodeInternal {
		t.Errorf("Reassign on an unreadable table = %v, want %s", err, errs.CodeInternal)
	}
}
