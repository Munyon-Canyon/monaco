package adapters

import "testing"

func TestPositiveAmount_hidesZeroAndKeepsTheRest(t *testing.T) {
	t.Parallel()
	if got := positiveAmount(0); got != nil {
		t.Errorf("positiveAmount(0) = %d, want none", *got)
	}
	if got := positiveAmount(25); got == nil || *got != 25 {
		t.Errorf("positiveAmount(25) = %v, want 25", got)
	}
}

func TestFirstNonEmpty_prefersTheFirstAndFallsBackToTheSecond(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ a, b, want string }{
		{"Mia", "mia", "Mia"}, {"", "mia", "mia"}, {"", "", ""},
	} {
		if got := firstNonEmpty(tc.a, tc.b); got != tc.want {
			t.Errorf("firstNonEmpty(%q, %q) = %q, want %q", tc.a, tc.b, got, tc.want)
		}
	}
}
