package app

import (
	"slices"
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/modules/cabal"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

func TestPreviewMembers_OrdersAndLimits(t *testing.T) {
	t.Parallel()
	at := time.Date(2026, 10, 4, 12, 0, 0, 0, time.UTC)
	newID := func() ids.UserID { return ids.UserIDFrom(ids.Real{}.NewV7()) }
	viewer, admin, voterOld, voterNew, nonVoter := newID(), newID(), newID(), newID(), newID()
	tieA, tieB := newID(), newID()
	tied := []ids.UserID{tieA, tieB}
	slices.SortFunc(tied, func(a, b ids.UserID) int {
		if a.String() < b.String() {
			return -1
		}
		return 1
	})
	member := func(u ids.UserID, role cabal.Role, vote bool, joined time.Time) cabal.MemberView {
		return cabal.MemberView{UserID: u, Role: role, CanVote: vote, JoinedAt: joined}
	}
	rows := []cabal.MemberView{
		member(nonVoter, cabal.RoleMember, false, at),
		member(viewer, cabal.RoleCreator, true, at),
		member(tieB, cabal.RoleMember, true, at.Add(time.Hour)),
		member(voterNew, cabal.RoleMember, true, at.Add(2*time.Hour)),
		member(tieA, cabal.RoleMember, true, at.Add(time.Hour)),
		member(voterOld, cabal.RoleMember, true, at),
		member(admin, cabal.RoleCreator, true, at.Add(3*time.Hour)),
	}
	if got, want := previewMembers(rows, viewer, 3), []ids.UserID{admin, voterOld, tied[0]}; !slices.Equal(got, want) {
		t.Fatalf("limit 3 = %v, want %v", got, want)
	}
	all := []ids.UserID{admin, voterOld, tied[0], tied[1], voterNew, nonVoter}
	if got := previewMembers(rows, viewer, 10); !slices.Equal(got, all) {
		t.Fatalf("limit 10 = %v, want %v", got, all)
	}
}
