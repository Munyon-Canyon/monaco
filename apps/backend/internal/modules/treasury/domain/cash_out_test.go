package domain

import (
	"slices"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/errs"
)

func TestNextCashOut(t *testing.T) {
	t.Parallel()
	cases := []struct {
		from  CashOutStatus
		event CashOutEvent
		want  CashOutStatus
		code  errs.Code
	}{
		{CashOutStarted, CashOutSell, CashOutSelling, ""},
		{CashOutStarted, CashOutStartPaying, CashOutPaying, ""},
		{CashOutSelling, CashOutStartPaying, CashOutPaying, ""},
		{CashOutSelling, CashOutFail, CashOutFailed, ""},
		{CashOutSelling, CashOutSell, CashOutSelling, errs.CodeVersionConflict},
		{CashOutPaying, CashOutCompletePartial, CashOutPartial, ""},
		{CashOutStarted, CashOutFail, CashOutFailed, ""},
		{CashOutPaying, CashOutComplete, CashOutCompleted, ""},
		{CashOutPaying, CashOutFail, CashOutFailed, ""},
		{CashOutCompleted, CashOutStartPaying, CashOutCompleted, errs.CodeVersionConflict},
		{CashOutPartial, CashOutComplete, CashOutPartial, errs.CodeVersionConflict},
		{CashOutFailed, CashOutStartPaying, CashOutFailed, errs.CodeVersionConflict},
		{"unknown", CashOutStartPaying, "unknown", errs.CodeDecodeFailed},
		{CashOutStarted, "unknown", CashOutStarted, errs.CodeDecodeFailed},
	}
	for _, c := range cases {
		got, err := NextCashOut(c.from, c.event)
		if got != c.want || (c.code == "" && err != nil) || (c.code != "" && errs.CodeOf(err) != c.code) {
			t.Fatalf("NextCashOut(%q, %q) = (%q, %s), want (%q, %s)",
				c.from, c.event, got, errs.CodeOf(err), c.want, c.code)
		}
	}
}

func TestParseCashOutStatus(t *testing.T) {
	t.Parallel()
	for _, status := range CashOutStatuses() {
		got, err := ParseCashOutStatus(string(status))
		if err != nil || got != status {
			t.Fatalf("ParseCashOutStatus(%q) = (%q, %v)", status, got, err)
		}
	}
	if _, err := ParseCashOutStatus("unknown"); errs.CodeOf(err) != errs.CodeDecodeFailed {
		t.Fatalf("ParseCashOutStatus unknown code = %s", errs.CodeOf(err))
	}
}

func TestCashOutEvents(t *testing.T) {
	t.Parallel()
	got := CashOutEvents()
	want := []CashOutEvent{CashOutSell, CashOutStartPaying, CashOutComplete, CashOutCompletePartial, CashOutFail}
	if !slices.Equal(got, want) {
		t.Fatalf("CashOutEvents() = %v, want %v", got, want)
	}
}
