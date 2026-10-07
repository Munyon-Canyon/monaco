package app

import (
	"context"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

func TestCashOutAmount(t *testing.T) {
	t.Parallel()
	shares, micros := money.SharesUnitsFromUint64, money.MicrosFromUint64
	for _, tt := range []struct {
		cmd           CashOut
		member, total money.SharesUnits
		code          errs.Code
	}{
		{CashOut{PayoutMicros: micros(99_999)}, shares(100), shares(100), errs.CodeInvalidInput},
		{CashOut{PayoutMicros: micros(2_000_000)}, shares(100), shares(100), errs.CodeInsufficientShares},
		{CashOut{PayoutMicros: micros(100_000)}, shares(1), shares(0), errs.CodeInvalidInput},
		{CashOut{All: true}, shares(1), shares(1_000_000), errs.CodePotValueChanged},
		{CashOut{All: true}, shares(0), shares(100), errs.CodePotValueChanged},
	} {
		if _, _, err := cashOutAmount(tt.cmd, tt.member, tt.total, micros(1_000_000)); errs.CodeOf(err) != tt.code {
			t.Fatalf("code = %q, want %q", errs.CodeOf(err), tt.code)
		}
	}
	for _, cmd := range []CashOut{{All: true}, {PayoutMicros: micros(901_000)}} {
		units, payout, err := cashOutAmount(cmd, shares(100), shares(100), micros(1_000_000))
		if err != nil || units != shares(100) || payout != micros(1_000_000) {
			t.Fatalf("cashOutAmount = (%v, %v, %v)", units, payout, err)
		}
	}
}

func TestCashOutAmount_zeroPotIsPriceUnavailable(t *testing.T) {
	t.Parallel()
	shares := money.SharesUnitsFromUint64
	for _, cmd := range []CashOut{{All: true}, {PayoutMicros: money.MicrosFromUint64(100_000)}} {
		_, _, err := cashOutAmount(cmd, shares(100), shares(100), money.Micros{})
		if errs.CodeOf(err) != errs.CodePriceUnavailable {
			t.Fatalf("code = %q, want %q", errs.CodeOf(err), errs.CodePriceUnavailable)
		}
	}
}

func TestCashOutPauseFunc(t *testing.T) {
	t.Parallel()
	called := false
	pause, err := CashOutPauseFunc(func(context.Context, ids.CabalID) (CashOutPause, error) {
		called = true
		return CashOutPause{Paused: true}, nil
	}).IsPaused(t.Context(), ids.CabalID{})
	if err != nil || !called || !pause.Paused {
		t.Fatalf("pause = %#v, %v", pause, err)
	}
}

func TestPromotesRemainder(t *testing.T) {
	t.Parallel()
	shares, micros := money.SharesUnitsFromUint64, money.MicrosFromUint64
	for _, tt := range []struct {
		member, units, total uint64
		want                 bool
		code                 errs.Code
	}{
		{1, 1, 1, false, ""}, {100, 91, 100, true, ""}, {1, 2, 2, false, errs.CodeInvalidInput}, {2, 1, 0, false, errs.CodeInvalidInput},
	} {
		got, err := promotesRemainder(shares(tt.member), shares(tt.units), shares(tt.total), micros(1_000_000))
		if (tt.code != "" && errs.CodeOf(err) != tt.code) || (tt.code == "" && (got != tt.want || err != nil)) {
			t.Fatalf("promotesRemainder = (%t, %v)", got, err)
		}
	}
}
