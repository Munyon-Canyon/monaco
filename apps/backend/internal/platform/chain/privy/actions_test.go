package privy_test

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"os"
	"reflect"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/multichain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/privy"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/testkit/fakes"
)

const baseDest = "0x00000000000000000000000000000000000000aa"

func toBase(micros uint64) multichain.TransferSpec {
	return multichain.TransferSpec{
		From: chain.NetworkSolana, To: chain.NetworkBase, Destination: baseDest,
		AmountMicros: money.MicrosFromUint64(micros),
	}
}

func TestTransfer_signsTheKeyedBodyAndAPollFindsBothChainSteps(t *testing.T) {
	t.Parallel()
	c, u, _ := overFakes(t)
	id, err := c.Transfer(t.Context(), "wallet-member", toBase(1_500_000), "withdrawal:1")
	if err != nil || id == "" {
		t.Fatalf("Transfer = %q, %v", id, err)
	}
	again, err := c.Transfer(t.Context(), "wallet-member", toBase(1_500_000), "withdrawal:1")
	if err != nil || again != id {
		t.Fatalf("retry with the same key = %q, %v, want %q", again, err, id)
	}
	req := u.requests()[0]
	if req.path != "/privy/v1/wallets/wallet-member/transfer" ||
		req.header.Get("privy-idempotency-key") != "withdrawal:1" ||
		req.header.Get("privy-authorization-signature") == "" {
		t.Fatalf("request = %s %v", req.path, req.header)
	}
	var body map[string]any
	_ = json.Unmarshal([]byte(req.body), &body)
	want := `{"amount":"1.5","amount_type":"exact_input","destination":{"address":"` + baseDest +
		`","asset":"usdc","chain":"base"},"source":{"asset":"usdc","chain":"solana"}}`
	if got, _ := json.Marshal(body); string(got) != want {
		t.Fatalf("body = %s\nwant   %s", got, want)
	}
	assertTransferPolls(t, c, u, id)
}

func assertTransferPolls(t *testing.T, c *privy.Client, u *upstream, id multichain.ActionID) {
	t.Helper()
	status, err := c.Action(t.Context(), "wallet-member", id)
	if err != nil || status.ID != id || status.State != multichain.ActionSucceeded || len(status.Steps) != 2 {
		t.Fatalf("Action = %+v, %v", status, err)
	}
	if s := status.Steps[0]; s.Network != chain.NetworkSolana || s.State != multichain.StepConfirmed || s.TxHash == "" {
		t.Fatalf("source step = %+v", s)
	}
	if q := u.requests()[2].query; q != "include=steps" {
		t.Fatalf("action query = %q", q)
	}
	_, err = c.Action(t.Context(), "wallet-other", id)
	wantCode(t, err, errs.CodeNotFound)
}

func TestTransfer_sendsEachChainsCashAsset(t *testing.T) {
	t.Parallel()
	const solanaDest = "67detL1H8561xpCWPUqQoy2WKeGKqauqkXRkfRoFviW8"
	cases := map[string]struct {
		from, to    chain.Network
		dest        string
		source, out string
	}{
		"solana to robinhood": {chain.NetworkSolana, chain.NetworkRobinhood, baseDest, "usdc", "usdg"},
		"robinhood to solana": {chain.NetworkRobinhood, chain.NetworkSolana, solanaDest, "usdg", "usdc"},
		"solana to base":      {chain.NetworkSolana, chain.NetworkBase, baseDest, "usdc", "usdc"},
		"base to solana":      {chain.NetworkBase, chain.NetworkSolana, solanaDest, "usdc", "usdc"},
	}
	for name, tc := range cases {
		c, u, _ := overFakes(t)
		spec := multichain.TransferSpec{
			From: tc.from, To: tc.to, Destination: tc.dest, AmountMicros: money.MicrosFromUint64(5_000_000),
		}
		if _, err := c.Transfer(t.Context(), "wallet-member", spec, "k"); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		type leg struct {
			Asset string `json:"asset"`
			Chain string `json:"chain"`
		}
		var body struct {
			Source      leg `json:"source"`
			Destination leg `json:"destination"`
		}
		_ = json.Unmarshal([]byte(u.requests()[0].body), &body)
		if body.Source.Asset != tc.source || body.Source.Chain != string(tc.from) ||
			body.Destination.Asset != tc.out || body.Destination.Chain != string(tc.to) {
			t.Fatalf("%s: sent %+v, want %s on %s to %s on %s", name, body, tc.source, tc.from, tc.out, tc.to)
		}
	}
}

