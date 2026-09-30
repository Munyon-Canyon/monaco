package domain_test

import (
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/modules/ranking/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

func TestEligible(t *testing.T) {
	t.Parallel()
	const tenDollars = 10_000_000
	tests := map[string]struct {
		net  money.SignedMicros
		min  money.Micros
		want bool
	}{
		"above the minimum":           {net: signed(tenDollars + 1), min: usd(tenDollars), want: true},
		"at the minimum":              {net: signed(tenDollars), min: usd(tenDollars), want: true},
		"under the minimum":           {net: signed(tenDollars - 1), min: usd(tenDollars)},
		"cashed out more than put in": {net: signed(-1), min: usd(0)},
		"nothing in with no minimum":  {net: signed(0), min: usd(0)},
		"anything in with no minimum": {net: signed(1), min: usd(0), want: true},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if got := domain.Eligible(tt.net, tt.min); got != tt.want {
				t.Fatalf("Eligible(%v, %v) = %v, want %v", tt.net, tt.min, got, tt.want)
			}
		})
	}
}
