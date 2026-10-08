package cabal_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/cabal/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity"
	api "github.com/monaco/monaco/apps/backend/internal/platform/httpx/api/cabalapi"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

func (f accessFixture) inviteHandler() *app.InviteMemberHandler {
	return app.NewInviteMemberHandler(f.uow, identity.New(module.Deps{Pool: f.pool}).Queries(), f.ids, f.clock)
}

func (f accessFixture) sendInvite(
	ctx context.Context, inviter ids.UserID, c ids.CabalID, handle string,
) (app.Access, error) {
	return f.inviteHandler().Handle(as(ctx, inviter), app.InviteMember{ActorID: inviter, CabalID: c, Handle: handle})
}

func (f accessFixture) inviteTerms(t *testing.T, id uuid.UUID) (string, time.Time) {
	t.Helper()
	var invitedBy string
	var expiresAt time.Time
	if err := f.pool.QueryRow(t.Context(), `SELECT invited_by::text, expires_at FROM cabal_access_requests
		WHERE id = $1`, id).Scan(&invitedBy, &expiresAt); err != nil {
		t.Fatal(err)
	}
	return invitedBy, expiresAt.UTC()
}

func TestInviteMember_theCreatorFilesAnInviteThatLivesSevenDays(t *testing.T) {
	t.Parallel()
	f := newAccess(t)
	c := testkit.NewCabal(t, f.pool, testkit.WithMembers(2))
	inviter := c.Creator.ID
	invitee, handle := f.handled(t)
	sent, err := f.sendInvite(t.Context(), inviter, c.ID, strings.ToUpper(handle))
	if err != nil || sent.Direction != "invite" || sent.Status != "pending" {
		t.Fatalf("InviteMember = %+v, %v; want a pending invite", sent, err)
	}
	if got := f.accessRow(t, sent.ID); got != (accessRow{invitee.String(), "invite", "pending", nil}) {
		t.Fatalf("row = %+v, want the invitee's pending invite", got)
	}
	wantExpiry := f.clock.Now().Add(7 * 24 * time.Hour).UTC()
	if invitedBy, expiresAt := f.inviteTerms(t, sent.ID); invitedBy != inviter.String() || expiresAt != wantExpiry {
		t.Fatalf("invited_by %s expires_at %s, want %s and %s", invitedBy, expiresAt, inviter, wantExpiry)
	}
	want := events.CabalAccessRequested{
		V: 1, RequestID: sent.ID, CabalID: c.ID.UUID(), UserID: invitee.UUID(), Direction: "invite",
		ActorID: inviter.UUID(), ExpiresAt: wantExpiry,
	}
	got := decoded[events.CabalAccessRequested](t, f, events.TypeCabalAccessRequested)
	if len(got) == 1 {
		got[0].ExpiresAt = got[0].ExpiresAt.UTC()
	}
	if len(got) != 1 || got[0] != want {
		t.Fatalf("access_requested = %+v, want %+v", got, want)
	}
}

func TestInviteMember_acceptingACreatorsInviteJoinsARequestCabalWithoutApproval(t *testing.T) {
	t.Parallel()
	f := newAccess(t)
	c := testkit.NewCabal(t, f.pool, testkit.WithJoinMode("request"))
	invitee, handle := f.handled(t)
	sent, err := f.sendInvite(t.Context(), c.Creator.ID, c.ID, handle)
	if err != nil {
		t.Fatal(err)
	}
	if got, err := f.decide(t.Context(), invitee, c.ID, sent.ID, app.Approve); err != nil || got.Status != "approved" {
		t.Fatalf("accept = %+v, %v; want approved", got, err)
	}
	if m, ok := f.membership(t, c.ID, invitee); !ok || m.role != "member" {
		t.Fatalf("membership = %+v, %v; want the invitee a member", m, ok)
	}
	if joins := f.joins(t); len(joins) != 1 || joins[0].Via != "invite" || joins[0].RequestID != sent.ID {
		t.Fatalf("member_joined = %+v, want one join via the invite", joins)
	}
}

