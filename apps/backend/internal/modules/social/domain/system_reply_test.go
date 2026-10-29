package domain_test

import (
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/modules/social/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/domain/feed"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

func shares(v uint64) *money.BaseUnits {
	units := money.NewBaseUnits(v, 6)
	return &units
}

func TestSystemReplyText(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		outcome domain.SystemOutcome
		side    feed.Action
		symbol  string
		before  *money.BaseUnits
		after   *money.BaseUnits
		want    string
	}{
		{
			"buy with a holding", domain.OutcomeFilled, feed.ActionBuy, "NVDA", shares(38_000_000), shares(39_820_000),
			"Passed and filled. 38 → 39.82 sh NVDA in the treasury.",
		},
		{
			"buy with nothing held", domain.OutcomeFilled, feed.ActionBuy, "META", nil, shares(420_000),
			"Passed and filled. 0.42 sh META in the treasury.",
		},
		{
			"sell", domain.OutcomeFilled, feed.ActionSell, "TSLA", shares(10_500_000), shares(4_000_000),
			"Passed and filled. 10.5 → 4 sh TSLA in the treasury.",
		},
		{"unknown outcome", domain.SystemOutcome("other"), feed.ActionBuy, "NVDA", nil, nil, ""},
		{
			"filled without a holding read", domain.OutcomeFilled, feed.ActionBuy, "NVDA", nil, nil,
			"Passed and filled.",
		},
		{
			"sell failed", domain.OutcomeClosed, feed.ActionSell, "TSLA", nil, nil,
			"Closed without passing. Nothing was sold.",
		},
		{
			"buy expired", domain.OutcomeClosed, feed.ActionBuy, "NVDA", nil, nil,
			"Closed without passing. Nothing was bought.",
		},
		{
			"buy trade failed", domain.OutcomeTradeFailed, feed.ActionBuy, "NVDA", nil, nil,
			"Passed, but the trade didn't go through. Nothing was bought.",
		},
		{
			"sell trade failed", domain.OutcomeTradeFailed, feed.ActionSell, "NVDA", nil, nil,
			"Passed, but the trade didn't go through. Nothing was sold.",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := domain.SystemReplyText(tt.outcome, tt.side, tt.symbol, tt.before, tt.after); got != tt.want {
				t.Fatalf("SystemReplyText() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestFormatShares(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		units money.BaseUnits
		want  string
	}{
		{"zero", money.NewBaseUnits(0, 6), "0"},
		{"whole", money.NewBaseUnits(5_000_000, 6), "5"},
		{"two decimals", money.NewBaseUnits(39_820_000, 6), "39.82"},
		{"trailing zero dropped", money.NewBaseUnits(1_500_000, 6), "1.5"},
		{"small padded", money.NewBaseUnits(50_000, 6), "0.05"},
		{"rounds half up", money.NewBaseUnits(1_235_000, 6), "1.24"},
		{"rounds to whole", money.NewBaseUnits(999_600, 6), "1"},
		{"no decimals", money.NewBaseUnits(7, 0), "7"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := domain.FormatShares(tt.units); got != tt.want {
				t.Fatalf("FormatShares() = %q, want %q", got, tt.want)
			}
		})
	}
}
