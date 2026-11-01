package trading_test

import (
	"strings"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/trading/app"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
)

func (e *venueEnv) inspect(t *testing.T, replies ...string) (app.EVMFill, error) {
	t.Helper()
	e.rpc.replies["/rpc"] = replies
	token, owner := evmAddress(t, venueToken), evmAddress(t, venueOwner)
	return e.venue.Inspect(t.Context(), chain.NetworkBase, venueHash, token, owner)
}

func TestEVMVenue_inspectReportsTheReceiptWithoutDemandingATransfer(t *testing.T) {
	t.Parallel()
	const none = `{"jsonrpc":"2.0","id":1,"result":null}`
	for name, tc := range map[string]struct {
		replies []string
		want    app.EVMFill
	}{
		"no receipt": {[]string{none, none}, app.EVMFill{State: app.FillPending}},
		"reverted":   {[]string{receiptReply("0x0")}, app.EVMFill{State: app.FillReverted}},
		"landed with a transfer": {
			[]string{receiptReply("0x1", transferLog(venueToken, venueOwner, "f4240"))},
			app.EVMFill{State: app.FillSucceeded, Out: 1_000_000},
		},
		"landed with no transfer to the owner": {
			[]string{receiptReply("0x1", transferLog(venueToken, "0x00000000000000000000000000000000000000cc", "64"))},
			app.EVMFill{State: app.FillSucceeded},
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			got, err := newVenueEnv(t).inspect(t, tc.replies...)
			if err != nil || got != tc.want {
				t.Fatalf("Inspect = %+v, %v, want %+v", got, err, tc.want)
			}
		})
	}
}

func TestEVMVenue_inspectRefusesWhatItCannotRead(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct {
		reply string
		want  errs.Code
	}{
		"rpc error": {`{"jsonrpc":"2.0","id":1,"error":{"code":-32000,"message":"header not found"}}`, errs.CodeRPCUnavailable},
		"overflow": {
			receiptReply("0x1", transferLog(venueToken, venueOwner, strings.Repeat("f", 16)),
				transferLog(venueToken, venueOwner, "1")), errs.CodeDecodeFailed,
		},
		"short data": {
			receiptReply("0x1", strings.Replace(transferLog(venueToken, venueOwner, "1"), `"data":"0x`, `"data":"0x0`, 1)),
			errs.CodeDecodeFailed,
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if _, err := newVenueEnv(t).inspect(t, tc.reply); errs.CodeOf(err) != tc.want {
				t.Fatalf("Inspect err = %v, want %s", err, tc.want)
			}
		})
	}
}
