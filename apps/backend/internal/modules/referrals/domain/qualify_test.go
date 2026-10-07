package domain_test

import (
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/modules/referrals/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

func TestSkipReason(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name     string
		amount   uint64
		verified bool
		want     string
	}{
		{"below minimum, verified", 9_999_999, true, "below_minimum"},
		{"exactly the minimum, verified", 10_000_000, true, ""},
		{"exactly the minimum, unverified", 10_000_000, false, "phone_unverified"},
		{"below minimum, unverified", 9_999_999, false, "below_minimum"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := domain.SkipReason(money.MicrosFromUint64(tc.amount), tc.verified); got != tc.want {
				t.Fatalf("SkipReason(%d, %v) = %q, want %q", tc.amount, tc.verified, got, tc.want)
			}
		})
	}
}
