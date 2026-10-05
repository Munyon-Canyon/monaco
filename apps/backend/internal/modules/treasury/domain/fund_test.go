package domain_test

import (
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/domain"
)

func TestFundStatus_movesOnlyForward(t *testing.T) {
	t.Parallel()
	all := []domain.FundStatus{
		domain.FundCreated, domain.FundSubmitted, domain.FundLanded, domain.FundSettled, domain.FundFailed,
	}
	allowed := map[[2]domain.FundStatus]bool{
		{domain.FundCreated, domain.FundSubmitted}: true,
		{domain.FundCreated, domain.FundFailed}:    true,
		{domain.FundSubmitted, domain.FundLanded}:  true,
		{domain.FundSubmitted, domain.FundFailed}:  true,
		{domain.FundLanded, domain.FundSettled}:    true,
	}
	for _, from := range all {
		for _, to := range all {
			if got := from.CanMoveTo(to); got != allowed[[2]domain.FundStatus{from, to}] {
				t.Errorf("%s.CanMoveTo(%s) = %t", from, to, got)
			}
		}
	}
}

func TestParseFundAmount(t *testing.T) {
	t.Parallel()
	got, err := domain.ParseFundAmount("1000000")
	if err != nil || got.String() != "1000000" {
		t.Fatalf("ParseFundAmount(1000000) = %v, %v, want 1000000", got, err)
	}
	for _, raw := range []string{"999999", "0", "-5", "abc", ""} {
		_, err := domain.ParseFundAmount(raw)
		if errs.CodeOf(err) != errs.CodeInvalidInput {
			t.Errorf("ParseFundAmount(%q) code = %q, want invalid_input", raw, errs.CodeOf(err))
		}
	}
}
