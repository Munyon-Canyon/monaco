package domain_test

import (
	"slices"
	"strconv"
	"testing"
	"time"

	"github.com/google/uuid"
	"pgregory.net/rapid"

	"github.com/monaco/monaco/apps/backend/internal/modules/ranking/domain"
)

func boardEpoch() time.Time { return time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC) }

func callID(n int) uuid.UUID { return uuid.NewSHA1(uuid.Nil, strconv.AppendInt(nil, int64(n), 10)) }

func callSubject(n int, right, resolved, voted, eligible int) domain.CallSubject {
	return domain.CallSubject{
		ID: callID(n), Right: right, Resolved: resolved, Voted: voted, Eligible: eligible,
		CreatedAt: boardEpoch().Add(time.Duration(n) * time.Hour),
	}
}

func rankByID(entries []domain.CallEntry, id uuid.UUID) *int {
	for _, e := range entries {
		if e.ID == id {
			return e.Rank
		}
	}
	return nil
}

func TestRankCalls_weekFixtureBreaksTheTieOnTurnout(t *testing.T) {
	t.Parallel()
	const kai, jordan, andres, rosa, first, second, third = 1, 2, 3, 4, 5, 6, 7
	entries := domain.RankCalls([]domain.CallSubject{
		callSubject(jordan, 4, 5, 5, 7),
		callSubject(kai, 4, 5, 6, 7),
		callSubject(andres, 3, 3, 3, 3),
		callSubject(rosa, 2, 2, 2, 2),
		callSubject(first, 5, 5, 7, 7),
		callSubject(second, 5, 5, 6, 7),
		callSubject(third, 5, 5, 5, 7),
	})
	want := map[int]int{first: 1, second: 2, third: 3, kai: 4, jordan: 5}
	for n, rank := range want {
		if got := rankByID(entries, callID(n)); got == nil || *got != rank {
			t.Errorf("rank of %d = %v, want %d", n, got, rank)
		}
	}
	for _, n := range []int{andres, rosa} {
		if got := rankByID(entries, callID(n)); got != nil {
			t.Errorf("rank of %d = %d, want unranked", n, *got)
		}
	}
}

func TestRankCalls_ratesFloorAndNeedFiveResolved(t *testing.T) {
	t.Parallel()
	entries := domain.RankCalls([]domain.CallSubject{callSubject(1, 12, 13, 13, 13), callSubject(2, 4, 4, 4, 4)})
	if entries[0].RateBps == nil || *entries[0].RateBps != 9230 {
		t.Fatalf("rate of 12 of 13 = %v, want 9230", entries[0].RateBps)
	}
	if entries[1].RateBps != nil || entries[1].Rank != nil {
		t.Fatalf("4 resolved = rate %v rank %v, want both nil", entries[1].RateBps, entries[1].Rank)
	}
}

func TestRankCalls_turnoutBpsFloors(t *testing.T) {
	t.Parallel()
	entries := domain.RankCalls([]domain.CallSubject{callSubject(1, 5, 5, 2, 3), callSubject(2, 5, 5, 0, 0)})
	got := map[uuid.UUID]int{}
	for _, e := range entries {
		got[e.ID] = e.TurnoutBps
	}
	if got[callID(1)] != 6666 || got[callID(2)] != 0 {
		t.Fatalf("turnout bps = %v, want 6666 and 0", got)
	}
}

func drawCallSubjects(t *rapid.T) []domain.CallSubject {
	n := rapid.IntRange(0, 12).Draw(t, "n")
	out := make([]domain.CallSubject, n)
	for i := range out {
		resolved := rapid.IntRange(0, 9).Draw(t, "resolved")
		eligible := rapid.IntRange(resolved, 12).Draw(t, "eligible")
		out[i] = callSubject(i+1, rapid.IntRange(0, resolved).Draw(t, "right"), resolved,
			rapid.IntRange(0, eligible).Draw(t, "voted"), eligible)
		out[i].CreatedAt = boardEpoch().Add(time.Duration(rapid.IntRange(0, 3).Draw(t, "age")) * time.Hour)
	}
	return out
}

func TestProperty_RankCallsRanksAreDenseFromOne(t *testing.T) {
	t.Parallel()
	rapid.Check(t, func(t *rapid.T) {
		entries := domain.RankCalls(drawCallSubjects(t))
		want := 1
		for _, e := range entries {
			if ranked := e.Resolved >= domain.MinResolvedCalls; ranked != (e.Rank != nil) {
				t.Fatalf("resolved %d has rank %v", e.Resolved, e.Rank)
			}
			if e.Rank == nil {
				continue
			}
			if *e.Rank != want {
				t.Fatalf("rank %d, want %d in %v", *e.Rank, want, entries)
			}
			want++
		}
	})
}

