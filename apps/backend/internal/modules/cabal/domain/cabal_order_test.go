package domain_test

import (
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/cabal/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

func memberships(n int) ([]domain.Membership, []ids.CabalID) {
	cabals := make([]ids.CabalID, n)
	current := make([]domain.Membership, n)
	for i := range cabals {
		cabals[i] = ids.CabalIDFrom(ids.Real{}.NewV7())
		current[i] = domain.Membership{CabalID: cabals[i], SortIndex: int32(i)}
	}
	return current, cabals
}

func assertOrder(t *testing.T, got []domain.Membership, want ...ids.CabalID) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("Reorder = %+v, want %d memberships", got, len(want))
	}
	for i, m := range got {
		if m.CabalID != want[i] || m.SortIndex != int32(i) {
			t.Fatalf("Reorder[%d] = %+v, want cabal %v at index %d", i, m, want[i], i)
		}
	}
}

func TestReorder_ListedCabalsComeFirstAndTheRestKeepTheirOrder(t *testing.T) {
	t.Parallel()
	current, c := memberships(3)
	assertOrder(t, domain.Reorder(current, []ids.CabalID{c[2], c[1]}), c[2], c[1], c[0])
}

func TestReorder_AFullRequestSetsTheWholeOrder(t *testing.T) {
	t.Parallel()
	current, c := memberships(3)
	assertOrder(t, domain.Reorder(current, []ids.CabalID{c[1], c[2], c[0]}), c[1], c[2], c[0])
}

func TestReorder_DropsIDsThatAreNotMemberships(t *testing.T) {
	t.Parallel()
	current, c := memberships(2)
	stale := ids.CabalIDFrom(ids.Real{}.NewV7())
	assertOrder(t, domain.Reorder(current, []ids.CabalID{stale, c[1]}), c[1], c[0])
}

func TestReorder_AnEmptyRequestKeepsThePriorOrderCompacted(t *testing.T) {
	t.Parallel()
	current, c := memberships(3)
	current[0].SortIndex, current[1].SortIndex, current[2].SortIndex = 4, 9, 20
	assertOrder(t, domain.Reorder(current, nil), c[0], c[1], c[2])
}

func TestReorder_NeverRepeatsAnIndexOrACabal(t *testing.T) {
	t.Parallel()
	current, c := memberships(4)
	got := domain.Reorder(current, []ids.CabalID{c[3], c[3], c[1], c[3]})
	assertOrder(t, got, c[3], c[1], c[0], c[2])
}

func TestParseCabalOrder(t *testing.T) {
	t.Parallel()
	_, c := memberships(2)
	long := make([]ids.CabalID, domain.MaxOrderedCabals+1)
	for i := range long {
		long[i] = ids.CabalIDFrom(ids.Real{}.NewV7())
	}
	for _, tt := range []struct {
		name string
		in   []ids.CabalID
		ok   bool
	}{
		{"empty", nil, true},
		{"two", c, true},
		{"duplicate", []ids.CabalID{c[0], c[0]}, false},
		{"limit", long[:domain.MaxOrderedCabals], true},
		{"over the limit", long, false},
	} {
		err := domain.ParseCabalOrder(tt.in)
		if (err == nil) != tt.ok || (err != nil && errs.CodeOf(err) != errs.CodeInvalidInput) {
			t.Errorf("%s: ParseCabalOrder = %v, ok %v", tt.name, err, tt.ok)
		}
	}
}
