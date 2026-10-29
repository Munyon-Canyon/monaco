package domain_test

import (
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/modules/ranking/domain"
)

func gapEntry(name string, right, resolved int) domain.CallEntry {
	rate := right * 10_000 / resolved
	return domain.CallEntry{
		CallSubject: domain.CallSubject{
			ID: uuid.NewSHA1(
				uuid.Nil,
				[]byte(name),
			),
			Right:     right,
			Resolved:  resolved,
			Voted:     resolved,
			Eligible:  resolved,
			CreatedAt: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		},
		RateBps: &rate,
	}
}

func rateEntry(name string, rateBps, right, resolved int) domain.CallEntry {
	e := gapEntry(name, right, resolved)
	e.RateBps = &rateBps
	return e
}

func TestRatePct_roundsHalfUp(t *testing.T) {
	t.Parallel()
	for in, want := range map[int]int{0: 0, 49: 0, 50: 1, 7777: 78, 7857: 79, 8900: 89, 9999: 100, 10_000: 100} {
		if got := domain.RatePct(in); got != want {
			t.Errorf("RatePct(%d) = %d, want %d", in, got, want)
		}
	}
}

func TestCallsToPass_countsTheFewestRightCallsWithNoMisses(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct {
		me, above domain.CallEntry
		want      *int
	}{
		"one call passes Kai":                    {gapEntry("andres", 7, 9), gapEntry("kai", 11, 14), gapPtr(1)},
		"never past a perfect row":               {gapEntry("andres", 7, 9), gapEntry("christelle", 12, 12), nil},
		"a tie on rate goes to more right calls": {gapEntry("andres", 5, 10), gapEntry("kai", 8, 10), gapPtr(15)},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			got := domain.CallsToPass(tc.me, tc.above)
			if gapDeref(got) != gapDeref(tc.want) {
				t.Fatalf("CallsToPass = %v, want %v", gapDeref(got), gapDeref(tc.want))
			}
		})
	}
}

func TestComputeGap(t *testing.T) {
	t.Parallel()
	andres, kai := gapEntry("andres", 7, 9), gapEntry("kai", 11, 14)
	titans, pilates := rateEntry("titans", 8900, 8, 9), rateEntry("pilates", 9300, 13, 14)
	christelle, logan := rateEntry("christelle", 9200, 11, 12), rateEntry("logan", 9100, 10, 11)
	tieA, tieB := gapEntry("a", 4, 5), gapEntry("b", 4, 5)
	perfect := gapEntry("leader", 9, 9)
	for name, tc := range map[string]struct {
		me          domain.CallEntry
		above       *domain.CallEntry
		below       *domain.CallEntry
		wantVersus  domain.GapVersus
		wantPoints  int
		wantPass    *int
		wantPerfect bool
	}{
		"behind Kai":            {andres, &kai, nil, domain.VersusAbove, -1, gapPtr(1), false},
		"Tech Titans behind":    {titans, &pilates, nil, domain.VersusAbove, -4, gapPtr(6), false},
		"leads Logan":           {christelle, nil, &logan, domain.VersusBelow, 1, nil, false},
		"a tie scores zero":     {tieA, &tieB, nil, domain.VersusAbove, 0, gapPtr(1), false},
		"a perfect row above":   {andres, &perfect, nil, domain.VersusAbove, -22, nil, true},
		"above wins over below": {andres, &kai, &logan, domain.VersusAbove, -1, gapPtr(1), false},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			got := domain.ComputeGap(tc.me, tc.above, tc.below)
			if got.Versus != tc.wantVersus || got.Points != tc.wantPoints || got.AboveIsPerfect != tc.wantPerfect ||
				gapDeref(got.CallsToPass) != gapDeref(tc.wantPass) {
				t.Fatalf("ComputeGap = %+v (calls %v), want %s %d calls %v perfect %t", got, gapDeref(got.CallsToPass),
					tc.wantVersus, tc.wantPoints, gapDeref(tc.wantPass), tc.wantPerfect)
			}
		})
	}
	if got := domain.ComputeGap(andres, nil, nil); got.Versus != domain.VersusNone {
		t.Fatalf("ComputeGap with nobody to compare = %+v, want no versus", got)
	}
}

func gapPtr(n int) *int { return &n }

func gapDeref(p *int) any {
	if p == nil {
		return nil
	}
	return *p
}
