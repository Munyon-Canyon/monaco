package cabal_test

import (
	"context"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/cabal/app"
	api "github.com/monaco/monaco/apps/backend/internal/platform/httpx/api/cabalapi"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

func (f accessFixture) request(ctx context.Context, user ids.UserID, c ids.CabalID) (app.Access, error) {
	return app.NewRequestAccessHandler(f.uow, f.ids, f.clock).Handle(as(ctx, user),
		app.RequestAccess{ActorID: user, CabalID: c})
}

func (f accessFixture) revoke(ctx context.Context, user ids.UserID, c ids.CabalID, id uuid.UUID) (app.Access, error) {
	return app.NewRevokeAccessHandler(f.uow, f.clock).Handle(as(ctx, user), app.RevokeAccess{
		ActorID: user, CabalID: c, RequestID: ids.AccessRequestIDFrom(id),
	})
}

func (f accessFixture) filed(t *testing.T, user ids.UserID, c ids.CabalID) uuid.UUID {
	t.Helper()
	filed, err := f.request(t.Context(), user, c)
	if err != nil {
		t.Fatal(err)
	}
	return filed.ID
}

func (f accessFixture) invite(t *testing.T, c ids.CabalID, invitee, inviter ids.UserID) uuid.UUID {
	t.Helper()
	id := ids.Real{}.NewV7()
	f.exec(t, `INSERT INTO cabal_access_requests (id, cabal_id, user_id, direction, invited_by, expires_at, created_at)
		VALUES ($1, $2, $3, 'invite', $4, $5, $6)`, id, c.UUID(), invitee.UUID(), inviter.UUID(),
		f.clock.Now().Add(7*24*time.Hour), f.clock.Now())
	return id
}

type accessRow struct {
	userID, direction, status string
	decidedBy                 *string
}

func (f accessFixture) accessRow(t *testing.T, id uuid.UUID) accessRow {
	t.Helper()
	var r accessRow
	if err := f.pool.QueryRow(t.Context(), `SELECT user_id::text, direction, status, decided_by::text
		FROM cabal_access_requests WHERE id = $1`, id).Scan(&r.userID, &r.direction, &r.status, &r.decidedBy); err != nil {
		t.Fatal(err)
	}
	return r
}

func TestRequestAccess_filesAPendingRequestAndAppendsIt(t *testing.T) {
	t.Parallel()
	f := newAccess(t)
	c := testkit.NewCabal(t, f.pool, testkit.WithJoinMode("request"))
	asker := f.user(t)
	filed, err := f.request(t.Context(), asker, c.ID)
	if err != nil || filed.Direction != "request" || filed.Status != "pending" {
		t.Fatalf("RequestAccess = %+v, %v; want a pending request", filed, err)
	}
	if got := f.accessRow(t, filed.ID); got != (accessRow{asker.String(), "request", "pending", nil}) {
		t.Fatalf("row = %+v, want the asker's pending request", got)
	}
	want := events.CabalAccessRequested{
		V: 1, RequestID: filed.ID, CabalID: c.ID.UUID(), UserID: asker.UUID(), Direction: "request",
		ActorID: asker.UUID(),
	}
	got := decoded[events.CabalAccessRequested](t, f, events.TypeCabalAccessRequested)
	if len(got) != 1 || got[0] != want {
		t.Fatalf("access_requested events = %+v, want %+v", got, want)
	}
	if _, ok := f.membership(t, c.ID, asker); ok {
		t.Fatal("a request made the asker a member")
	}
}

func TestRequestAccess_refusesWithoutFilingAnything(t *testing.T) {
	t.Parallel()
	f := newAccess(t)
	request := testkit.NewCabal(t, f.pool, testkit.WithJoinMode("request"), testkit.WithMembers(2))
	open := testkit.NewCabal(t, f.pool)
	banned := testkit.NewCabal(t, f.pool, testkit.WithJoinMode("request"))
	f.exec(t, `UPDATE cabals SET status = 'banned' WHERE id = $1`, banned.ID.UUID())
	asker, invitee := f.user(t), f.user(t)
	f.filed(t, asker, request.ID)
	f.invite(t, request.ID, invitee, request.Creator.ID)
	for _, tt := range []struct {
		name  string
		user  ids.UserID
		cabal ids.CabalID
		want  errs.Code
	}{
		{"an unknown cabal", asker, ids.CabalIDFrom(ids.Real{}.NewV7()), errs.CodeCabalNotFound},
		{"a banned cabal", asker, banned.ID, errs.CodeCabalBanned},
		{"a member", request.Members[1].ID, request.ID, errs.CodeAlreadyMember},
		{"an open cabal", asker, open.ID, errs.CodeRequestNotNeeded},
		{"a second request", asker, request.ID, errs.CodeRequestPending},
		{"a pending invite", invitee, request.ID, errs.CodeRequestPending},
	} {
		if _, err := f.request(t.Context(), tt.user, tt.cabal); errs.CodeOf(err) != tt.want {
			t.Errorf("%s: err = %v, want %s", tt.name, err, tt.want)
		}
	}
	if got := decoded[events.CabalAccessRequested](t, f, events.TypeCabalAccessRequested); len(got) != 1 {
		t.Fatalf("access_requested events = %d, want only the first request", len(got))
	}
}

func TestRequestAccess_letsADeniedOrRevokedUserAskAgain(t *testing.T) {
	t.Parallel()
	f := newAccess(t)
	c := testkit.NewCabal(t, f.pool, testkit.WithJoinMode("request"))
	asker := f.user(t)
	first := f.filed(t, asker, c.ID)
	if _, err := f.revoke(t.Context(), asker, c.ID, first); err != nil {
		t.Fatal(err)
	}
	second := f.filed(t, asker, c.ID)
	if second == first || f.accessRow(t, second).status != "pending" {
		t.Fatalf("second request %s after revoking %s is not a new pending row", second, first)
	}
}

func TestRequestAccess_wrapsAFailedInsertAsInternal(t *testing.T) {
	t.Parallel()
	f := newAccess(t)
	c := testkit.NewCabal(t, f.pool, testkit.WithJoinMode("request"))
	f.exec(t, `ALTER TABLE cabal_access_requests ADD CONSTRAINT no_requests CHECK (direction <> 'request')`)
	_, err := f.request(t.Context(), f.user(t), c.ID)
	wantErr(t, err, errs.CodeInternal)
}

func TestRevokeAccess_withdrawsTheRequestersPendingRequest(t *testing.T) {
	t.Parallel()
	f := newAccess(t)
	c := testkit.NewCabal(t, f.pool, testkit.WithJoinMode("request"))
	asker := f.user(t)
	id := f.filed(t, asker, c.ID)
	revoked, err := f.revoke(t.Context(), asker, c.ID, id)
	if err != nil || revoked != (app.Access{ID: id, Direction: "request", Status: "revoked"}) {
		t.Fatalf("RevokeAccess = %+v, %v; want the request revoked", revoked, err)
	}
	by := asker.String()
	if got := f.accessRow(t, id); got.status != "revoked" || got.decidedBy == nil || *got.decidedBy != by {
		t.Fatalf("row = %+v, want revoked by the asker", got)
	}
	want := events.CabalAccessDecided{
		V: 1, RequestID: id, CabalID: c.ID.UUID(), UserID: asker.UUID(), Direction: "request",
		Decision: "revoked", ActorID: asker.UUID(),
	}
	if got := decoded[events.CabalAccessDecided](t, f, events.TypeCabalAccessDecided); len(got) != 1 || got[0] != want {
		t.Fatalf("access_decided events = %+v, want %+v", got, want)
	}
}

func TestRevokeAccess_letsTheInviterOrTheCreatorWithdrawAnInvite(t *testing.T) {
	t.Parallel()
	f := newAccess(t)
	c := testkit.NewCabal(t, f.pool, testkit.WithMembers(2))
	inviter := c.Members[1].ID
	for _, by := range []ids.UserID{inviter, c.Creator.ID} {
		invite := f.invite(t, c.ID, f.user(t), inviter)
		if got, err := f.revoke(t.Context(), by, c.ID, invite); err != nil || got.Status != "revoked" {
			t.Fatalf("revoke by %s = %+v, %v", by, got, err)
		}
	}
}

func TestRevokeAccess_refusesWithoutDecidingAnything(t *testing.T) {
	t.Parallel()
	f := newAccess(t)
	c := testkit.NewCabal(t, f.pool, testkit.WithJoinMode("request"))
	other := testkit.NewCabal(t, f.pool, testkit.WithJoinMode("request"))
	asker, done := f.user(t), f.user(t)
	pending := f.filed(t, asker, c.ID)
	elsewhere := f.filed(t, asker, other.ID)
	settled := f.filed(t, done, c.ID)
	if _, err := f.revoke(t.Context(), done, c.ID, settled); err != nil {
		t.Fatal(err)
	}
	for _, tt := range []struct {
		name    string
		user    ids.UserID
		cabal   ids.CabalID
		request uuid.UUID
		want    errs.Code
	}{
		{"an unknown cabal", asker, ids.CabalIDFrom(ids.Real{}.NewV7()), pending, errs.CodeCabalNotFound},
		{"an unknown request", asker, c.ID, ids.Real{}.NewV7(), errs.CodeNotFound},
		{"a request in another cabal", asker, c.ID, elsewhere, errs.CodeNotFound},
		{"the creator on someone's request", c.Creator.ID, c.ID, pending, errs.CodeCannotRevokeAccess},
		{"a request no longer pending", done, c.ID, settled, errs.CodeAccessRequestNotPending},
	} {
		if _, err := f.revoke(t.Context(), tt.user, tt.cabal, tt.request); errs.CodeOf(err) != tt.want {
			t.Errorf("%s: err = %v, want %s", tt.name, err, tt.want)
		}
	}
	if got := decoded[events.CabalAccessDecided](t, f, events.TypeCabalAccessDecided); len(got) != 1 {
		t.Fatalf("access_decided events = %d, want only the setup revoke", len(got))
	}
}

func TestRevokeAccess_losesARaceToAConcurrentDecisionAsNotPending(t *testing.T) {
	t.Parallel()
	f := newAccess(t)
	c := testkit.NewCabal(t, f.pool, testkit.WithJoinMode("request"))
	asker := f.user(t)
	id := f.filed(t, asker, c.ID)
	commit := f.holdWrite(t, `UPDATE cabal_access_requests SET status = 'denied', decided_by = $2, decided_at = now()
		WHERE id = $1`, id, c.Creator.ID.UUID())
	done := make(chan error, 1)
	var revoking sync.WaitGroup
	t.Cleanup(revoking.Wait)
	revoking.Go(func() {
		_, err := f.revoke(t.Context(), asker, c.ID, id)
		done <- err
	})
	f.waitForALockWaiter(t)
	commit()
	wantErr(t, <-done, errs.CodeAccessRequestNotPending)
	if got := f.accessRow(t, id).status; got != "denied" {
		t.Fatalf("status = %s, want the concurrent denial to stand", got)
	}
}

func TestRevokeAccess_wrapsStoreFailuresAsInternal(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name      string
		statement string
	}{
		{"the request lookup", `ALTER TABLE cabal_access_requests RENAME TO requests_gone`},
		{"the guarded update", `ALTER TABLE cabal_access_requests ADD CONSTRAINT no_revokes CHECK (status <> 'revoked')`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			f := newAccess(t)
			c := testkit.NewCabal(t, f.pool, testkit.WithJoinMode("request"))
			asker := f.user(t)
			id := f.filed(t, asker, c.ID)
			f.exec(t, tt.statement)
			_, err := f.revoke(t.Context(), asker, c.ID, id)
			wantErr(t, err, errs.CodeInternal)
		})
	}
}

