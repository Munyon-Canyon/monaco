package trading_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/trading/adapters"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpclient"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

type cashRPC struct {
	reply  string
	status int
	call   map[string]string
}

func (c *cashRPC) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Method string            `json:"method"`
		Params []json.RawMessage `json:"params"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)
	if req.Method == "eth_call" {
		_ = json.Unmarshal(req.Params[0], &c.call)
	}
	if c.status != 0 {
		w.WriteHeader(c.status)
	}
	_, _ = fmt.Fprint(w, c.reply)
}

func (c *cashRPC) cash(t *testing.T) *adapters.EVMCash {
	t.Helper()
	srv := httptest.NewServer(c)
	t.Cleanup(srv.Close)
	rpc := httpclient.New(
		"evm-rpc-robinhood-test", httpclient.WithBaseURL(srv.URL), httpclient.WithTimeout(time.Second),
	)
	return adapters.NewEVMCash(map[chain.Network]*httpclient.Client{chain.NetworkRobinhood: rpc})
}

func result(hex string) string { return `{"jsonrpc":"2.0","id":1,"result":"` + hex + `"}` }

func TestEVMCash_readsTheNetworksCashBalanceOfTheWallet(t *testing.T) {
	t.Parallel()
	c := &cashRPC{reply: result(fmt.Sprintf("0x%064x", 974_070))}

	got, err := c.cash(t).Balance(t.Context(), chain.NetworkRobinhood, evmAddress(t, rhTreasury))

	wantData := "0x70a08231" + strings.Repeat("0", 24) + strings.TrimPrefix(rhTreasury, "0x")
	if err != nil || got != money.MicrosFromUint64(974_070) {
		t.Fatalf("Balance = %s, %v, want 974070", got, err)
	}
	if c.call["to"] != rhUSDG || c.call["data"] != wantData {
		t.Fatalf("eth_call = %v, want balanceOf(treasury) on the network's cash token", c.call)
	}
}

func TestEVMCash_refusesWhatItCannotTrust(t *testing.T) {
	t.Parallel()
	cases := map[string]struct {
		rpc  cashRPC
		want errs.Code
	}{
		"empty result":      {cashRPC{reply: result("0x")}, errs.CodeDecodeFailed},
		"short result":      {cashRPC{reply: result("0x01")}, errs.CodeDecodeFailed},
		"not hex":           {cashRPC{reply: result("0x" + strings.Repeat("zz", 32))}, errs.CodeDecodeFailed},
		"wider than uint64": {cashRPC{reply: result("0x" + strings.Repeat("f", 64))}, errs.CodeDecodeFailed},
		"a node fault":      {cashRPC{reply: `{"error":{"code":-32000,"message":"boom"}}`}, errs.CodeRPCUnavailable},
		"a bad status":      {cashRPC{reply: "{}", status: http.StatusBadGateway}, errs.CodeRPCUnavailable},
		"a bad body":        {cashRPC{reply: "not json"}, errs.CodeDecodeFailed},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			_, err := tc.rpc.cash(t).Balance(t.Context(), chain.NetworkRobinhood, evmAddress(t, rhTreasury))
			if errs.CodeOf(err) != tc.want {
				t.Fatalf("Balance err = %v, want %s", err, tc.want)
			}
		})
	}
}

func TestEVMCash_refusesANetworkWithoutAnRPC(t *testing.T) {
	t.Parallel()
	_, err := adapters.NewEVMCash(nil).Balance(t.Context(), chain.NetworkBase, evmAddress(t, rhTreasury))
	if errs.CodeOf(err) != errs.CodeInvalidInput {
		t.Fatalf("Balance err = %v, want invalid_input", err)
	}
}
