package feed_test

import (
	"encoding/json"
	"errors"
	"math"
	"reflect"
	"strconv"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/domain/feed"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

func fillBase() time.Time { return time.Date(2026, 10, 3, 8, 41, 0, 0, time.UTC) }

func swapID(n int) uuid.UUID { return uuid.NewSHA1(uuid.Nil, []byte(strconv.Itoa(n))) }

func fillOf(tokens, micros uint64, minutes int) feed.Fill {
	return wideFillOf(money.UnitsFromUint64(tokens), micros, minutes)
}

func wideFillOf(tokens money.Units, micros uint64, minutes int) feed.Fill {
	return feed.Fill{
		TokenAmount: feed.NewTokens(tokens, 8), USDCMicros: money.MicrosFromUint64(micros),
		FilledAt: fillBase().Add(time.Duration(minutes) * time.Minute),
	}
}

func parseUnits(t *testing.T, raw string) money.Units {
	t.Helper()
	u, err := money.ParseUnits(raw)
	if err != nil {
		t.Fatal(err)
	}
	return u
}

func passed() feed.Payload { return feed.Payload{Outcome: feed.OutcomePassed} }

func TestDeriveFill_namesTheStateOfTheOrder(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		name  string
		build func(*feed.Payload) error
		want  string
	}{
		{"no trade event yet", func(*feed.Payload) error { return nil }, feed.FillPending},
		{"one of two confirmed", func(p *feed.Payload) error { return p.RecordFill(swapID(1), fillOf(1, 1, 0), 2) }, feed.FillPending},
		{"every swap confirmed", func(p *feed.Payload) error {
			return errors.Join(p.RecordFill(swapID(1), fillOf(1, 1, 0), 2), p.RecordFill(swapID(2), fillOf(1, 1, 0), 2))
		}, feed.FillFilled},
		{"one confirmed one failed", func(p *feed.Payload) error {
			return errors.Join(p.RecordFill(swapID(1), fillOf(1, 1, 0), 2), p.RecordFailure(swapID(2), "x", 2))
		}, feed.FillPartial},
		{"every swap failed", func(p *feed.Payload) error {
			return errors.Join(p.RecordFailure(swapID(1), "x", 2), p.RecordFailure(swapID(2), "x", 2))
		}, feed.FillFailed},
		{"failed with no batch", func(p *feed.Payload) error { return p.RecordFailure(swapID(1), "x", 0) }, feed.FillFailed},
		{"one of two failed", func(p *feed.Payload) error { return p.RecordFailure(swapID(1), "x", 2) }, feed.FillPending},
	} {
		p := passed()
		if err := c.build(&p); err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		if got, _, _, _, err := feed.DeriveFill(p); err != nil || got != c.want {
			t.Errorf("%s: state = %q, %v, want %q", c.name, got, err, c.want)
		}
	}
}

func TestDeriveFill_aVoteThatDidNotPassHasNoState(t *testing.T) {
	t.Parallel()
	p := feed.Payload{Outcome: feed.OutcomeFailed}
	if got, _, _, _, err := feed.DeriveFill(p); err != nil || got != "" {
		t.Fatalf("state = %q, %v, want none", got, err)
	}
}

func TestDeriveFill_sumsTheConfirmedSwapsAndNamesTheLatest(t *testing.T) {
	t.Parallel()
	p := passed()
	if err := p.RecordFill(swapID(1), fillOf(100, 10, 5), 2); err != nil {
		t.Fatal(err)
	}
	if err := p.RecordFill(swapID(2), fillOf(50, 7, 9), 2); err != nil {
		t.Fatal(err)
	}
	_, tokens, usdc, at, err := feed.DeriveFill(p)
	if err != nil || !tokens.Amount.Equal(money.UnitsFromUint64(150)) || tokens.Decimals != 8 || usdc.Uint64() != 17 ||
		!at.Equal(fillBase().Add(9*time.Minute)) {
		t.Fatalf("sums = %s %s %s, %v", tokens.Amount, usdc, at, err)
	}
}

func TestDeriveFill_anOverflowIsAnError(t *testing.T) {
	t.Parallel()
	max256 := parseUnits(t, "115792089237316195423570985008687907853269984665640564039457584007913129639935")
	for name, fills := range map[string][2]feed.Fill{
		"usdc":   {fillOf(1, math.MaxUint64, 0), fillOf(1, 1, 0)},
		"tokens": {wideFillOf(max256, 1, 0), fillOf(1, 1, 0)},
	} {
		p := passed()
		if err := errors.Join(p.RecordFill(swapID(1), fills[0], 2), p.RecordFill(swapID(2), fills[1], 2)); err != nil {
			t.Fatal(err)
		}
		if _, _, _, _, err := feed.DeriveFill(p); err == nil {
			t.Errorf("%s: DeriveFill = nil, want an overflow error", name)
		}
	}
}

func TestDeriveFill_fillsOfDifferentDecimalsAreAnError(t *testing.T) {
	t.Parallel()
	odd := fillOf(1, 1, 0)
	odd.TokenAmount = feed.NewTokens(odd.TokenAmount.Amount, 6)
	p := passed()
	if err := errors.Join(p.RecordFill(swapID(1), fillOf(1, 1, 0), 2), p.RecordFill(swapID(2), odd, 2)); err != nil {
		t.Fatal(err)
	}
	if _, _, _, _, err := feed.DeriveFill(p); err == nil {
		t.Fatal("DeriveFill of fills with different decimals = nil, want an error")
	}
}