func TestListAccessRequests_showsTheCreatorPendingRequestsOldestFirst(t *testing.T) {
	t.Parallel()
	f := newAccess(t)
	c := testkit.NewCabal(t, f.pool, testkit.WithJoinMode("request"))
	first := testkit.SeedUser(t, f.pool, testkit.UserOpts{Handle: "first"}).ID
	second, withdrawn, invitee := f.user(t), f.user(t), f.user(t)
	firstID := f.filed(t, first, c.ID)
	f.clock.Advance(time.Minute)
	secondID := f.filed(t, second, c.ID)
	gone := f.filed(t, withdrawn, c.ID)
	if _, err := f.revoke(t.Context(), withdrawn, c.ID, gone); err != nil {
		t.Fatal(err)
	}
	f.invite(t, c.ID, invitee, c.Creator.ID)
	got, err := app.ListAccessRequests(t.Context(), f.pool, f.users, c.ID, c.Creator.ID)
	if err != nil {
		t.Fatal(err)
	}
	ids := make([]uuid.UUID, 0, len(got))
	for _, p := range got {
		ids = append(ids, p.ID)
	}
	if !slices.Equal(ids, []uuid.UUID{firstID, secondID}) || got[0].User.UserID != first.UUID() ||
		got[0].User.Handle != "first" || !got[0].CreatedAt.Before(got[1].CreatedAt) {
		t.Fatalf("pending = %+v, want the two pending requests oldest first with the first asker's card", got)
	}
}

