package cabal_test

import (
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/cabal/app"
	"github.com/monaco/monaco/apps/backend/internal/platform/auth"
	api "github.com/monaco/monaco/apps/backend/internal/platform/httpx/api/cabalapi"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

func (f accessFixture) makeAdmin(t *testing.T, c ids.CabalID, user ids.UserID) {
	t.Helper()
	f.exec(t, `UPDATE cabal_members SET role = 'admin', can_vote = true WHERE cabal_id = $1 AND user_id = $2`,
		c.UUID(), user.UUID())
}

func TestAdminRole_anAdminListsAndApprovesRequests(t *testing.T) {
	t.Parallel()
	f := newAccess(t)
	c := testkit.NewCabal(t, f.pool, testkit.WithJoinMode("request"), testkit.WithMembers(3))
	admin := c.Members[1].ID
	f.makeAdmin(t, c.ID, admin)
	asker := f.user(t)
	filed := f.filed(t, asker, c.ID)

	pending, err := app.ListAccessRequests(t.Context(), f.pool, f.users, c.ID, admin)
	if err != nil || len(pending) != 1 || pending[0].ID != filed {
		t.Fatalf("ListAccessRequests as an admin = %+v, %v; want the one request", pending, err)
	}
	if _, err := f.decide(t.Context(), admin, c.ID, filed, app.Approve); err != nil {
		t.Fatalf("an admin approving a request: %v", err)
	}
	if m, ok := f.membership(t, c.ID, asker); !ok || m.role != "member" {
		t.Fatalf("membership of the approved asker = %+v, %v; want a member", m, ok)
	}
}

func TestAdminRole_anAdminInvitesAndRevokesTheirOwnAndTheCreatorsInvite(t *testing.T) {
	t.Parallel()
	f := newAccess(t)
	c := testkit.NewCabal(t, f.pool, testkit.WithJoinMode("request"), testkit.WithMembers(2))
	admin := c.Members[1].ID
	f.makeAdmin(t, c.ID, admin)
	_, handle := f.handled(t)
	sent, err := f.sendInvite(t.Context(), admin, c.ID, handle)
	if err != nil || sent.Status != "pending" {
		t.Fatalf("an admin's invite = %+v, %v; want a pending invite", sent, err)
	}
	if revoked, err := f.revoke(t.Context(), admin, c.ID, sent.ID); err != nil || revoked.Status != "revoked" {
		t.Fatalf("an admin revoking their own invite = %+v, %v", revoked, err)
	}
	byCreator := f.invite(t, c.ID, f.user(t), c.Creator.ID)
	if revoked, err := f.revoke(t.Context(), admin, c.ID, byCreator); err != nil || revoked.Status != "revoked" {
		t.Fatalf("an admin revoking the creator's invite = %+v, %v; want it revoked", revoked, err)
	}
}

func TestAdminRole_aPlainMemberIsRefusedTheDoorButRevokesTheInviteTheySent(t *testing.T) {
	t.Parallel()
	f := newAccess(t)
	c := testkit.NewCabal(t, f.pool, testkit.WithJoinMode("request"), testkit.WithMembers(3))
	member := c.Members[1].ID
	filed := f.filed(t, f.user(t), c.ID)
	_, handle := f.handled(t)

	_, err := app.ListAccessRequests(t.Context(), f.pool, f.users, c.ID, member)
	wantErr(t, err, errs.CodeNotCabalAdmin)
	_, err = f.decide(t.Context(), member, c.ID, filed, app.Approve)
	wantErr(t, err, errs.CodeNotCabalAdmin)
	_, err = f.sendInvite(t.Context(), member, c.ID, handle)
	wantErr(t, err, errs.CodeNotCabalAdmin)

	own := f.invite(t, c.ID, f.user(t), member)
	theirs := f.invite(t, c.ID, f.user(t), c.Creator.ID)
	_, err = f.revoke(t.Context(), member, c.ID, theirs)
	wantErr(t, err, errs.CodeCannotRevokeAccess)
	if revoked, err := f.revoke(t.Context(), member, c.ID, own); err != nil || revoked.Status != "revoked" {
		t.Fatalf("a member revoking the invite they sent = %+v, %v; want it revoked", revoked, err)
	}
}

func TestAdminRole_anAdminSeesTheRequestCountOnMyCabals(t *testing.T) {
	t.Parallel()
	f := newCreate(t)
	c := testkit.NewCabal(t, f.pool, testkit.WithJoinMode("request"), testkit.WithMembers(2))
	requestAccess(t, f, c.ID.UUID())
	if _, err := f.pool.Exec(t.Context(),
		`UPDATE cabal_members SET role = 'admin' WHERE cabal_id = $1 AND user_id = $2`,
		c.ID.UUID(), c.Members[1].ID.UUID()); err != nil {
		t.Fatal(err)
	}
	mine, err := f.routes(nil).GetMyCabals(auth.WithActor(t.Context(), auth.Actor{
		Kind: auth.ActorUser, ID: c.Members[1].ID.String(), Standing: auth.StandingActive,
	}), api.GetMyCabalsRequestObject{})
	if err != nil {
		t.Fatal(err)
	}
	items := []api.MyCabal(mine.(api.GetMyCabals200JSONResponse))
	if len(items) != 1 || items[0].Role != "admin" || items[0].PendingRequestCount != 1 {
		t.Fatalf("my cabals = %+v, want an admin role and one pending request", items)
	}
}

func TestAdminRole_theAdminListReadFailsInternalWhenTheRolesCannotBeRead(t *testing.T) {
	t.Parallel()
	f := newAccess(t)
	c := testkit.NewCabal(t, f.pool, testkit.WithJoinMode("request"), testkit.WithMembers(2))
	f.exec(t, `ALTER TABLE cabal_members RENAME COLUMN role TO role_gone`)
	_, err := app.ListAccessRequests(t.Context(), f.pool, f.users, c.ID, c.Creator.ID)
	wantErr(t, err, errs.CodeInternal)
}
