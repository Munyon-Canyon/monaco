package feed_test

import (
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/domain/feed"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

func TestPayload_roundTripsAmountsAsIntegerStrings(t *testing.T) {
	t.Parallel()
	want := feed.Payload{
		CabalName: "Alpha", Symbol: "AAPLx", Action: feed.ActionBuy, USDCMicros: usd(500_000_001),
		ChangeBps: -507, TokenAmount: 7, Status: "open", StatusCode: "x",
		ExpiresAt: time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC),
	}
	raw := want.JSON()
	if got := string(raw); got != `{"cabal_name":"Alpha","symbol":"AAPLx","action":"buy",`+
		`"usdc_micros":"500000001","change_bps":-507,"token_amount":"7","status":"open","status_code":"x",`+
		`"expires_at":"2026-01-02T03:04:05Z"}` {
		t.Fatalf("JSON() = %s", got)
	}
	got, err := feed.ParsePayload(raw)
	if err != nil || got != want {
		t.Fatalf("ParsePayload = %+v, %v, want %+v", got, err, want)
	}
}

func TestParsePayload_refusesAFloatAmount(t *testing.T) {
	t.Parallel()
	_, err := feed.ParsePayload([]byte(`{"usdc_micros":1.5}`))
	if errs.CodeOf(err) != errs.CodeInternal {
		t.Fatalf("err = %v, want internal", err)
	}
}

func TestFillPrice(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct {
		usdc     uint64
		tokens   uint64
		decimals uint8
		want     uint64
	}{
		"whole price":      {500_000_000, 250_000_000, 8, 200_000_000},
		"truncates micros": {10_000_000, 3_000_000_000, 8, 333_333},
		"no tokens":        {1, 0, 8, 0},
		"decimals too big": {1, 1, 20, 0},
		"overflow":         {1 << 63, 1, 19, 0},
	} {
		if got := feed.FillPrice(money.MicrosFromUint64(tc.usdc), tc.tokens, tc.decimals).Uint64(); got != tc.want {
			t.Errorf("%s: FillPrice = %d, want %d", name, got, tc.want)
		}
	}
}
