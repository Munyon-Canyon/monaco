package domain

import (
	"bytes"
	"slices"
	"time"

	"github.com/google/uuid"
)

const (
	RankAdmin = iota
	RankVoter
	RankMember
)

type OrderedMember struct {
	UserID  uuid.UUID
	Role    Role
	CanVote bool
	Joined  time.Time
	Bps     int32
}

type MemberKey struct {
	Rank   int
	Bps    int32
	Joined time.Time
	UserID uuid.UUID
}

func (m OrderedMember) Rank() int {
	switch {
	case m.Role.IsAdmin():
		return RankAdmin
	case m.CanVote:
		return RankVoter
	}
	return RankMember
}

func (m OrderedMember) RoleKey() MemberKey {
	return MemberKey{Rank: m.Rank(), Bps: m.Bps, Joined: m.Joined, UserID: m.UserID}
}

func (m OrderedMember) JoinedKey() MemberKey {
	return MemberKey{Joined: m.Joined, UserID: m.UserID}
}

func CompareMemberKeys(a, b MemberKey) int {
	if a.Rank != b.Rank {
		return a.Rank - b.Rank
	}
	if a.Bps != b.Bps {
		return int(b.Bps - a.Bps)
	}
	if c := a.Joined.Compare(b.Joined); c != 0 {
		return c
	}
	return bytes.Compare(a.UserID[:], b.UserID[:])
}

func MemberOrder(members []OrderedMember) {
	slices.SortFunc(members, func(a, b OrderedMember) int { return CompareMemberKeys(a.RoleKey(), b.RoleKey()) })
}

type MemberTotals struct {
	Admins  int
	Voters  int
	Members int
}

func TotalMembers(members []OrderedMember) MemberTotals {
	var t MemberTotals
	for _, m := range members {
		switch m.Rank() {
		case RankAdmin:
			t.Admins++
		case RankVoter:
			t.Voters++
		default:
			t.Members++
		}
	}
	return t
}

const MinCrownCalls = 5

const bpsPerWhole = 10000

type MemberCalls struct {
	Right    int
	Resolved int
	Voted    int
	Eligible int
}

func (c MemberCalls) rateBps() int {
	return int(int64(c.Right) * bpsPerWhole / int64(c.Resolved))
}

func betterCalls(a, b MemberCalls) int {
	if c := b.rateBps() - a.rateBps(); c != 0 {
		return c
	}
	if c := b.Right - a.Right; c != 0 {
		return c
	}
	left, right := int64(b.Voted)*int64(max(a.Eligible, 1)), int64(a.Voted)*int64(max(b.Eligible, 1))
	switch {
	case left < right:
		return -1
	case left > right:
		return 1
	}
	return 0
}

func CrownHolder(members []OrderedMember, calls map[uuid.UUID]MemberCalls) uuid.UUID {
	var best *OrderedMember
	for i := range members {
		m := &members[i]
		if calls[m.UserID].Resolved < MinCrownCalls {
			continue
		}
		if best == nil || compareCrown(*m, *best, calls) < 0 {
			best = m
		}
	}
	if best == nil {
		return uuid.Nil
	}
	return best.UserID
}

func compareCrown(a, b OrderedMember, calls map[uuid.UUID]MemberCalls) int {
	if c := betterCalls(calls[a.UserID], calls[b.UserID]); c != 0 {
		return c
	}
	if c := a.Joined.Compare(b.Joined); c != 0 {
		return c
	}
	return bytes.Compare(a.UserID[:], b.UserID[:])
}