func TestInviteMember_refusesWithoutFilingAnything(t *testing.T) {
	t.Parallel()
	f := newAccess(t)
	open := testkit.NewCabal(t, f.pool, testkit.WithMembers(2))
	gated := testkit.NewCabal(t, f.pool, testkit.WithJoinMode("request"), testkit.WithMembers(2))
	banned := testkit.NewCabal(t, f.pool)
	f.exec(t, `UPDATE cabals SET status = 'banned' WHERE id = $1`, banned.ID.UUID())
	invitee, handle := f.handled(t)
	memberHandle := "m" + strings.ReplaceAll(ids.Real{}.NewV7().String(), "-", "")[14:]
	asker, askerHandle := f.handled(t)
	f.filed(t, asker, gated.ID)
	invited, invitedHandle := f.handled(t)
	f.invite(t, open.ID, invited, open.Creator.ID)
	f.exec(t, `UPDATE users SET handle = $1 WHERE id = $2`, memberHandle, open.Members[1].ID.UUID())
	outsider := f.user(t)
	for _, tt := range []struct {
		name    string
		inviter ids.UserID
		cabal   ids.CabalID
		handle  string
		want    errs.Code
	}{
		{"an unknown handle", open.Creator.ID, open.ID, "nobody_here", errs.CodeUserNotFound},
		{"an unknown cabal", open.Creator.ID, ids.CabalIDFrom(ids.Real{}.NewV7()), handle, errs.CodeCabalNotFound},
		{"an outsider", outsider, open.ID, handle, errs.CodeNotCabalMember},
		{"a member of a request cabal", gated.Members[1].ID, gated.ID, handle, errs.CodeNotCabalCreator},
		{"a banned cabal", banned.Creator.ID, banned.ID, handle, errs.CodeCabalBanned},
		{"an existing member", open.Creator.ID, open.ID, memberHandle, errs.CodeAlreadyMember},
		{"a user with a pending request", gated.Creator.ID, gated.ID, askerHandle, errs.CodeRequestPending},
		{"a user with a pending invite", open.Creator.ID, open.ID, invitedHandle, errs.CodeRequestPending},
	} {
		if _, err := f.sendInvite(t.Context(), tt.inviter, tt.cabal, tt.handle); errs.CodeOf(err) != tt.want {
			t.Errorf("%s: err = %v, want %s", tt.name, err, tt.want)
		}
	}
	if got := decoded[events.CabalAccessRequested](t, f, events.TypeCabalAccessRequested); len(got) != 1 {
		t.Fatalf("access_requested events = %d, want only the setup request", len(got))
	}
	if _, ok := f.membership(t, open.ID, invitee); ok {
		t.Fatal("a refused invite added a member")
	}
}

func TestInviteMember_wrapsStoreFailuresAsInternal(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name      string
		statement string
	}{
		{"the cabal lock", `ALTER TABLE cabal_members RENAME TO members_gone`},
		{"the invite insert", `ALTER TABLE cabal_access_requests ADD CONSTRAINT no_invites CHECK (direction <> 'invite')`},
		{"the event append", `ALTER TABLE events ADD CONSTRAINT no_requests CHECK (type <> 'cabal.access_requested')`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			f := newAccess(t)
			c := testkit.NewCabal(t, f.pool)
			_, handle := f.handled(t)
			f.exec(t, tt.statement)
			_, err := f.sendInvite(t.Context(), c.Creator.ID, c.ID, handle)
			wantErr(t, err, errs.CodeInternal)
		})
	}
}

func TestPostCabalInvite_needsACallerAndAHandleAndReturnsThePendingInvite(t *testing.T) {
	t.Parallel()
	f := newAccess(t)
	c := testkit.NewCabal(t, f.pool)
	_, handle := f.handled(t)
	routes := f.routes(nil)
	post := func(ctx context.Context, body *api.InviteMemberRequest) (api.PostCabalInviteResponseObject, error) {
		return routes.PostCabalInvite(ctx, api.PostCabalInviteRequestObject{Id: c.ID.UUID(), Body: body})
	}
	creator := as(t.Context(), c.Creator.ID)
	_, err := post(t.Context(), &api.InviteMemberRequest{Handle: handle})
	wantErr(t, err, errs.CodeUnauthorized)
	_, err = post(creator, nil)
	wantErr(t, err, errs.CodeInvalidInput)
	_, err = post(creator, &api.InviteMemberRequest{Handle: "  "})
	wantErr(t, err, errs.CodeInvalidInput)
	_, err = post(creator, &api.InviteMemberRequest{Handle: "nobody_here"})
	wantErr(t, err, errs.CodeUserNotFound)
	res, err := post(creator, &api.InviteMemberRequest{Handle: " " + handle + " "})
	sent, ok := res.(api.PostCabalInvite201JSONResponse)
	if err != nil || !ok || sent.Direction != "invite" || sent.Status != "pending" {
		t.Fatalf("PostCabalInvite = %+v, %v; want a pending invite", res, err)
	}
}
