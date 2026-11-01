package trading_test

import (
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/modules/trading/app"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
)

func userOperationReply(success string, receipt string) string {
	return `{"jsonrpc":"2.0","id":1,"result":{"success":` + success + `,"receipt":` + receipt + `}}`
}

func TestEVMVenue_fillResolvesAUserOperationHashToItsReceipt(t *testing.T) {
	t.Parallel()
	const none = `{"jsonrpc":"2.0","id":1,"result":null}`
	filled := receiptReply("0x1", transferLog(venueToken, venueOwner, "f4240"))
	receipt := filled[len(`{"jsonrpc":"2.0","id":1,"result":`) : len(filled)-1]
	for name, tc := range map[string]struct {
		replies []string
		want    app.EVMFill
	}{
		"executed": {[]string{none, userOperationReply("true", receipt)}, app.EVMFill{State: app.FillSucceeded, Out: 1_000_000}},
		"reverted": {[]string{none, userOperationReply("false", receipt)}, app.EVMFill{State: app.FillReverted}},
		"unknown":  {[]string{none, `{"jsonrpc":"2.0","id":1,"result":null}`}, app.EVMFill{State: app.FillPending}},
		"bundler does not serve it": {
			[]string{none, `{"jsonrpc":"2.0","id":1,"error":{"code":-32601,"message":"method not found"}}`},
			app.EVMFill{State: app.FillPending},
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			e := newVenueEnv(t)
			e.rpc.replies["/rpc"] = tc.replies
			got, err := e.venue.Fill(t.Context(), chain.NetworkBase, venueHash, evmAddress(t, venueToken),
				evmAddress(t, venueOwner))
			if err != nil || got != tc.want {
				t.Fatalf("Fill = %+v, %v, want %+v", got, err, tc.want)
			}
			calls := e.rpc.calls
			if len(calls) != 2 || calls[0].body["method"] != "eth_getTransactionReceipt" ||
				calls[1].body["method"] != "eth_getUserOperationReceipt" || calls[1].body["params"].([]any)[0] != venueHash {
				t.Fatalf("rpc calls = %+v, want the transaction receipt and then the user operation receipt", calls)
			}
		})
	}
}
