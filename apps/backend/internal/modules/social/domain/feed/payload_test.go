package feed_test

import (
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/domain/feed"
)

func TestPayload_roundTripsAmountsAsIntegerStrings(t *testing.T) {
	t.Parallel()
	want := feed.Payload{
		CabalName: "Alpha", Symbol: "AAPLx", Action: feed.ActionBuy, USDCMicros: usd(500_000_001),
		ChangeBps: -507, VoterCount: 3, YesVotes: 2,
	}
	raw := want.JSON()
	if got := string(raw); got != `{"cabal_name":"Alpha","symbol":"AAPLx","action":"buy",`+
		`"usdc_micros":"500000001","change_bps":-507,"voter_count":3,"yes_votes":2}` {
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
