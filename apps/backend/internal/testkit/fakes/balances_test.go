package fakes

import (
	"errors"
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/modules/funding"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

func TestBalances(t *testing.T) {
	t.Parallel()
	balances := NewBalances()
	user := ids.UserIDFrom(ids.Real{}.NewV7())
	want := funding.Balance{
		OnChainMicros: money.MicrosFromUint64(9_000_000), InFlightFundMicros: money.MicrosFromUint64(1_000_000),
		InFlightWithdrawalMicros: money.MicrosFromUint64(2_000_000), AvailableMicros: money.MicrosFromUint64(6_000_000),
		AsOf: time.Date(2026, time.October, 3, 0, 0, 0, 0, time.UTC),
	}
	balances.Set(user, want)
	got, err := balances.Available(t.Context(), user)
	if err != nil || got != want {
		t.Fatalf("Available = (%+v, %v), want (%+v, nil)", got, err, want)
	}
	unknown, err := balances.Available(t.Context(), ids.UserIDFrom(ids.Real{}.NewV7()))
	if err != nil || unknown != (funding.Balance{}) {
		t.Fatalf("unknown Available = (%+v, %v), want (zero, nil)", unknown, err)
	}
}

func TestBalances_Faults(t *testing.T) {
	t.Parallel()
	balances := NewBalances()
	once := errors.New("once")
	always := errors.New("always")
	balances.FailOnce("Available", once)
	balances.Fail("Available", always)
	if _, err := balances.Available(t.Context(), ids.UserIDFrom(ids.Real{}.NewV7())); !errors.Is(err, once) {
		t.Fatalf("first Available error = %v, want %v", err, once)
	}
	if _, err := balances.Available(t.Context(), ids.UserIDFrom(ids.Real{}.NewV7())); !errors.Is(err, always) {
		t.Fatalf("second Available error = %v, want %v", err, always)
	}
}
