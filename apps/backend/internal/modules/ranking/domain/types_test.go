package domain_test

import (
	"slices"
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/ranking/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

func TestParseRange(t *testing.T) {
	t.Parallel()
	for _, want := range []domain.Range{domain.Range1H, domain.Range1D, domain.Range1W, domain.Range1M, domain.RangeAll} {
		if got, err := domain.ParseRange(string(want)); err != nil || got != want {
			t.Errorf("ParseRange(%q) = %q, %v", want, got, err)
		}
	}
	for _, raw := range []string{"", "1h", "all", "2D", "1Y"} {
		if _, err := domain.ParseRange(raw); errs.CodeOf(err) != errs.CodeInvalidInput {
			t.Errorf("ParseRange(%q) err = %v, want invalid_input", raw, err)
		}
	}
}

func TestRangeStart(t *testing.T) {
	t.Parallel()
	now := clock.Real{}.Now().UTC()
	tests := map[domain.Range]time.Duration{
		domain.Range1H: time.Hour,
		domain.Range1D: 24 * time.Hour,
		domain.Range1W: 7 * 24 * time.Hour,
		domain.Range1M: 30 * 24 * time.Hour,
	}
	for r, span := range tests {
		if got, ok := r.Start(now); !ok || !got.Equal(now.Add(-span)) {
			t.Errorf("%s.Start = %v, %v, want %v", r, got, ok, now.Add(-span))
		}
	}
	if got, ok := domain.RangeAll.Start(now); ok || !got.IsZero() {
		t.Errorf("ALL.Start = %v, %v, want zero, false", got, ok)
	}
}

func TestParseSort(t *testing.T) {
	t.Parallel()
	for _, want := range []domain.Sort{domain.SortReturn, domain.SortPnL, domain.SortName} {
		if got, err := domain.ParseSort(string(want)); err != nil || got != want {
			t.Errorf("ParseSort(%q) = %q, %v", want, got, err)
		}
	}
	if _, err := domain.ParseSort("size"); errs.CodeOf(err) != errs.CodeInvalidInput {
		t.Errorf("ParseSort(size) err = %v", err)
	}
}

func TestParseMembership(t *testing.T) {
	t.Parallel()
	joins := map[domain.Membership][]string{
		domain.MembershipAll:               {},
		domain.MembershipAcceptingRequests: {"open", "request"},
		domain.MembershipOpen:              {"open"},
		domain.MembershipApproval:          {"request"},
	}
	for want, modes := range joins {
		if got, err := domain.ParseMembership(string(want)); err != nil || got != want ||
			!slices.Equal(got.JoinModes(), modes) {
			t.Errorf("ParseMembership(%q) = %q, %v, modes %v", want, got, err, got.JoinModes())
		}
	}
	if _, err := domain.ParseMembership("closed"); errs.CodeOf(err) != errs.CodeInvalidInput {
		t.Errorf("ParseMembership(closed) err = %v", err)
	}
}

func TestParseMinMembers(t *testing.T) {
	t.Parallel()
	low, high, ok := -1, domain.MaxMinMembers+1, 5
	for _, bad := range []*int{&low, &high} {
		if _, err := domain.ParseMinMembers(bad); errs.CodeOf(err) != errs.CodeInvalidInput {
			t.Errorf("ParseMinMembers(%d) err = %v", *bad, err)
		}
	}
	if got, err := domain.ParseMinMembers(&ok); err != nil || got != 5 {
		t.Errorf("ParseMinMembers(5) = %d, %v", got, err)
	}
	if got, err := domain.ParseMinMembers(nil); err != nil || got != 0 {
		t.Errorf("ParseMinMembers(nil) = %d, %v", got, err)
	}
}

func TestViewIsDefaultOnlyWithoutASortOrFilter(t *testing.T) {
	t.Parallel()
	for view, want := range map[domain.View]bool{
		{}:                                       true,
		{Sort: domain.SortReturn}:                true,
		{Membership: domain.MembershipAll}:       true,
		{Sort: domain.SortPnL}:                   false,
		{Membership: domain.MembershipOpen}:      false,
		{Sort: domain.SortReturn, MinMembers: 5}: false,
	} {
		if got := view.IsDefault(); got != want {
			t.Errorf("%+v IsDefault = %v, want %v", view, got, want)
		}
	}
}

func TestSortEntriesOrdersByNameOrPnLAndBreaksTiesByRank(t *testing.T) {
	t.Parallel()
	entry := func(rank int, name string, pnl int64) domain.Entry {
		return domain.Entry{Rank: rank, Subject: domain.Subject{Name: name}, PnL: money.SignedMicrosFromInt64(pnl)}
	}
	rows := func() []domain.Entry {
		return []domain.Entry{entry(1, "b", 5), entry(2, "A", 9), entry(3, "a", 5), entry(4, "c", 9)}
	}
	ranks := func(sort domain.Sort) []int {
		in := rows()
		domain.SortEntries(in, sort)
		out := make([]int, len(in))
		for i, e := range in {
			out[i] = e.Rank
		}
		return out
	}
	for sort, want := range map[domain.Sort][]int{
		"":                {1, 2, 3, 4},
		domain.SortReturn: {1, 2, 3, 4},
		domain.SortPnL:    {2, 4, 1, 3},
		domain.SortName:   {2, 3, 1, 4},
	} {
		if got := ranks(sort); !slices.Equal(got, want) {
			t.Errorf("SortEntries(%q) ranks = %v, want %v", sort, got, want)
		}
	}
}
