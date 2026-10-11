package multichain_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	mc "github.com/monaco/monaco/apps/backend/internal/platform/chain/multichain"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/testkit/fakes/multichain"
)

var (
	_ mc.WalletActions     = (*multichain.WalletActions)(nil)
	_ mc.EVMWallets        = (*multichain.EVMWallets)(nil)
	_ mc.DepositForwarding = (*multichain.DepositForwarding)(nil)
	_ mc.Quoter            = (*multichain.Quoter)(nil)
	_ mc.Router            = (*multichain.Router)(nil)
	_ mc.EVMPriceSource    = (*multichain.EVMPriceSource)(nil)
)

func TestQuoterReplaysThePrivyBridgeQuote(t *testing.T) {
	t.Parallel()
	f := multichain.New()
	listing := mc.Listing{Network: chain.NetworkBase, Venue: mc.VenueUniswap}
	q, err := f.Quoter.Quote(context.Background(), listing, mc.SideBuy, money.MicrosFromUint64(10_000_000))
	if err != nil {
		t.Fatal(err)
	}
	if len(q.Costs) != 1 || q.Costs[0].Kind != mc.CostBridge || q.Costs[0].Micros.Uint64() != 7_500 {
		t.Fatalf("costs = %+v, want the 7.5 bps bridge cost Privy quoted for 10 USDC", q.Costs)
	}
	if q.NetMicros.Uint64() != 9_992_500 || q.Network != chain.NetworkBase || q.InputMicros.Uint64() != 10_000_000 {
		t.Fatalf("quote = %+v", q)
	}
}

func TestScriptedRepliesComeBeforeTheRecording(t *testing.T) {
	t.Parallel()
	f := multichain.New()
	f.WalletActions.Transfers.Push(
		multichain.Reply[mc.ActionID]{Value: "scripted"},
		multichain.Reply[mc.ActionID]{Code: errs.CodePrivyUnavailable},
	)
	ctx := context.Background()
	for _, want := range []struct {
		id   mc.ActionID
		code errs.Code
	}{{"scripted", ""}, {"", errs.CodePrivyUnavailable}, {"privy-action-transfer-1", ""}} {
		id, err := f.WalletActions.Transfer(ctx, "w", mc.TransferSpec{}, "k")
		if id != want.id || (err == nil) != (want.code == "") || err != nil && errs.CodeOf(err) != want.code {
			t.Fatalf("Transfer = %q, %v, want %q, %q", id, err, want.id, want.code)
		}
	}
	if f.WalletActions.Transfers.Calls() != 3 {
		t.Fatalf("Calls() = %d", f.WalletActions.Transfers.Calls())
	}
}

func TestEveryRecordingDecodes(t *testing.T) {
	t.Parallel()
	f := multichain.New()
	ctx := context.Background()
	status, err := f.WalletActions.Action(ctx, "w", "a")
	if err != nil || status.State != mc.ActionSucceeded || len(status.Steps) != 2 {
		t.Fatalf("Action = %+v, %v", status, err)
	}
	wallet, err := f.EVMWallets.CreateAppEVMWallet(ctx, "k")
	if err != nil || wallet.Address.String() != "0x5aaeb6053f3e94c9b9a09f33669435e7ef1beaed" {
		t.Fatalf("CreateAppEVMWallet = %+v, %v", wallet, err)
	}
	if _, err := f.EVMWallets.SignEVMTransaction(ctx, "w", mc.EVMTransaction{}, true); err != nil {
		t.Fatal(err)
	}
	if hash, err := f.EVMWallets.SendEVMTransaction(ctx, "w", mc.EVMTransaction{}, true); err != nil || hash == "" {
		t.Fatalf("SendEVMTransaction = %q, %v", hash, err)
	}
}

func TestDepositAndRoutingRecordingsDecode(t *testing.T) {
	t.Parallel()
	f := multichain.New()
	ctx := context.Background()
	if id, err := f.DepositForwarding.CreateAutomation(ctx); err != nil || id == "" {
		t.Fatalf("CreateAutomation = %q, %v", id, err)
	}
	if err := f.DepositForwarding.Attach(ctx, "w", "dest"); err != nil {
		t.Fatal(err)
	}
	page, err := f.DepositForwarding.Executions(ctx, "w", "")
	if err != nil || len(page.Executions) != 1 || page.Executions[0].AmountMicros.Uint64() != 9_992_500 {
		t.Fatalf("Executions = %+v, %v", page, err)
	}
	if err := f.DepositForwarding.Reindex(ctx, "w", chain.NetworkBase); err != nil {
		t.Fatal(err)
	}
	if _, err := f.Router.Best(ctx, mc.StockID{}, mc.SideBuy, money.Micros{}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.EVMPriceSource.Prices(ctx, nil); err != nil {
		t.Fatal(err)
	}
}

func TestMountScriptsAndReplaysOverHTTP(t *testing.T) {
	t.Parallel()
	mux := http.NewServeMux()
	multichain.New().Mount(mux)
	srv := httptest.NewServer(mux)
	defer srv.Close()
	post := func(path, body string) int {
		t.Helper()
		resp := send(t, srv.URL+path, body)
		defer func() { _ = resp.Body.Close() }()
		return resp.StatusCode
	}
	if code := post("/_multichain/evm_wallets_send", `{"value":"0xabc"}`); code != http.StatusNoContent {
		t.Fatalf("script status = %d", code)
	}
	resp := send(t, srv.URL+"/multichain/evm_wallets_send", "")
	defer func() { _ = resp.Body.Close() }()
	got, err := io.ReadAll(resp.Body)
	if err != nil || string(got) != `{"value":"0xabc"}` {
		t.Fatalf("replay = %s, %v", got, err)
	}
	if code := post("/_multichain/nope", `{}`); code != http.StatusNotFound {
		t.Fatalf("unknown script status = %d", code)
	}
}

func send(t *testing.T, url, body string) *http.Response {
	t.Helper()
	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, url, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	return resp
}
