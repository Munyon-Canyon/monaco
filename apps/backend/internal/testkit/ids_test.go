package testkit_test

import (
	"slices"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

func sequence(seed uint64, n int) []string {
	g := testkit.NewIDs(seed)
	out := make([]string, n)
	for i := range out {
		out[i] = ids.New[struct{}](g).String()
	}
	return out
}

func TestIDsSameSeedSameSequence(t *testing.T) {
	t.Parallel()
	a, b := sequence(7, 50), sequence(7, 50)
	for i := range a {
		if a[i] != b[i] {
			t.Fatalf("seed 7 id %d = %s then %s", i, a[i], b[i])
		}
	}
	if c := sequence(8, 1); c[0] == a[0] {
		t.Fatalf("seeds 7 and 8 both start with %s", c[0])
	}
}

func TestIDsAreSortedDistinctV7(t *testing.T) {
	t.Parallel()
	got := sequence(1, 200)
	for i, raw := range got {
		if _, err := ids.ParseUserID(raw); err != nil {
			t.Fatalf("id %d %s is not a canonical v7: %v", i, raw, err)
		}
		if i > 0 && got[i-1] >= raw {
			t.Fatalf("id %d %s does not sort after %s", i, raw, got[i-1])
		}
	}
}

func TestIDsReseedRepeatsTheSequence(t *testing.T) {
	t.Parallel()
	g := testkit.NewIDs(3)
	first := []string{g.NewV7().String(), g.NewV7().String()}
	g.Reseed(3)
	again := []string{g.NewV7().String(), g.NewV7().String()}
	if !slices.Equal(first, again) {
		t.Fatalf("after Reseed(3) ids = %v, want %v", again, first)
	}
	g.Reseed(4)
	if other := g.NewV7().String(); other == first[0] {
		t.Fatalf("Reseed(4) repeated seed 3's first id %s", other)
	}
}