func TestRecord_theFirstOutcomeOfASwapWins(t *testing.T) {
	t.Parallel()
	p := passed()
	if err := p.RecordFill(swapID(1), fillOf(1, 1, 0), 1); err != nil {
		t.Fatal(err)
	}
	if err := p.RecordFailure(swapID(1), "x", 1); err != nil || len(p.Failures) != 0 {
		t.Fatalf("failure after a fill: %v, failures %v, want a no-op", err, p.Failures)
	}
	if err := p.RecordFill(
		swapID(1),
		fillOf(9, 9, 0),
		1,
	); err != nil ||
		p.Fills[swapID(1).String()].USDCMicros.Uint64() != 1 {
		t.Fatalf("second fill: %v, fills %v, want the first kept", err, p.Fills)
	}
}

func TestRecord_aDifferentBatchSizeIsAMismatch(t *testing.T) {
	t.Parallel()
	p := passed()
	if err := p.RecordFill(swapID(1), fillOf(1, 1, 0), 2); err != nil {
		t.Fatal(err)
	}
	for _, err := range []error{p.RecordFill(swapID(2), fillOf(1, 1, 0), 3), p.RecordFailure(swapID(3), "x", 1)} {
		if got := errs.CodeOf(err); got != errs.CodeFeedFillBatchMismatch {
			t.Fatalf("code = %s, want %s", got, errs.CodeFeedFillBatchMismatch)
		}
	}
	if len(p.Fills) != 1 || len(p.Failures) != 0 || p.ExpectedFills != 2 {
		t.Fatalf("payload = %+v, want it unchanged", p)
	}
}

func TestFill_roundTripsThroughThePayload(t *testing.T) {
	t.Parallel()
	p := passed()
	if err := p.RecordFill(swapID(1), fillOf(42_000_000, 300_000_000, 0), 1); err != nil {
		t.Fatal(err)
	}
	var wire struct {
		Fills map[string]json.RawMessage `json:"fills"`
	}
	if err := json.Unmarshal(p.JSON(), &wire); err != nil {
		t.Fatal(err)
	}
	if got := string(wire.Fills[swapID(1).String()]); got !=
		`{"token_amount":"42000000","token_decimals":8,"usdc_micros":"300000000","filled_at":"2026-10-03T08:41:00Z"}` {
		t.Fatalf("fill wire = %s", got)
	}
	back, err := feed.ParsePayload(p.JSON())
	if err != nil || !reflect.DeepEqual(back.Fills[swapID(1).String()], p.Fills[swapID(1).String()]) {
		t.Fatalf("round trip = %+v, %v", back, err)
	}
	if _, err := feed.ParsePayload([]byte(`{"fills":{"a":{"token_amount":"x"}}}`)); err == nil {
		t.Fatal("ParsePayload of a bad token amount = nil, want an error")
	}
}

func TestFill_aMalformedFillIsAnError(t *testing.T) {
	t.Parallel()
	if _, err := feed.ParsePayload([]byte(`{"fills":{"a":"x"}}`)); err == nil {
		t.Fatal("ParsePayload of a fill that is not an object = nil, want an error")
	}
}

func TestDeriveFill_aFillPastUint64RoundTripsAndAggregates(t *testing.T) {
	t.Parallel()
	first, second := parseUnits(t, "25000000000000000000"), parseUnits(t, "5000000000000000000")
	p := passed()
	if err := errors.Join(
		p.RecordFill(swapID(1), wideFillOf(first, 300_000_000, 0), 2),
		p.RecordFill(swapID(2), wideFillOf(second, 60_000_000, 3), 2),
	); err != nil {
		t.Fatal(err)
	}
	var wire struct {
		Fills map[string]json.RawMessage `json:"fills"`
	}
	if err := json.Unmarshal(p.JSON(), &wire); err != nil {
		t.Fatal(err)
	}
	if got := string(wire.Fills[swapID(1).String()]); got !=
		`{"token_amount":"25000000000000000000","token_decimals":8,"usdc_micros":"300000000",`+
			`"filled_at":"2026-10-03T08:41:00Z"}` {
		t.Fatalf("fill wire = %s", got)
	}
	back, err := feed.ParsePayload(p.JSON())
	if err != nil {
		t.Fatal(err)
	}
	if got := back.Fills[swapID(1).String()].TokenAmount.Amount; !got.Equal(first) {
		t.Fatalf("round trip amount = %s, want %s", got, first)
	}
	state, tokens, usdc, _, err := feed.DeriveFill(back)
	if err != nil || state != feed.FillFilled || tokens.Amount.String() != "30000000000000000000" ||
		usdc.Uint64() != 360_000_000 {
		t.Fatalf("DeriveFill = %q %s %s, %v, want filled 30e18 360000000", state, tokens.Amount, usdc, err)
	}
}

func TestPayload_aTokenAmountPastUint64RoundTrips(t *testing.T) {
	t.Parallel()
	want := feed.Payload{Action: feed.ActionBuy, TokenAmount: parseUnits(t, "25000000000000000000"), TokenDecimals: 18}
	raw := want.JSON()
	if got := string(raw); got !=
		`{"action":"buy","token_amount":"25000000000000000000","token_decimals":18}` {
		t.Fatalf("JSON() = %s", got)
	}
	got, err := feed.ParsePayload(raw)
	if err != nil || !got.TokenAmount.Equal(want.TokenAmount) || got.TokenDecimals != 18 {
		t.Fatalf("ParsePayload = %+v, %v", got, err)
	}
	old, err := feed.ParsePayload([]byte(`{"token_amount":"18446744073709551615"}`))
	if err != nil || old.TokenAmount.String() != "18446744073709551615" {
		t.Fatalf("old payload = %+v, %v", old, err)
	}
}
