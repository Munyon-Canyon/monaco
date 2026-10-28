package domain

import (
	"bytes"
	"slices"
	"time"

	"github.com/google/uuid"
)

const (
	MinResolvedCalls = 5
	CallBoardEvery   = 5 * time.Minute
)

type CallSubject struct {
	ID        uuid.UUID
	Right     int
	Resolved  int
	Voted     int
	Eligible  int
	CreatedAt time.Time
}

type CallEntry struct {
	CallSubject
	Rank       *int
	RateBps    *int
	TurnoutBps int
}

func RankCalls(subjects []CallSubject) []CallEntry {
	entries := make([]CallEntry, len(subjects))
	for i, s := range subjects {
		entries[i] = CallEntry{CallSubject: s, TurnoutBps: ratioBps(s.Voted, s.Eligible)}
		if s.Resolved >= MinResolvedCalls {
			rate := ratioBps(s.Right, s.Resolved)
			entries[i].RateBps = &rate
		}
	}
	slices.SortFunc(entries, compareCallEntries)
	for i := range entries {
		if entries[i].RateBps == nil {
			break
		}
		rank := i + 1
		entries[i].Rank = &rank
	}
	return entries
}

func compareCallEntries(a, b CallEntry) int {
	if c := compareStanding(a, b); c != 0 {
		return c
	}
	if c := a.CreatedAt.Compare(b.CreatedAt); c != 0 {
		return c
	}
	return bytes.Compare(a.ID[:], b.ID[:])
}

func compareStanding(a, b CallEntry) int {
	switch {
	case a.RateBps == nil && b.RateBps == nil:
		return b.Resolved - a.Resolved
	case a.RateBps == nil:
		return 1
	case b.RateBps == nil:
		return -1
	}
	if c := *b.RateBps - *a.RateBps; c != 0 {
		return c
	}
	if c := b.Right - a.Right; c != 0 {
		return c
	}
	return compareTurnout(b, a)
}

func compareTurnout(a, b CallEntry) int {
	left, right := int64(a.Voted)*int64(max(b.Eligible, 1)), int64(b.Voted)*int64(max(a.Eligible, 1))
	switch {
	case left < right:
		return -1
	case left > right:
		return 1
	}
	return 0
}

func ratioBps(part, whole int) int {
	if whole <= 0 {
		return 0
	}
	return int(int64(part) * bpsPerUnit / int64(whole))
}
