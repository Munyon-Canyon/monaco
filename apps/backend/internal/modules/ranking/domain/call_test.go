package domain_test

import (
	"math"
	"testing"
	"time"

	"pgregory.net/rapid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/ranking/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

func TestRight_followsTheTableAtEachSideOfTheReference(t *testing.T) {
	t.Parallel()
	ref := usd(716_520_000)
	below, above := usd(ref.Uint64()-1), usd(ref.Uint64()+1)
	for name, tc := range map[string]struct {
		kind                 domain.Kind
		side                 domain.Side
		wantBelow, wantEqual bool
		wantAbove            bool
	}{
		"buy yes":  {domain.KindBuy, domain.SideYes, false, false, true},
		"buy no":   {domain.KindBuy, domain.SideNo, true, true, false},
		"sell yes": {domain.KindSell, domain.SideYes, true, false, false},
		"sell no":  {domain.KindSell, domain.SideNo, false, true, true},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			for label, got := range map[string][2]bool{
				"one micro below": {domain.Right(tc.kind, tc.side, ref, below), tc.wantBelow},
				"equal":           {domain.Right(tc.kind, tc.side, ref, ref), tc.wantEqual},
				"one micro above": {domain.Right(tc.kind, tc.side, ref, above), tc.wantAbove},
			} {
				if got[0] != got[1] {
					t.Errorf("Right(%s, %s) %s = %t, want %t", tc.kind, tc.side, label, got[0], got[1])
				}
			}
		})
	}
}

func TestRight_anUnknownKindOrSideIsNeverRight(t *testing.T) {
	t.Parallel()
	ref, end := usd(100), usd(200)
	if domain.Right("rule", domain.SideYes, ref, end) || domain.Right(domain.KindBuy, "maybe", ref, end) {
		t.Fatal("Right accepted a kind or side outside the table")
	}
}

func TestRight_aRiseUnderOneBasisPointIsStillARightBuyYes(t *testing.T) {
	t.Parallel()
	ref, end := usd(716_520_000), usd(716_520_001)
	got, err := domain.MoveBps(ref, end)
	if err != nil || got != 0 {
		t.Fatalf("MoveBps = %d, %v, want 0 for a one micro rise", got, err)
	}
	if !domain.Right(domain.KindBuy, domain.SideYes, ref, end) {
		t.Error("a buy Yes is wrong after a sub-basis-point rise, want right: direction comes from raw prices")
	}
	if domain.Right(domain.KindBuy, domain.SideNo, ref, end) {
		t.Error("a buy No is right after a sub-basis-point rise, want wrong")
	}
	if !domain.Right(domain.KindSell, domain.SideYes, ref, usd(716_519_999)) {
		t.Error("a sell Yes is wrong after a sub-basis-point fall, want right")
	}
}

func TestMoveBps_isSignedAndRoundsTowardZero(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct {
		ref, end uint64
		want     int
	}{
		"up 4.2 percent":            {192_600_000, 200_689_200, 420},
		"down 4.2 percent":          {192_600_000, 184_510_800, -420},
		"fractions truncate up":     {1_000_000, 1_000_099, 0},
		"fractions truncate down":   {1_000_000, 999_901, 0},
		"just over one basis point": {1_000_000, 1_000_100, 1},
		"flat":                      {716_520_000, 716_520_000, 0},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			got, err := domain.MoveBps(usd(tc.ref), usd(tc.end))
			if err != nil || got != tc.want {
				t.Fatalf("MoveBps(%d, %d) = %d, %v, want %d", tc.ref, tc.end, got, err, tc.want)
			}
		})
	}
}

func TestMoveBps_refusesWhatItCannotRepresent(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct{ ref, end money.Micros }{
		"a zero reference":              {usd(0), usd(1)},
		"a gap past int64":              {usd(1), usd(math.MaxUint64)},
		"a gap whose scaling overflows": {usd(1), usd(math.MaxInt64)},
		"more basis points than int32":  {usd(1), usd(math.MaxInt32)},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if got, err := domain.MoveBps(tc.ref, tc.end); errs.CodeOf(err) != errs.CodeInvalidInput {
				t.Fatalf("MoveBps = %d, %v, want invalid_input", got, err)
			}
		})
	}
}

func TestFillPrice_usesTheTokenSideOfTheTradeAndTheAssetDecimals(t *testing.T) {
	t.Parallel()
	buy := events.TradeConfirmed{
		V: 1, Action: "buy", InAmount: 716_520_000, OutAmount: 1_000_000, USDCMicros: usd(716_520_000),
	}
	got, err := domain.FillPrice(buy, 6)
	if err != nil || got.Uint64() != 716_520_000 {
		t.Fatalf("FillPrice(buy) = %v, %v, want 716520000", got, err)
	}
	sell := events.TradeConfirmed{
		V: 1, Action: "sell", InAmount: 2_000_000_000, OutAmount: 385_200_000, USDCMicros: usd(385_200_000),
	}
	got, err = domain.FillPrice(sell, 9)
	if err != nil || got.Uint64() != 192_600_000 {
		t.Fatalf("FillPrice(sell) = %v, %v, want 192600000 from the in mint's 9 decimals", got, err)
	}
}

