package domain_test

import (
	"errors"
	"math"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/ranking/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

func TestMemberEquity(t *testing.T) {
	t.Parallel()
	tests := map[string]struct {
		shares, total money.SharesUnits
		nav, want     money.Micros
	}{
		"no shares":          {total: shares(0), nav: usd(100)},
		"no shares of a pot": {total: shares(10), nav: usd(100)},
		"sole member":        {shares: shares(10), total: shares(10), nav: usd(100), want: usd(100)},
		"rounds down":        {shares: shares(1), total: shares(3), nav: usd(100), want: usd(33)},
		"no big-number drift": {
			shares: shares(math.MaxUint64 - 1),
			total:  shares(math.MaxUint64),
			nav:    usd(math.MaxUint64),
			want:   usd(math.MaxUint64 - 1),
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if got, err := domain.MemberEquity(tt.shares, tt.total, tt.nav); err != nil || got != tt.want {
				t.Fatalf("MemberEquity = %v, %v, want %v", got, err, tt.want)
			}
		})
	}
}

func TestMemberEquity_MoreSharesThanTotal(t *testing.T) {
	t.Parallel()
	for _, total := range []money.SharesUnits{shares(0), shares(4)} {
		if got, err := domain.MemberEquity(
			shares(5),
			total,
			usd(100),
		); errs.CodeOf(err) != errs.CodeInvalidInput || !errors.Is(err, domain.ErrUnusableStart) ||
			!got.IsZero() {
			t.Fatalf("MemberEquity(5 of %v) = %v, %v, want invalid_input", total, got, err)
		}
	}
}
