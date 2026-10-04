package cabal_test

import (
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/cabal/app"
	api "github.com/monaco/monaco/apps/backend/internal/platform/httpx/api/cabalapi"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

func (f accessFixture) handled(t *testing.T) (ids.UserID, string) {
	t.Helper()
	handle := "h" + strings.ReplaceAll(ids.Real{}.NewV7().String(), "-", "")[14:]
	return testkit.SeedUser(t, f.pool, testkit.UserOpts{Handle: handle}).ID, handle
}

type inboxLine struct {
	id, cabal, inviter uuid.UUID
	name, picture      string
	members            int32
	expires            time.Time
}

func inboxLines(invites []app.ReceivedInvite) []inboxLine {
	out := make([]inboxLine, 0, len(invites))
	for _, r := range invites {
		line := inboxLine{
			id: r.ID, cabal: r.Cabal.ID, inviter: r.InvitedBy.UserID, name: r.Cabal.Name,
			members: r.Cabal.MemberCount, expires: r.ExpiresAt,
		}
		if r.Cabal.PictureURL != nil {
			line.picture = *r.Cabal.PictureURL
		}
		out = append(out, line)
	}
	return out
}

func TestListMyInvites_showsEachLiveInviteWithItsCabalAndInviterOldestFirst(t *testing.T) {
	t.Parallel()
	f := newAccess(t)
	first := testkit.NewCabal(t, f.pool, testkit.WithMembers(3))
	second := testkit.NewCabal(t, f.pool, testkit.WithJoinMode("request"))
	banned := testkit.NewCabal(t, f.pool)
	invitee := f.user(t)
	f.exec(t, `UPDATE cabals SET name = 'First pot' WHERE id = $1`, first.ID.UUID())
	f.exec(t, `UPDATE cabals SET name = 'Second pot', picture_url = 'https://cdn.test/p.png' WHERE id = $1`,
		second.ID.UUID())
	a := f.invite(t, first.ID, invitee, first.Members[2].ID)
	aExpires := f.clock.Now().Add(7 * 24 * time.Hour)
	f.clock.Advance(time.Second)
	b := f.invite(t, second.ID, invitee, second.Creator.ID)
	f.invite(t, banned.ID, invitee, banned.Creator.ID)
	f.exec(t, `UPDATE cabals SET status = 'banned' WHERE id = $1`, banned.ID.UUID())
	got, err := app.ListMyInvites(t.Context(), f.pool, f.users, invitee, f.clock.Now())
	want := []inboxLine{
		{a, first.ID.UUID(), first.Members[2].ID.UUID(), "First pot", "", 3, aExpires},
		{
			b, second.ID.UUID(), second.Creator.ID.UUID(), "Second pot", "https://cdn.test/p.png", 1,
			f.clock.Now().Add(7 * 24 * time.Hour),
		},
	}
	lines := inboxLines(got)
	if err != nil || len(lines) != len(want) {
		t.Fatalf("ListMyInvites = %+v, %v; want %+v", lines, err, want)
	}
	for i := range want {
		if lines[i].expires.Equal(want[i].expires) {
			lines[i].expires = want[i].expires
		}
		if lines[i] != want[i] {
			t.Errorf("invite %d = %+v, want %+v", i, lines[i], want[i])
		}
	}
	f.clock.Advance(8 * 24 * time.Hour)
	if got, err := app.ListMyInvites(
		t.Context(),
		f.pool,
		f.users,
		invitee,
		f.clock.Now(),
	); err != nil ||
		len(got) != 0 {
		t.Fatalf("ListMyInvites after expiry = %+v, %v; want none", got, err)
	}
}

func TestListMyInvites_wrapsStoreFailures(t *testing.T) {
	t.Parallel()
	f := newAccess(t)
	c := testkit.NewCabal(t, f.pool)
	invitee := f.user(t)
	f.invite(t, c.ID, invitee, c.Creator.ID)
	_, err := app.ListMyInvites(t.Context(), f.pool, failCards{}, invitee, f.clock.Now())
	wantErr(t, err, errs.CodeInternal)
	f.exec(t, `ALTER TABLE cabal_access_requests RENAME TO requests_gone`)
	_, err = app.ListMyInvites(t.Context(), f.pool, f.users, invitee, f.clock.Now())
	wantErr(t, err, errs.CodeInternal)
}

func TestListCabalInvites_showsMembersTheLiveInvitesWithInviteeAndInviter(t *testing.T) {
	t.Parallel()
	f := newAccess(t)
	c := testkit.NewCabal(t, f.pool, testkit.WithMembers(2))
	invitee, handle := f.handled(t)
	sent := f.invite(t, c.ID, invitee, c.Members[1].ID)
	asker := f.user(t)
	f.exec(t, `INSERT INTO cabal_access_requests (id, cabal_id, user_id, direction, created_at)
		VALUES ($1, $2, $3, 'request', $4)`, ids.Real{}.NewV7(), c.ID.UUID(), asker.UUID(), f.clock.Now())
	got, err := app.ListCabalInvites(t.Context(), f.pool, f.users, c.ID, c.Creator.ID, f.clock.Now())
	if err != nil || len(got) != 1 || got[0].ID != sent || got[0].User.UserID != invitee.UUID() ||
		got[0].User.Handle != handle || got[0].InvitedBy.UserID != c.Members[1].ID.UUID() ||
		!got[0].ExpiresAt.Equal(f.clock.Now().Add(7*24*time.Hour)) {
		t.Fatalf("ListCabalInvites = %+v, %v; want the one invite from member 1", got, err)
	}
}

func TestListCabalInvites_refusesNonMembersAndWrapsStoreFailures(t *testing.T) {
	t.Parallel()
	f := newAccess(t)
	c := testkit.NewCabal(t, f.pool)
	f.invite(t, c.ID, f.user(t), c.Creator.ID)
	list := func(users app.UserCards, cabal ids.CabalID, actor ids.UserID) error {
		_, err := app.ListCabalInvites(t.Context(), f.pool, users, cabal, actor, f.clock.Now())
		return err
	}
	wantErr(t, list(f.users, ids.CabalIDFrom(ids.Real{}.NewV7()), c.Creator.ID), errs.CodeCabalNotFound)
	wantErr(t, list(f.users, c.ID, f.user(t)), errs.CodeNotCabalMember)
	wantErr(t, list(failCards{}, c.ID, c.Creator.ID), errs.CodeInternal)
	f.exec(t, `ALTER TABLE cabal_access_requests RENAME TO requests_gone`)
	wantErr(t, list(f.users, c.ID, c.Creator.ID), errs.CodeInternal)
	f.exec(t, `ALTER TABLE cabal_members RENAME COLUMN role TO role_gone`)
	wantErr(t, list(f.users, c.ID, c.Creator.ID), errs.CodeInternal)
	f.exec(t, `ALTER TABLE cabals RENAME TO cabals_gone`)
	wantErr(t, list(f.users, c.ID, c.Creator.ID), errs.CodeInternal)
}

func TestInviteListRoutes_needACallerAndWireBothLists(t *testing.T) {
	t.Parallel()
	f := newAccess(t)
	c := testkit.NewCabal(t, f.pool)
	invitee, handle := f.handled(t)
	sent := f.invite(t, c.ID, invitee, c.Creator.ID)
	routes := f.routes(nil)
	creator := as(t.Context(), c.Creator.ID)
	_, err := routes.GetCabalInvites(t.Context(), api.GetCabalInvitesRequestObject{Id: c.ID.UUID()})
	wantErr(t, err, errs.CodeUnauthorized)
	_, err = routes.GetCabalInvites(creator, api.GetCabalInvitesRequestObject{Id: ids.Real{}.NewV7()})
	wantErr(t, err, errs.CodeCabalNotFound)
	listed, err := routes.GetCabalInvites(creator, api.GetCabalInvitesRequestObject{Id: c.ID.UUID()})
	items, ok := listed.(api.GetCabalInvites200JSONResponse)
	if err != nil || !ok || len(items) != 1 || items[0].RequestId != sent ||
		items[0].User.UserId != invitee.UUID() || *items[0].User.Handle != handle ||
		items[0].InvitedBy.UserId != c.Creator.ID.UUID() {
		t.Fatalf("GetCabalInvites = %+v, %v", listed, err)
	}
	_, err = routes.GetMyCabalInvites(t.Context(), api.GetMyCabalInvitesRequestObject{})
	wantErr(t, err, errs.CodeUnauthorized)
	f.exec(t, `ALTER TABLE cabal_access_requests RENAME TO requests_gone`)
	_, err = routes.GetMyCabalInvites(as(t.Context(), invitee), api.GetMyCabalInvitesRequestObject{})
	wantErr(t, err, errs.CodeInternal)
}

func TestGetMyCabalInvites_wiresTheInboxForTheInvitee(t *testing.T) {
	t.Parallel()
	f := newAccess(t)
	c := testkit.NewCabal(t, f.pool)
	invitee := f.user(t)
	sent := f.invite(t, c.ID, invitee, c.Creator.ID)
	res, err := f.routes(nil).GetMyCabalInvites(as(t.Context(), invitee), api.GetMyCabalInvitesRequestObject{})
	inbox, ok := res.(api.GetMyCabalInvites200JSONResponse)
	if err != nil || !ok || len(inbox) != 1 {
		t.Fatalf("GetMyCabalInvites = %+v, %v; want one invite", res, err)
	}
	got := inbox[0]
	if got.RequestId != sent || got.Cabal.Id != c.ID.UUID() || got.Cabal.MemberCount != 1 ||
		got.Cabal.PictureUrl != nil || got.InvitedBy.UserId != c.Creator.ID.UUID() ||
		!got.ExpiresAt.Equal(f.clock.Now().Add(7*24*time.Hour)) {
		t.Fatalf("invite = %+v, want the creator's invite into %s", got, c.ID)
	}
}