func TestTransfer_amountsAreDecimalUSDC(t *testing.T) {
	t.Parallel()
	for micros, want := range map[uint64]string{1_000_000: "1", 1: "0.000001", 12_340_000: "12.34", 5_000_001: "5.000001"} {
		c, u, _ := overFakes(t)
		if _, err := c.Transfer(t.Context(), "wallet-member", toBase(micros), "k"); err != nil {
			t.Fatal(err)
		}
		var body struct {
			Amount string `json:"amount"`
		}
		_ = json.Unmarshal([]byte(u.requests()[0].body), &body)
		if body.Amount != want {
			t.Fatalf("%d micros sent as %q, want %q", micros, body.Amount, want)
		}
	}
}

func TestTransfer_refusesBeforeCallingPrivy(t *testing.T) {
	t.Parallel()
	unknown := toBase(1)
	unknown.From = chain.Network("tron")
	solanaDest := toBase(1)
	solanaDest.To = chain.NetworkSolana
	cases := map[string]struct {
		spec multichain.TransferSpec
		key  string
		want errs.Code
	}{
		"no key":                {toBase(1), "", errs.CodeInvalidInput},
		"zero amount":           {toBase(0), "k", errs.CodeInvalidInput},
		"evm address on solana": {solanaDest, "k", errs.CodeInvalidInput},
		"unknown source chain":  {unknown, "k", errs.CodeInvalidNetwork},
	}
	for name, tc := range cases {
		c, u, _ := overFakes(t)
		_, err := c.Transfer(t.Context(), "wallet-member", tc.spec, tc.key)
		if errs.CodeOf(err) != tc.want || len(u.requests()) != 0 {
			t.Fatalf("%s: err = %v, %d requests", name, err, len(u.requests()))
		}
	}
	cfg := testConfig()
	cfg.Privy.AuthorizationPrivateKey = ""
	_, err := clientWith(cfg, nil, clock.Real{}).Transfer(t.Context(), "w", toBase(1), "k")
	wantCode(t, err, errs.CodeInternal)
}

func TestTransfer_aForgedSignatureIsRefused(t *testing.T) {
	t.Parallel()
	forger, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	der, _ := x509.MarshalPKCS8PrivateKey(forger)
	cfg := testConfig()
	cfg.Privy.AuthorizationPrivateKey = "wallet-auth:" + base64.StdEncoding.EncodeToString(der)
	_, err := clientWith(cfg, &upstream{handler: fakes.New()}, clock.Real{}).
		Transfer(t.Context(), "wallet-member", toBase(1), "k")
	wantCode(t, err, errs.CodeInternal)
}

func TestAction_decodesStrictly(t *testing.T) {
	t.Parallel()
	ok := `{"id":"a1","status":"pending","steps":[` +
		`{"type":"evm_transaction","status":"queued","caip2":"eip155:8453","transaction_hash":"0xab"},` +
		`{"type":"evm_user_operation","status":"reverted","caip2":"eip155:1","bundle_transaction_hash":"0xcd"},` +
		`{"type":"external_transaction","status":"preparing"}]}`
	got, err := client(replying(http.StatusOK, ok)).Action(t.Context(), "w", "a1")
	want := []multichain.ActionStep{
		{Network: chain.NetworkBase, TxHash: "0xab", State: multichain.StepPending},
		{Network: chain.NetworkEthereum, TxHash: "0xcd", State: multichain.StepFailed},
		{State: multichain.StepPending},
	}
	if err != nil || got.State != multichain.ActionPending || len(got.Steps) != 3 ||
		got.Steps[0] != want[0] || got.Steps[1] != want[1] || got.Steps[2] != want[2] {
		t.Fatalf("Action = %+v, %v", got, err)
	}
	for _, body := range []string{
		`{"id":"a1","status":"exploded"}`,
		`{"id":"","status":"pending"}`,
		`{"id":"a1","status":"pending","steps":[{"type":"svm_transaction","status":"teleported","caip2":""}]}`,
		`{"id":"a1","status":"pending","steps":[{"type":"carrier_pigeon","status":"pending"}]}`,
		`{"id":"a1","status":"pending","steps":[{"type":"evm_transaction","status":"pending","caip2":"eip155:999"}]}`,
		`not json`,
	} {
		_, err := client(replying(http.StatusOK, body)).Action(t.Context(), "w", "a1")
		wantCode(t, err, errs.CodeDecodeFailed)
		_, err = client(replying(http.StatusOK, body)).Transfer(t.Context(), "w", toBase(1), "k")
		wantCode(t, err, errs.CodeDecodeFailed)
	}
}