func TestListAccessRequests_refusesAnyoneButTheCreatorAndWrapsStoreFailures(t *testing.T) {
	t.Parallel()
	f := newAccess(t)
	c := testkit.NewCabal(t, f.pool, testkit.WithJoinMode("request"), testkit.WithMembers(2))
	f.filed(t, f.user(t), c.ID)
	list := func(users app.UserCards, cabal ids.CabalID, actor ids.UserID) error {
		_, err := app.ListAccessRequests(t.Context(), f.pool, users, cabal, actor)
		return err
	}
	wantErr(t, list(f.users, ids.CabalIDFrom(ids.Real{}.NewV7()), c.Creator.ID), errs.CodeCabalNotFound)
	wantErr(t, list(f.users, c.ID, c.Members[1].ID), errs.CodeNotCabalCreator)
	wantErr(t, list(failCards{}, c.ID, c.Creator.ID), errs.CodeInternal)
	f.exec(t, `ALTER TABLE cabal_access_requests RENAME TO requests_gone`)
	wantErr(t, list(f.users, c.ID, c.Creator.ID), errs.CodeInternal)
	f.exec(t, `ALTER TABLE cabals RENAME TO cabals_gone`)
	wantErr(t, list(f.users, c.ID, c.Creator.ID), errs.CodeInternal)
}

