package cabal_test

import (
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/modules/cabal/adapters"
	"github.com/monaco/monaco/apps/backend/internal/modules/cabal/port"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

func TestPendingInvites_listsLiveInvitesNewestFirstWithCabalAndInviter(t *testing.T) {
	t.Parallel()
	f := newAccess(t)
	first := testkit.NewCabal(t, f.pool, testkit.WithMembers(3))
	second := testkit.NewCabal(t, f.pool)
	stale := testkit.NewCabal(t, f.pool)
	banned := testkit.NewCabal(t, f.pool)
	invitee := f.user(t)
	f.exec(t, `UPDATE cabals SET name = 'First pot' WHERE id = $1`, first.ID.UUID())
	f.exec(t, `UPDATE cabals SET name = 'Second pot', picture_url = 'https://cdn.test/p.png' WHERE id = $1`,
		second.ID.UUID())
	f.invite(t, stale.ID, invitee, stale.Creator.ID)
	f.clock.Advance(6 * 24 * time.Hour)
	older := f.invite(t, first.ID, invitee, first.Members[2].ID)
	f.clock.Advance(time.Hour)
	newer := f.invite(t, second.ID, invitee, second.Creator.ID)
	f.invite(t, banned.ID, invitee, banned.Creator.ID)
	f.exec(t, `UPDATE cabals SET status = 'banned' WHERE id = $1`, banned.ID.UUID())
	f.clock.Advance(2 * 24 * time.Hour)

	reads := adapters.NewQueries(f.pool).WithUsers(f.users)
	got, err := reads.PendingInvites(t.Context(), invitee, f.clock.Now())
	if err != nil || len(got) != 2 {
		t.Fatalf("PendingInvites = %+v, %v; want the two live invites", got, err)
	}
	wantInvite(t, got[0], wantedInvite{
		id: newer, cabal: second.ID, name: "Second pot", picture: "https://cdn.test/p.png", members: 1,
		inviter: second.Creator.ID,
	})
	wantInvite(t, got[1], wantedInvite{
		id: older, cabal: first.ID, name: "First pot", members: 3, inviter: first.Members[2].ID,
	})
	if !got[0].CreatedAt.After(got[1].CreatedAt) {
		t.Errorf("created at = %v then %v, want newest first", got[0].CreatedAt, got[1].CreatedAt)
	}
}

type wantedInvite struct {
	id      uuid.UUID
	cabal   ids.CabalID
	inviter ids.UserID
	name    string
	picture string
	members int
}

func wantInvite(t *testing.T, got port.PendingInvite, want wantedInvite) {
	t.Helper()
	have := wantedInvite{
		id: got.ID, cabal: got.CabalID, inviter: got.InviterID, name: got.CabalName, picture: got.PictureURL,
		members: got.MemberCount,
	}
	if have != want {
		t.Errorf("invite = %+v, want %+v", have, want)
	}
}

func TestPendingInvites_withoutUserCardsFails(t *testing.T) {
	t.Parallel()
	f := newAccess(t)
	if _, err := adapters.NewQueries(f.pool).PendingInvites(t.Context(), f.user(t), f.clock.Now()); err == nil {
		t.Fatal("PendingInvites without user cards succeeded")
	}
}

func TestPendingInvites_returnsTheInviteReadFailure(t *testing.T) {
	t.Parallel()
	f := newAccess(t)
	f.exec(t, `ALTER TABLE cabal_access_requests RENAME TO requests_gone`)
	reads := adapters.NewQueries(f.pool).WithUsers(f.users)
	if _, err := reads.PendingInvites(t.Context(), f.user(t), f.clock.Now()); err == nil {
		t.Fatal("PendingInvites with the invite table gone succeeded")
	}
}