func TestAction_readsTheDeliveredAmountWhenTheSolanaLegIsExternal(t *testing.T) {
	t.Parallel()
	body := `{"id":"a1","status":"succeeded","destination_amount":"0.951225","steps":[` +
		`{"type":"evm_user_operation","status":"confirmed","caip2":"eip155:4663","bundle_transaction_hash":"0xe9"},` +
		`{"type":"external_transaction","status":"confirmed"}]}`
	got, err := client(replying(http.StatusOK, body)).Action(t.Context(), "w", "a1")
	if err != nil || got.Delivered != money.MicrosFromUint64(951_225) || len(got.Steps) != 2 ||
		got.Steps[0].Network != chain.NetworkRobinhood || got.Steps[1] != (multichain.ActionStep{State: multichain.StepConfirmed}) {
		t.Fatalf("Action = %+v, %v", got, err)
	}
	for _, amount := range []string{"0.9512251", "abc", "-1"} {
		_, err := client(replying(http.StatusOK, `{"id":"a1","status":"succeeded","destination_amount":"`+amount+`"}`)).
			Action(t.Context(), "w", "a1")
		wantCode(t, err, errs.CodeDecodeFailed)
	}
}

func TestAction_decodesARealBaseToSolanaResponseWithItsDeliveredAmount(t *testing.T) {
	t.Parallel()
	body, err := os.ReadFile("testdata/action_base_to_solana_succeeded.json")
	if err != nil {
		t.Fatal(err)
	}
	got, err := client(replying(http.StatusOK, string(body))).Action(t.Context(), "w", "a1")
	want := multichain.ActionStatus{
		ID: "cff0078b-a71b-44c3-adff-7d0e93e666f9", State: multichain.ActionSucceeded,
		Delivered: money.MicrosFromUint64(1_997_002),
		Steps: []multichain.ActionStep{
			{
				Network: chain.NetworkBase, State: multichain.StepConfirmed,
				TxHash: "0x541e45ddac84037e7e08cf9c5b1f090aeeff361047e6ed45749ea33547bc13f5",
			},
			{State: multichain.StepConfirmed},
		},
	}
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("Action = %+v, %v, want %+v", got, err, want)
	}
}

func TestCreateAppEVMWallet_isAppOwnedAndKeyed(t *testing.T) {
	t.Parallel()
	c, u, _ := overFakes(t)
	first, err := c.CreateAppEVMWallet(t.Context(), "evm-deposit:user-1")
	if err != nil || first.ID == "" || first.Address == (chain.EVMAddress{}) {
		t.Fatalf("CreateAppEVMWallet = %+v, %v", first, err)
	}
	again, err := c.CreateAppEVMWallet(t.Context(), "evm-deposit:user-1")
	if err != nil || again != first {
		t.Fatalf("same key = %+v, %v, want %+v", again, err, first)
	}
	other, _ := c.CreateAppEVMWallet(t.Context(), "evm-deposit:user-2")
	if other.ID == first.ID {
		t.Fatal("a second key reused the first wallet")
	}
	req := u.requests()[0]
	var body map[string]any
	_ = json.Unmarshal([]byte(req.body), &body)
	if req.header.Get("privy-idempotency-key") != "evm-deposit:user-1" || body["chain_type"] != "ethereum" ||
		body["owner_id"] != fakes.PrivyAuthorizationKeyID {
		t.Fatalf("create = %v %s", req.header, req.body)
	}
	_, err = c.CreateAppEVMWallet(t.Context(), "")
	wantCode(t, err, errs.CodeInvalidInput)
	for _, body := range []string{
		`{"id":"w","address":"not-hex","owner_id":"` + fakes.PrivyAuthorizationKeyID + `"}`,
		`{"id":"w","address":"` + baseDest + `","owner_id":"someone-else"}`,
	} {
		_, err := client(replying(http.StatusOK, body)).CreateAppEVMWallet(t.Context(), "k")
		wantCode(t, err, errs.CodeDecodeFailed)
	}
}

func TestCreateAppEVMWallet_refusesWithoutTheQuorumAndReportsPrivyFailures(t *testing.T) {
	t.Parallel()
	cfg := testConfig()
	cfg.Privy.AuthorizationKeyID = ""
	_, err := clientWith(cfg, nil, clock.Real{}).CreateAppEVMWallet(t.Context(), "k")
	wantCode(t, err, errs.CodeInternal)
	_, err = client(replying(http.StatusBadRequest, `{}`)).CreateAppEVMWallet(t.Context(), "k")
	wantCode(t, err, errs.CodeInvalidInput)
}