func TestAccessRequestRoutes_wireTheCommandsAndTheList(t *testing.T) {
	t.Parallel()
	f := newAccess(t)
	c := testkit.NewCabal(t, f.pool, testkit.WithJoinMode("request"))
	asker := f.user(t)
	routes := f.routes(nil)
	posted, err := routes.PostCabalAccessRequest(as(t.Context(), asker),
		api.PostCabalAccessRequestRequestObject{Id: c.ID.UUID()})
	filed, ok := posted.(api.PostCabalAccessRequest201JSONResponse)
	if err != nil || !ok || filed.Direction != "request" || filed.Status != "pending" {
		t.Fatalf("PostCabalAccessRequest = %+v, %v", posted, err)
	}
	listed, err := routes.GetCabalAccessRequests(as(t.Context(), c.Creator.ID),
		api.GetCabalAccessRequestsRequestObject{Id: c.ID.UUID()})
	want := api.GetCabalAccessRequests200JSONResponse{{
		Id: filed.Id, User: api.CabalPerson{UserId: asker.UUID()}, CreatedAt: f.clock.Now(),
	}}
	if pending, ok := listed.(api.GetCabalAccessRequests200JSONResponse); err != nil || !ok ||
		len(pending) != 1 || pending[0].Id != want[0].Id || pending[0].User.UserId != want[0].User.UserId {
		t.Fatalf("GetCabalAccessRequests = %+v, %v; want %+v", listed, err, want)
	}
	deleted, err := routes.DeleteCabalAccessRequest(as(t.Context(), asker),
		api.DeleteCabalAccessRequestRequestObject{Id: c.ID.UUID(), RequestId: filed.Id})
	wantRevoked := api.DeleteCabalAccessRequest200JSONResponse{Id: filed.Id, Direction: "request", Status: "revoked"}
	if err != nil || deleted != wantRevoked {
		t.Fatalf("DeleteCabalAccessRequest = %+v, %v; want %+v", deleted, err, wantRevoked)
	}
}

func TestAccessRequestRoutes_passErrorsThrough(t *testing.T) {
	t.Parallel()
	f := newAccess(t)
	open := testkit.NewCabal(t, f.pool)
	routes, user := f.routes(nil), f.user(t)
	post := api.PostCabalAccessRequestRequestObject{Id: open.ID.UUID()}
	get := api.GetCabalAccessRequestsRequestObject{Id: open.ID.UUID()}
	del := api.DeleteCabalAccessRequestRequestObject{Id: open.ID.UUID(), RequestId: ids.Real{}.NewV7()}
	for _, tt := range []struct {
		name string
		call func(context.Context) error
		code errs.Code
	}{
		{
			"post", func(ctx context.Context) error { _, err := routes.PostCabalAccessRequest(ctx, post); return err },
			errs.CodeRequestNotNeeded,
		},
		{
			"get", func(ctx context.Context) error { _, err := routes.GetCabalAccessRequests(ctx, get); return err },
			errs.CodeNotCabalCreator,
		},
		{
			"delete", func(ctx context.Context) error { _, err := routes.DeleteCabalAccessRequest(ctx, del); return err },
			errs.CodeNotFound,
		},
	} {
		if err := tt.call(t.Context()); errs.CodeOf(err) != errs.CodeUnauthorized {
			t.Errorf("%s without a caller: %v, want unauthorized", tt.name, err)
		}
		if err := tt.call(as(t.Context(), user)); errs.CodeOf(err) != tt.code {
			t.Errorf("%s: %v, want %s", tt.name, err, tt.code)
		}
	}
}