func TestProperty_RankCallsOrdersIdenticallyWhateverTheInputOrder(t *testing.T) {
	t.Parallel()
	rapid.Check(t, func(t *rapid.T) {
		in := drawCallSubjects(t)
		shuffled := rapid.Permutation(in).Draw(t, "shuffled")
		a, b := domain.RankCalls(in), domain.RankCalls(shuffled)
		if !slices.EqualFunc(a, b, func(x, y domain.CallEntry) bool { return x.ID == y.ID }) {
			t.Fatalf("order differs: %v vs %v", a, b)
		}
	})
}

func TestProperty_AnExtraRightCallNeverLowersTheRank(t *testing.T) {
	t.Parallel()
	rapid.Check(t, func(t *rapid.T) {
		in := drawCallSubjects(t)
		if len(in) == 0 {
			return
		}
		i := rapid.IntRange(0, len(in)-1).Draw(t, "who")
		before := rankByID(domain.RankCalls(in), in[i].ID)
		in[i].Right++
		in[i].Resolved++
		in[i].Voted++
		in[i].Eligible++
		after := rankByID(domain.RankCalls(in), in[i].ID)
		if before != nil && (after == nil || *after > *before) {
			t.Fatalf("rank went from %d to %v after a right call", *before, after)
		}
	})
}

func TestMovement_isTheBaselineRankMinusTheRankNow(t *testing.T) {
	t.Parallel()
	const up, down, same, fresh, unranked, order = 1, 2, 3, 4, 5, 6
	entries := domain.RankCalls([]domain.CallSubject{
		callSubject(up, 9, 10, 10, 10), callSubject(order, 8, 10, 10, 10), callSubject(down, 7, 10, 10, 10),
		callSubject(same, 6, 10, 10, 10), callSubject(fresh, 5, 10, 10, 10), callSubject(unranked, 2, 3, 3, 3),
	})
	baseline := map[uuid.UUID]int{
		callID(up):       3,
		callID(order):    2,
		callID(down):     1,
		callID(same):     4,
		callID(unranked): 6,
	}
	got := map[uuid.UUID]domain.CallEntry{}
	for _, e := range domain.Movement(baseline, entries) {
		got[e.ID] = e
	}
	for name, tc := range map[string]struct {
		n        int
		kind     domain.MovementKind
		movement *int
		baseline *int
	}{
		"up two":                    {up, domain.MovementUp, new(2), new(3)},
		"unchanged":                 {order, domain.MovementSame, new(0), new(2)},
		"down two":                  {down, domain.MovementDown, new(-2), new(1)},
		"same rank":                 {same, domain.MovementSame, new(0), new(4)},
		"first ranked today":        {fresh, domain.MovementNew, nil, nil},
		"still below five resolved": {unranked, domain.MovementNew, nil, new(6)},
	} {
		e := got[callID(tc.n)]
		if e.MovementKind != tc.kind || !equalInts(e.Movement, tc.movement) || !equalInts(e.BaselineRank, tc.baseline) {
			t.Errorf("%s = kind %q movement %v baseline %v, want %q %v %v", name, e.MovementKind,
				deref(e.Movement), deref(e.BaselineRank), tc.kind, deref(tc.movement), deref(tc.baseline))
		}
	}
}

func TestMovement_everyRowIsNewOnTheFirstDayOfAPeriod(t *testing.T) {
	t.Parallel()
	entries := domain.RankCalls([]domain.CallSubject{callSubject(1, 5, 5, 5, 5), callSubject(2, 1, 2, 2, 2)})
	got := domain.Movement(nil, entries)
	for _, e := range got {
		if e.MovementKind != domain.MovementNew || e.Movement != nil || e.BaselineRank != nil {
			t.Errorf("entry %v = %+v, want new with no movement", e.ID, e)
		}
	}
	if entries[0].MovementKind != "" {
		t.Errorf("Movement changed its input: %+v", entries[0])
	}
}

func equalInts(a, b *int) bool { return (a == nil) == (b == nil) && (a == nil || *a == *b) }

func deref(n *int) any {
	if n == nil {
		return nil
	}
	return *n
}
