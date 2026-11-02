package domain

import (
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

const maxUint256 = "115792089237316195423570985008687907853269984665640564039457584007913129639935"

func mustUnits(t *testing.T, raw string) money.Units {
	t.Helper()
	v, err := money.ParseUnits(raw)
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func TestMinOut_aFloorThatCannotBeRepresentedIsZeroSoTheTradeIsRefused(t *testing.T) {
	t.Parallel()
	got := Slippage{bps: -1_000_000_000}.MinOut(mustUnits(t, maxUint256))
	if !got.IsZero() {
		t.Fatalf("MinOut = %s, want 0 for a floor past 2^256-1", got)
	}
}

func TestLotPart_aPartLargerThanTheLotIsCappedAtTheLot(t *testing.T) {
	t.Parallel()
	lot := Lot{Units: money.UnitsFromUint64(10), QuoteUSDC: 5}
	got, err := lot.part(7)
	if err != nil || !got.Units.Equal(lot.Units) || got.QuoteUSDC != 5 {
		t.Fatalf("part = %+v, %v, want the whole lot of 10 units quoted at 5", got, err)
	}
}

func TestLotPart_aPartWhoseUnitsPassUint256IsRefused(t *testing.T) {
	t.Parallel()
	lot := Lot{Units: mustUnits(t, maxUint256), QuoteUSDC: 1}
	if got, err := lot.part(1 << 40); err == nil {
		t.Fatalf("part = %+v, want an error past 2^256-1", got)
	}
}

func TestParseTokenAmount_refusesWhatIsNotAnUnsignedWholeNumber(t *testing.T) {
	t.Parallel()
	for _, raw := range []string{"-1", "1.5", "abc", "115792089237316195423570985008687907853269984665640564039457584007913129639936"} {
		if _, err := ParseTokenAmount(raw); errs.CodeOf(err) != errs.CodeDecodeFailed {
			t.Fatalf("ParseTokenAmount(%q) = %v, want decode_failed", raw, err)
		}
		if _, present, err := ParseOptionalTokenAmount(raw); present || errs.CodeOf(err) != errs.CodeDecodeFailed {
			t.Fatalf("ParseOptionalTokenAmount(%q) = %v, %v, want decode_failed and absent", raw, present, err)
		}
	}
	if v, present, err := ParseOptionalTokenAmount(""); err != nil || present || !v.IsZero() {
		t.Fatalf("ParseOptionalTokenAmount(\"\") = %s, %v, %v, want absent", v, present, err)
	}
}
