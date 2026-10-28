package domain_test

import (
	"math"
	"testing"
	"time"

	"pgregory.net/rapid"

	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/ranking/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

func TestRight_followsTheTableAroundTheReference(t *testing.T) {
	t.Parallel()
	ref := money.MicrosFromUint64(1_000_000)
	ends := map[string]money.Micros{
		"below": money.MicrosFromUint64(999_999),
		"equal": ref,
		"above": money.MicrosFromUint64(1_000_001),
	}
	tests := []struct {
		kind, side string
		want       map[string]bool
	}{
		{"buy", "yes", map[string]bool{"below": false, "equal": false, "above": true}},
		{"buy", "no", map[string]bool{"below": true, "equal": true, "above": false}},
		{"sell", "yes", map[string]bool{"below": true, "equal": false, "above": false}},
		{"sell", "no", map[string]bool{"below": false, "equal": true, "above": true}},
	}
	for _, tt := range tests {
		for name, end := range ends {
			if got := domain.Right(tt.kind, tt.side, ref, end); got != tt.want[name] {
				t.Errorf("Right(%s, %s, ref, %s) = %v, want %v", tt.kind, tt.side, name, got, tt.want[name])
			}
		}
	}
}

func TestRight_isFalseForAnUnknownKindOrSide(t *testing.T) {
	t.Parallel()
	ref := money.MicrosFromUint64(10)
	if domain.Right("rule", "yes", ref, money.MicrosFromUint64(11)) || domain.Right("buy", "maybe", ref, ref) {
		t.Fatal("Right() = true for an unknown kind or side")
	}
}

func TestRight_judgesASubBasisPointMoveOnTheRawPrices(t *testing.T) {
	t.Parallel()
	ref := money.MicrosFromUint64(716_520_000)
	up := money.MicrosFromUint64(716_520_001)
	down := money.MicrosFromUint64(716_519_999)
	if bps, err := domain.MoveBps(ref, up); err != nil || bps != 0 {
		t.Fatalf("MoveBps(rise) = %d, %v, want 0", bps, err)
	}
	if !domain.Right("buy", "yes", ref, up) || domain.Right("buy", "no", ref, up) {
		t.Error("a sub-basis-point rise must be a right buy Yes")
	}
	if !domain.Right("sell", "yes", ref, down) || domain.Right("sell", "no", ref, down) {
		t.Error("a sub-basis-point fall must be a right sell Yes")
	}
}

func TestMoveBps(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		ref, end uint64
		want     int
	}{
		{"rise", 1_000_000, 1_042_000, 420},
		{"fall", 1_000_000, 976_000, -240},
		{"unchanged", 1_000_000, 1_000_000, 0},
		{"rounds toward zero up", 3_000_000, 3_000_999, 3},
		{"rounds toward zero down", 3_000_000, 2_999_001, -3},
	}
	for _, tt := range tests {
		got, err := domain.MoveBps(money.MicrosFromUint64(tt.ref), money.MicrosFromUint64(tt.end))
		if err != nil || got != tt.want {
			t.Errorf("%s: MoveBps = %d, %v, want %d", tt.name, got, err, tt.want)
		}
	}
}

func TestMoveBps_errors(t *testing.T) {
	t.Parallel()
	if _, err := domain.MoveBps(money.Micros{}, money.MicrosFromUint64(1)); err == nil {
		t.Error("MoveBps with a zero reference error = nil")
	}
	if _, err := domain.MoveBps(money.MicrosFromUint64(1), money.MicrosFromUint64(math.MaxInt64)); err == nil {
		t.Error("MoveBps overflowing int32 error = nil")
	}
	if _, err := domain.MoveBps(money.MicrosFromUint64(1), money.MicrosFromUint64(math.MaxUint64)); err == nil {
		t.Error("MoveBps overflowing the delta error = nil")
	}
}

