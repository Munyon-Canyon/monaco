package domain_test

import (
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/modules/cabal/domain"
)

func orderedMember(id byte, role domain.Role, vote bool, day int, bps int32) domain.OrderedMember {
	return domain.OrderedMember{
		UserID: uuid.UUID{id}, Role: role, CanVote: vote, Bps: bps,
		Joined: time.Date(2026, 9, day, 12, 0, 0, 0, time.UTC),
	}
}

func TestMemberOrder_ranksRoleThenShareThenJoinThenID(t *testing.T) {
	t.Parallel()
	members := []domain.OrderedMember{
		orderedMember(1, domain.RoleMember, false, 1, 9000),
		orderedMember(2, domain.RoleMember, true, 5, 100),
		orderedMember(3, domain.RoleMember, true, 3, 100),
		orderedMember(4, domain.RoleMember, true, 3, 100),
		orderedMember(5, domain.RoleMember, true, 9, 800),
		orderedMember(6, domain.RoleAdmin, true, 9, 0),
		orderedMember(7, domain.RoleCreator, true, 9, 2000),
	}
	domain.MemberOrder(members)
	want := []byte{7, 6, 5, 3, 4, 2, 1}
	for i, id := range want {
		if members[i].UserID[0] != id {
			t.Fatalf("position %d = %d, want %d (%v)", i, members[i].UserID[0], id, members)
		}
	}
}

func TestTotalMembers_countsTheWholeCabal(t *testing.T) {
	t.Parallel()
	got := domain.TotalMembers([]domain.OrderedMember{
		orderedMember(1, domain.RoleCreator, true, 1, 0), orderedMember(2, domain.RoleAdmin, true, 1, 0),
		orderedMember(3, domain.RoleMember, true, 1, 0), orderedMember(4, domain.RoleMember, false, 1, 0),
	})
	if got != (domain.MemberTotals{Admins: 2, Voters: 1, Members: 1}) {
		t.Fatalf("totals = %+v", got)
	}
}

func TestMemberKeys_joinedKeyIgnoresRoleAndShare(t *testing.T) {
	t.Parallel()
	a, b := orderedMember(1, domain.RoleMember, false, 1, 0), orderedMember(2, domain.RoleCreator, true, 2, 9000)
	if domain.CompareMemberKeys(a.JoinedKey(), b.JoinedKey()) >= 0 {
		t.Fatal("earlier joiner must sort first by joined key")
	}
}

func TestCrownHolder_ranksRateThenRightThenTurnoutThenJoinThenID(t *testing.T) {
	t.Parallel()
	members := []domain.OrderedMember{
		orderedMember(1, domain.RoleMember, true, 1, 0), orderedMember(2, domain.RoleMember, true, 2, 0),
		orderedMember(3, domain.RoleMember, true, 3, 0), orderedMember(4, domain.RoleMember, true, 4, 0),
		orderedMember(5, domain.RoleMember, true, 4, 0),
	}
	id := func(b byte) uuid.UUID { return uuid.UUID{b} }
	for name, tt := range map[string]struct {
		calls map[uuid.UUID]domain.MemberCalls
		want  uuid.UUID
	}{
		"nobody has five": {map[uuid.UUID]domain.MemberCalls{id(1): {Right: 4, Resolved: 4, Voted: 4, Eligible: 4}}, uuid.Nil},
		"rate wins": {map[uuid.UUID]domain.MemberCalls{
			id(1): {Right: 5, Resolved: 6, Voted: 6, Eligible: 6}, id(2): {Right: 5, Resolved: 5, Voted: 5, Eligible: 9},
		}, id(2)},
		"right breaks a tie": {map[uuid.UUID]domain.MemberCalls{
			id(1): {Right: 5, Resolved: 5, Voted: 5, Eligible: 5}, id(2): {Right: 10, Resolved: 10, Voted: 10, Eligible: 10},
		}, id(2)},
		"turnout breaks a tie": {map[uuid.UUID]domain.MemberCalls{
			id(1): {Right: 5, Resolved: 5, Voted: 5, Eligible: 9}, id(2): {Right: 5, Resolved: 5, Voted: 9, Eligible: 9},
			id(3): {Right: 5, Resolved: 5, Voted: 1, Eligible: 9},
		}, id(2)},
		"join date then id break a full tie": {map[uuid.UUID]domain.MemberCalls{
			id(3): {Right: 5, Resolved: 5, Voted: 5, Eligible: 5}, id(2): {Right: 5, Resolved: 5, Voted: 5, Eligible: 5},
			id(5): {Right: 5, Resolved: 5, Voted: 5, Eligible: 5}, id(4): {Right: 5, Resolved: 5, Voted: 5, Eligible: 5},
		}, id(2)},
		"id breaks a join tie": {map[uuid.UUID]domain.MemberCalls{
			id(5): {Right: 5, Resolved: 5, Voted: 5, Eligible: 5}, id(4): {Right: 5, Resolved: 5, Voted: 5, Eligible: 5},
		}, id(4)},
	} {
		if got := domain.CrownHolder(members, tt.calls); got != tt.want {
			t.Fatalf("%s: crown = %v, want %v", name, got, tt.want)
		}
	}
}
