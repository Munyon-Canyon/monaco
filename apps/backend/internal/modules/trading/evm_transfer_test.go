package trading_test

import (
	"net/http"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/trading/app"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

const rhOther = "0x00000000000000000000000000000000000000aa"

func (c *fakeChain) transfer(t *testing.T, hash string) (app.EVMArrival, bool, error) {
	t.Helper()
	return c.arrivals(t, 10).Transfer(t.Context(), chain.NetworkRobinhood, evmAddress(t, rhUSDG),
		evmAddress(t, rhTreasury), hash)
}

func TestEVMTransfer_readsTheCashTransferToTheTreasuryByHash(t *testing.T) {
	t.Parallel()
	c := newFakeChain()
	c.logs = []chainLog{
		{block: 500, token: rhUSDG, from: rhBridge, to: rhOther, amount: 111, tx: txHash(1)},
		{block: 600, token: rhUSDG, from: rhBridge, to: rhTreasury, amount: 972_507, tx: txHash(2)},
	}

	got, found, err := c.transfer(t, txHash(2))

	want := app.EVMArrival{Amount: money.MicrosFromUint64(972_507), TxHash: txHash(2)}
	if err != nil || !found || got != want {
		t.Fatalf("Transfer = %+v, %v, %v, want %+v", got, found, err, want)
	}
}

func TestEVMTransfer_findsNothingForATransferThatIsNotCashToTheTreasury(t *testing.T) {
	t.Parallel()
	c := newFakeChain()
	c.logs = []chainLog{
		{block: 500, token: rhUSDG, from: rhBridge, to: rhOther, amount: 972_507, tx: txHash(1)},
		{block: 600, token: rhBridge, from: rhOther, to: rhTreasury, amount: 972_507, tx: txHash(2)},
		{block: 700, token: rhUSDG, from: rhBridge, to: rhTreasury, amount: 972_507, tx: txHash(3)},
	}
	c.reverted = map[string]bool{txHash(3): true}

	for name, hash := range map[string]string{
		"to someone else": txHash(1), "another token": txHash(2), "reverted": txHash(3), "unknown": txHash(4),
	} {
		if got, found, err := c.transfer(t, hash); err != nil || found || got != (app.EVMArrival{}) {
			t.Fatalf("%s: Transfer = %+v, %v, %v, want not found", name, got, found, err)
		}
	}
}

func TestEVMTransfer_waitsForTheTransferToSettle(t *testing.T) {
	t.Parallel()
	c := newFakeChain()
	c.logs = []chainLog{
		{block: chainHead - 10, token: rhUSDG, from: rhBridge, to: rhTreasury, amount: 1, tx: txHash(1)},
	}

	_, found, err := c.transfer(t, txHash(1))

	if found || errs.CodeOf(err) != errs.CodeUpstreamUnavailable {
		t.Fatalf("Transfer = %v, %v, want upstream_unavailable until it is past the safe depth", found, err)
	}
}

func TestEVMTransfer_refusesAHashWithSeveralTransfersToTheTreasury(t *testing.T) {
	t.Parallel()
	c := newFakeChain()
	c.logs = []chainLog{
		{block: 500, token: rhUSDG, from: rhBridge, to: rhTreasury, amount: 1, tx: txHash(1)},
		{block: 500, token: rhUSDG, from: rhOther, to: rhTreasury, amount: 2, tx: txHash(1)},
	}

	_, found, err := c.transfer(t, txHash(1))

	if found || errs.CodeOf(err) != errs.CodeInvalidInput {
		t.Fatalf("Transfer = %v, %v, want invalid_input", found, err)
	}
}

func TestEVMTransfer_refusesWhatItCannotRead(t *testing.T) {
	t.Parallel()
	ok := func(result string) (int, string) {
		return http.StatusOK, `{"jsonrpc":"2.0","id":1,"result":` + result + `}`
	}
	receipt := `{"status":"0x1","blockNumber":"0x10","logs":[{"address":"` + rhUSDG + `","topics":["` + transfer +
		`","` + topic(rhBridge) + `","` + topic(rhTreasury) + `"],"data":"0x01","transactionHash":"` + txHash(1) + `"}]}`
	for name, tc := range map[string]struct {
		reply func(string) (int, string)
		want  errs.Code
	}{
		"receipt outage": {func(string) (int, string) { return http.StatusServiceUnavailable, `{}` }, errs.CodeRPCUnavailable},
		"head outage": {func(method string) (int, string) {
			if method == "eth_getTransactionReceipt" {
				return ok(receipt)
			}
			return http.StatusServiceUnavailable, `{}`
		}, errs.CodeRPCUnavailable},
		"short transfer data": {func(method string) (int, string) {
			if method == "eth_getTransactionReceipt" {
				return ok(receipt)
			}
			return ok(`"0x1000"`)
		}, errs.CodeDecodeFailed},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			_, _, err := rawRPC(t, tc.reply).Transfer(t.Context(), chain.NetworkRobinhood, evmAddress(t, rhUSDG),
				evmAddress(t, rhTreasury), txHash(1))
			if errs.CodeOf(err) != tc.want {
				t.Fatalf("Transfer err = %v, want %s", err, tc.want)
			}
		})
	}
}

func TestEVMTransfer_refusesANetworkWithoutAnRPC(t *testing.T) {
	t.Parallel()

	_, _, err := newFakeChain().arrivals(t, 10).Transfer(t.Context(), chain.NetworkBase, evmAddress(t, rhUSDG),
		evmAddress(t, rhTreasury), txHash(1))

	if errs.CodeOf(err) != errs.CodeInvalidInput {
		t.Fatalf("Transfer err = %v, want invalid_input", err)
	}
}