func TestFillPrice(t *testing.T) {
	t.Parallel()
	buy := events.TradeConfirmed{
		Action:     "buy",
		USDCMicros: money.MicrosFromUint64(716_520_000),
		OutAmount:  1_000_000,
		InAmount:   7,
	}
	if got, err := domain.FillPrice(buy, 6); err != nil || got.Uint64() != 716_520_000 {
		t.Errorf("FillPrice(buy) = %v, %v, want 716520000", got, err)
	}
	sell := events.TradeConfirmed{
		Action:     "sell",
		USDCMicros: money.MicrosFromUint64(100_000_000),
		InAmount:   500_000_000,
		OutAmount:  9,
	}
	if got, err := domain.FillPrice(sell, 8); err != nil || got.Uint64() != 20_000_000 {
		t.Errorf("FillPrice(sell) = %v, %v, want 20000000 from the in side at 8 decimals", got, err)
	}
	if _, err := domain.FillPrice(
		events.TradeConfirmed{Action: "buy", USDCMicros: money.MicrosFromUint64(5)},
		6,
	); err == nil {
		t.Error("FillPrice with a zero token amount error = nil")
	}
	if _, err := domain.FillPrice(events.TradeConfirmed{Action: "sell", OutAmount: 5}, 6); err == nil {
		t.Error("FillPrice of a sell with a zero in amount error = nil")
	}
}

func TestPeriodContaining_boundaries(t *testing.T) {
	t.Parallel()
	sunday := time.Date(2026, 10, 25, 23, 59, 59, 0, time.UTC)
	week := domain.PeriodContaining(domain.PeriodWeek, sunday)
	if want := time.Date(2026, 10, 19, 0, 0, 0, 0, time.UTC); !week.Start.Equal(want) {
		t.Errorf("week start = %s, want %s", week.Start, want)
	}
	if want := time.Date(2026, 10, 26, 0, 0, 0, 0, time.UTC); !week.End.Equal(want) {
		t.Errorf("week end = %s, want %s", week.End, want)
	}
	monday := domain.PeriodContaining(domain.PeriodWeek, week.End)
	if !monday.Start.Equal(week.End) {
		t.Errorf("Monday 00:00 starts the next week, got start %s", monday.Start)
	}
	month := domain.PeriodContaining(domain.PeriodMonth, time.Date(2026, 10, 31, 23, 59, 59, 0, time.UTC))
	if want := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC); !month.Start.Equal(want) {
		t.Errorf("month start = %s, want %s", month.Start, want)
	}
	if want := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC); !month.End.Equal(want) {
		t.Errorf("month end = %s, want %s", month.End, want)
	}
	zoned := time.Date(2026, 11, 1, 1, 0, 0, 0, time.FixedZone("x", 2*3600))
	if got := domain.PeriodContaining(domain.PeriodMonth, zoned); !got.Start.Equal(month.Start) {
		t.Errorf("a time in UTC+2 is still judged in UTC, got start %s", got.Start)
	}
	all := domain.PeriodContaining(domain.PeriodAll, sunday)
	if !all.Start.Before(sunday) || !all.End.After(sunday) {
		t.Errorf("all period %s..%s does not contain %s", all.Start, all.End, sunday)
	}
}

func TestProperty_RightBuyYesAndNoNeverBothHold(t *testing.T) {
	t.Parallel()
	rapid.Check(t, func(t *rapid.T) {
		ref := money.MicrosFromUint64(rapid.Uint64().Draw(t, "ref"))
		end := money.MicrosFromUint64(rapid.Uint64().Draw(t, "end"))
		if domain.Right("buy", "yes", ref, end) == domain.Right("buy", "no", ref, end) {
			t.Fatalf("buy yes and no agree at ref %v end %v", ref, end)
		}
		if domain.Right("sell", "yes", ref, end) == domain.Right("sell", "no", ref, end) {
			t.Fatalf("sell yes and no agree at ref %v end %v", ref, end)
		}
	})
}

func TestProperty_MoveBpsKeepsTheSignOfTheMove(t *testing.T) {
	t.Parallel()
	rapid.Check(t, func(t *rapid.T) {
		ref := rapid.Uint64Range(1, 1<<50).Draw(t, "ref")
		end := rapid.Uint64Range(0, 2*ref).Draw(t, "end")
		bps, err := domain.MoveBps(money.MicrosFromUint64(ref), money.MicrosFromUint64(end))
		if err != nil {
			t.Fatalf("MoveBps(%d, %d) = %v", ref, end, err)
		}
		if (end > ref && bps < 0) || (end < ref && bps > 0) || (end == ref && bps != 0) {
			t.Fatalf("MoveBps(%d, %d) = %d has the wrong sign", ref, end, bps)
		}
	})
}
