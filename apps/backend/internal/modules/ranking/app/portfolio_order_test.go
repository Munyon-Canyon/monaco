package app_test

import (
	"slices"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

func TestReadPortfolio_RowsFollowTheMembershipOrderNotValue(t *testing.T) {
	t.Parallel()
	f := newMultiFixture()
	f.joined.cabals = []ids.CabalID{f.cabals[2], f.cabals[3], f.cabals[0], f.cabals[1]}
	got, err := f.read(t, fakeBoards{run: f.run, hasRun: true})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	order := make([]ids.CabalID, 0, len(got.Rows))
	for _, row := range got.Rows {
		order = append(order, row.CabalID)
	}
	if !slices.Equal(order, f.joined.cabals) {
		t.Fatalf("rows = %v, want the membership order %v", order, f.joined.cabals)
	}
	var sum uint64
	for _, row := range got.Rows {
		sum += row.SliceBps
	}
	if sum != 10_000 {
		t.Fatalf("slices sum to %d, want 10000 whatever the order", sum)
	}
}
