package privy_test

import (
	"encoding/json"
	"math/big"
	"net/http"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/multichain"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
)

func swapTx(key string) multichain.EVMTransaction {
	to, _ := chain.ParseEVMAddress("0x00000000000000000000000000000000000000bb")
	return multichain.EVMTransaction{
		Network: chain.NetworkBase, To: to, Data: []byte{0xab, 0xcd}, Value: big.NewInt(255), IdempotencyKey: key,
	}
}

func TestSendEVMTransaction_sendsOnceAKeyAndRefusesItForAnotherBody(t *testing.T) {
	t.Parallel()
	c, u, srv := overFakes(t)
	hash, err := c.SendEVMTransaction(t.Context(), "wallet-evm", swapTx("trade:1:swap"), false)
	if err != nil || len(hash) != 66 {
		t.Fatalf("SendEVMTransaction = %q, %v", hash, err)
	}
	again, err := c.SendEVMTransaction(t.Context(), "wallet-evm", swapTx("trade:1:swap"), false)
	if err != nil || again != hash || srv.PrivyEVMSends() != 1 {
		t.Fatalf("replay = %q, %v with %d sends; want the same hash and one send", again, err, srv.PrivyEVMSends())
	}
	changed := swapTx("trade:1:swap")
	changed.Data = []byte{0xff}
	_, err = c.SendEVMTransaction(t.Context(), "wallet-evm", changed, false)
	wantCode(t, err, errs.CodeInvalidInput)
	req := u.requests()[0]
	var body map[string]any
	_ = json.Unmarshal([]byte(req.body), &body)
	want := `{"caip2":"eip155:8453","chain_type":"ethereum","method":"eth_sendTransaction","params":{"transaction":` +
		`{"chain_id":8453,"data":"0xabcd","to":"0x00000000000000000000000000000000000000bb","value":"0xff"}}}`
	if got, _ := json.Marshal(body); string(got) != want || req.header.Get("privy-idempotency-key") != "trade:1:swap" ||
		req.path != "/privy/v1/wallets/wallet-evm/rpc" {
		t.Fatalf("send = %s %s %v\nwant %s", req.path, got, req.header, want)
	}
}

func TestSendEVMTransaction_sponsoredReturnsTheUserOperationHash(t *testing.T) {
	t.Parallel()
	c, u, _ := overFakes(t)
	tx := swapTx("trade:2:swap")
	tx.Value = nil
	hash, err := c.SendEVMTransaction(t.Context(), "wallet-evm", tx, true)
	if err != nil || len(hash) != 66 {
		t.Fatalf("sponsored send = %q, %v", hash, err)
	}
	var body struct {
		Sponsor bool `json:"sponsor"`
		Params  struct {
			Transaction struct {
				Value string `json:"value"`
			} `json:"transaction"`
		} `json:"params"`
	}
	_ = json.Unmarshal([]byte(u.requests()[0].body), &body)
	if !body.Sponsor || body.Params.Transaction.Value != "0x0" {
		t.Fatalf("sponsored body = %+v", body)
	}
}

func TestEVMTransactions_refuseBeforeCallingPrivy(t *testing.T) {
	t.Parallel()
	solana := swapTx("k")
	solana.Network = chain.NetworkSolana
	negative := swapTx("k")
	negative.Value = big.NewInt(-1)
	c, u, _ := overFakes(t)
	for name, tc := range map[string]struct {
		tx      multichain.EVMTransaction
		sponsor bool
		want    errs.Code
	}{
		"no key":         {swapTx(""), false, errs.CodeInvalidInput},
		"solana":         {solana, false, errs.CodeInvalidNetwork},
		"negative value": {negative, false, errs.CodeInvalidInput},
	} {
		_, err := c.SendEVMTransaction(t.Context(), "w", tc.tx, tc.sponsor)
		if errs.CodeOf(err) != tc.want {
			t.Fatalf("send %s: err = %v, want %s", name, err, tc.want)
		}
	}
	_, err := c.SignEVMTransaction(t.Context(), "w", swapTx("k"), true)
	wantCode(t, err, errs.CodeInvalidInput)
	_, err = c.SignEVMTransaction(t.Context(), "w", solana, false)
	wantCode(t, err, errs.CodeInvalidNetwork)
	if n := len(u.requests()); n != 0 {
		t.Fatalf("%d requests reached Privy", n)
	}
	cfg := testConfig()
	cfg.Privy.AuthorizationPrivateKey = ""
	_, err = clientWith(cfg, nil, clock.Real{}).SendEVMTransaction(t.Context(), "w", swapTx("k"), false)
	wantCode(t, err, errs.CodeInternal)
}

func TestSignEVMTransaction_returnsTheSignedBytes(t *testing.T) {
	t.Parallel()
	c, u, _ := overFakes(t)
	signed, err := c.SignEVMTransaction(t.Context(), "wallet-evm", swapTx(""), false)
	if err != nil || len(signed) != 33 || signed[0] != 0x02 {
		t.Fatalf("SignEVMTransaction = %x, %v", signed, err)
	}
	if u.requests()[0].header.Get("privy-idempotency-key") != "" {
		t.Fatal("a sign without a key sent one")
	}
	for _, body := range []string{`{"data":{"signed_transaction":"02ab"}}`, `{"data":{"signed_transaction":"0xzz"}}`, `{}`} {
		_, err := client(replying(http.StatusOK, body)).SignEVMTransaction(t.Context(), "w", swapTx(""), false)
		wantCode(t, err, errs.CodeDecodeFailed)
	}
	_, err = client(replying(http.StatusBadRequest, `{}`)).SignEVMTransaction(t.Context(), "w", swapTx(""), false)
	wantCode(t, err, errs.CodeInvalidInput)
}

func TestSendEVMTransaction_decodesTheHashStrictly(t *testing.T) {
	t.Parallel()
	for _, body := range []string{`{"data":{"hash":"0xabc"}}`, `{"data":{"hash":"","user_operation_hash":""}}`, `{}`} {
		_, err := client(replying(http.StatusOK, body)).SendEVMTransaction(t.Context(), "w", swapTx("k"), false)
		wantCode(t, err, errs.CodeDecodeFailed)
	}
	_, err := client(
		replying(http.StatusServiceUnavailable, `{}`),
	).SendEVMTransaction(t.Context(), "w", swapTx("k"), false)
	wantCode(t, err, errs.CodePrivyUnavailable)
}

func TestTransfer_theFakeCanRejectFailAndRefuseAChangedReplay(t *testing.T) {
	t.Parallel()
	for outcome, want := range map[string]multichain.ActionState{
		"rejected": multichain.ActionRejected, "failed": multichain.ActionFailed,
	} {
		c, _, srv := overFakes(t)
		srv.SetPrivyActionOutcome(outcome)
		id, err := c.Transfer(t.Context(), "wallet-member", toBase(1_000_000), "k")
		if err != nil {
			t.Fatal(err)
		}
		status, err := c.Action(t.Context(), "wallet-member", id)
		if err != nil || status.State != want || len(status.Steps) != 1 ||
			status.Steps[0].State != multichain.StepFailed {
			t.Fatalf("%s: Action = %+v, %v", outcome, status, err)
		}
	}
	c, _, _ := overFakes(t)
	if _, err := c.Transfer(t.Context(), "wallet-member", toBase(1_000_000), "k"); err != nil {
		t.Fatal(err)
	}
	_, err := c.Transfer(t.Context(), "wallet-member", toBase(2_000_000), "k")
	wantCode(t, err, errs.CodeInvalidInput)
}
