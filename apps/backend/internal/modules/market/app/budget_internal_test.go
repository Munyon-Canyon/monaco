package app

import "testing"

func TestBudgetWithoutLimitIsAlwaysOpen(t *testing.T) {
	t.Parallel()

	for _, limit := range []int{0, -1} {
		b := NewBudget(nil, nil, limit)
		if got := b.levelAt(1_000_000); got != BudgetOpen {
			t.Fatalf("limit %d: level = %v, want %v", limit, got, BudgetOpen)
		}
	}
}