func TestFillPrice_refusesAZeroPrice(t *testing.T) {
	t.Parallel()
	for name, ev := range map[string]events.TradeConfirmed{
		"no tokens out on a buy": {V: 1, Action: "buy", InAmount: 5, USDCMicros: usd(5)},
		"no tokens in on a sell": {V: 1, Action: "sell", OutAmount: 5, USDCMicros: usd(5)},
		"no cash":                {V: 1, Action: "buy", OutAmount: 5},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if got, err := domain.FillPrice(ev, 6); errs.CodeOf(err) != errs.CodeInvalidInput {
				t.Fatalf("FillPrice = %v, %v, want invalid_input", got, err)
			}
		})
	}
}

func TestPeriodContaining_attributesTheBoundaryInstantsToTheirOwnPeriod(t *testing.T) {
	t.Parallel()
	at := func(m time.Month, d, h, mi, s int) time.Time { return time.Date(2026, m, d, h, mi, s, 0, time.UTC) }
	for name, tc := range map[string]struct {
		kind       domain.PeriodKind
		t          time.Time
		start, end time.Time
	}{
		"sunday night belongs to its week": {
			domain.PeriodWeek, at(10, 18, 23, 59, 59), at(10, 12, 0, 0, 0), at(10, 19, 0, 0, 0),
		},
		"monday midnight opens the next week": {
			domain.PeriodWeek, at(10, 19, 0, 0, 0), at(10, 19, 0, 0, 0), at(10, 26, 0, 0, 0),
		},
		"midweek": {domain.PeriodWeek, at(10, 21, 15, 4, 5), at(10, 19, 0, 0, 0), at(10, 26, 0, 0, 0)},
		"last second of october": {
			domain.PeriodMonth, at(10, 31, 23, 59, 59), at(10, 1, 0, 0, 0), at(11, 1, 0, 0, 0),
		},
		"first instant of november": {
			domain.PeriodMonth, at(11, 1, 0, 0, 0), at(11, 1, 0, 0, 0), at(12, 1, 0, 0, 0),
		},
		"a week across a month end": {domain.PeriodWeek, at(11, 1, 9, 0, 0), at(10, 26, 0, 0, 0), at(11, 2, 0, 0, 0)},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			got, err := domain.PeriodContaining(tc.kind, tc.t)
			if err != nil || !got.Start.Equal(tc.start) || !got.End.Equal(tc.end) || got.Kind != tc.kind {
				t.Fatalf("PeriodContaining(%s, %s) = %+v, %v, want [%s, %s)", tc.kind, tc.t, got, err, tc.start, tc.end)
			}
		})
	}
}

func TestPeriodContaining_readsTheInstantInUTC(t *testing.T) {
	t.Parallel()
	tokyo := time.FixedZone("JST", 9*3600)
	local := time.Date(2026, time.November, 1, 5, 0, 0, 0, tokyo)
	got, err := domain.PeriodContaining(domain.PeriodMonth, local)
	if err != nil || got.Start.Month() != time.October {
		t.Fatalf(
			"PeriodContaining(month, %s) = %+v, %v, want October: the instant is still Oct 31 in UTC",
			local,
			got,
			err,
		)
	}
}

func TestPeriodContaining_allTimeCoversEveryInstantAndAnUnknownKindFails(t *testing.T) {
	t.Parallel()
	all, err := domain.PeriodContaining(domain.PeriodAll, time.Date(2026, time.October, 28, 0, 0, 0, 0, time.UTC))
	if err != nil || all.Start.After(time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)) ||
		all.End.Before(time.Date(2100, time.January, 1, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("PeriodContaining(all) = %+v, %v, want a window from the epoch to the far future", all, err)
	}
	if _, err := domain.PeriodContaining(
		"season",
		time.Date(2026, time.October, 28, 0, 0, 0, 0, time.UTC),
	); errs.CodeOf(
		err,
	) != errs.CodeInvalidInput {
		t.Fatalf("PeriodContaining(season) err = %v, want invalid_input", err)
	}
}

func TestProperty_RightIsNeverBothYesAndNoAndMoveBpsKeepsTheSign(t *testing.T) {
	t.Parallel()
	rapid.Check(t, func(t *rapid.T) {
		ref := usd(rapid.Uint64Range(1, 1<<50).Draw(t, "ref"))
		end := usd(rapid.Uint64Range(0, 1<<51).Draw(t, "end"))
		for _, kind := range []domain.Kind{domain.KindBuy, domain.KindSell} {
			if domain.Right(kind, domain.SideYes, ref, end) == domain.Right(kind, domain.SideNo, ref, end) {
				t.Fatalf(
					"Right(%s) agrees for Yes and No at ref %v end %v: exactly one side must be right",
					kind,
					ref,
					end,
				)
			}
		}
		bps, err := domain.MoveBps(ref, end)
		if err != nil {
			return
		}
		switch cmp := end.Cmp(ref); {
		case cmp > 0 && bps < 0, cmp < 0 && bps > 0, cmp == 0 && bps != 0:
			t.Fatalf("MoveBps(%v, %v) = %d, want the sign of end minus ref (%d)", ref, end, bps, cmp)
		}
	})
}
