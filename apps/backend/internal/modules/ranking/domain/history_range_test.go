package domain_test

import (
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/ranking/domain"
)

func TestParseHistoryRange(t *testing.T) {
	t.Parallel()
	for _, raw := range []string{"1H", "1D", "1W", "1M", "3M", "6M", "ALL"} {
		if got, err := domain.ParseHistoryRange(raw); err != nil || string(got) != raw {
			t.Errorf("ParseHistoryRange(%q) = %q, %v", raw, got, err)
		}
	}
	for _, raw := range []string{"", "3m", "1Y", "12M", "all"} {
		if _, err := domain.ParseHistoryRange(raw); errs.CodeOf(err) != errs.CodeInvalidInput {
			t.Errorf("ParseHistoryRange(%q) err = %v, want invalid_input", raw, err)
		}
	}
}

func TestParseRangeRefusesTheHistoryOnlyRanges(t *testing.T) {
	t.Parallel()
	for _, raw := range []string{"3M", "6M"} {
		if _, err := domain.ParseRange(raw); errs.CodeOf(err) != errs.CodeInvalidInput {
			t.Errorf("ParseRange(%q) err = %v, want invalid_input", raw, err)
		}
	}
}

func TestHistoryRangesStayWithinTheMonthlyPointBudget(t *testing.T) {
	t.Parallel()
	now := bucketNowAt()
	month := len(domain.BucketTimes(domain.Range1M, now, now.AddDate(-1, 0, 0)))
	for _, r := range []domain.Range{domain.Range3M, domain.Range6M} {
		if got := len(domain.BucketTimes(r, now, now.AddDate(-1, 0, 0))); got > month {
			t.Errorf("%s has %d buckets, want at most the 1M %d", r, got, month)
		}
	}
}
