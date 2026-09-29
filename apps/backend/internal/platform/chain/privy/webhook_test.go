package privy_test

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"net/http"
	"strconv"
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/privy"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

const (
	webhookKey  = "c2lnbmluZy1rZXktZm9yLXRlc3Rz"
	depositBody = `{"type":"wallet.funds_deposited","wallet_id":"wallet-member",` +
		`"recipient":"Dht9c9YfstFWkNYXgqr8HZbhqVn563bCpNU6zL32Ftqf","sender":"F4nbnZw67wKUQGW46MSRwN23VAKQRhYUMEXt88GQwD9z",` +
		`"amount":"25000000","transaction_hash":"sig-1","asset":{"type":"spl","address":"EPjFWdd5AufqSSqeM2qN1xzybapC8G4wEGGkZwyTDt1v"}}`
)

func svix(id string, at time.Time, body string) http.Header {
	key, _ := base64.StdEncoding.DecodeString(webhookKey)
	ts := strconv.FormatInt(at.Unix(), 10)
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(id + "." + ts + "." + body))
	h := http.Header{}
	h.Set("svix-id", id)
	h.Set("svix-timestamp", ts)
	h.Set("svix-signature", "v1,bm9wZQ== v1,"+base64.StdEncoding.EncodeToString(mac.Sum(nil)))
	return h
}

func TestVerifyWebhook_decodesFundsDeposited(t *testing.T) {
	t.Parallel()
	clk := testkit.NewClock(clock.Real{}.Now())
	got, err := clientWith(
		testConfig(),
		nil,
		clk,
	).VerifyWebhook(svix("msg_1", clk.Now(), depositBody), []byte(depositBody))
	want := privy.Deposit{
		WalletID: "wallet-member", Recipient: "Dht9c9YfstFWkNYXgqr8HZbhqVn563bCpNU6zL32Ftqf",
		Sender: "F4nbnZw67wKUQGW46MSRwN23VAKQRhYUMEXt88GQwD9z", Mint: "EPjFWdd5AufqSSqeM2qN1xzybapC8G4wEGGkZwyTDt1v",
		Amount: 25_000_000, Signature: "sig-1",
	}
	if err != nil || got.ID != "msg_1" || got.Type != privy.FundsDeposited || got.Deposit == nil ||
		*got.Deposit != want {
		t.Fatalf("VerifyWebhook = %+v, %v", got, err)
	}
	other := `{"type":"user.created"}`
	got, err = clientWith(testConfig(), nil, clk).VerifyWebhook(svix("msg_2", clk.Now(), other), []byte(other))
	if err != nil || got.Type != "user.created" || got.Deposit != nil {
		t.Fatalf("other event = %+v, %v", got, err)
	}
}

func TestVerifyWebhook_refusals(t *testing.T) {
	t.Parallel()
	clk := testkit.NewClock(clock.Real{}.Now())
	now := clk.Now()
	tampered := svix("msg_1", now, depositBody)
	tampered.Set("svix-signature", "v2,"+tampered.Get("svix-signature")[len("v1,bm9wZQ== v1,"):])
	for name, tc := range map[string]struct {
		header http.Header
		body   string
		secret string
		want   errs.Code
	}{
		"wrong body":    {svix("msg_1", now, depositBody), `{"type":"x"}`, "", errs.CodeUnauthorized},
		"wrong version": {tampered, depositBody, "", errs.CodeUnauthorized},
		"stale":         {svix("msg_1", now.Add(-6*time.Minute), depositBody), depositBody, "", errs.CodeUnauthorized},
		"future":        {svix("msg_1", now.Add(6*time.Minute), depositBody), depositBody, "", errs.CodeUnauthorized},
		"no id":         {svix("", now, depositBody), depositBody, "", errs.CodeUnauthorized},
		"no secret":     {svix("msg_1", now, depositBody), depositBody, "whsec_", errs.CodeUnauthorized},
		"bad secret":    {svix("msg_1", now, depositBody), depositBody, "whsec_!!", errs.CodeUnauthorized},
		"not json":      {svix("msg_1", now, "nope"), "nope", "", errs.CodeInvalidInput},
		"bad amount":    {svix("msg_1", now, `{"type":"wallet.funds_deposited","amount":"x"}`), `{"type":"wallet.funds_deposited","amount":"x"}`, "", errs.CodeInvalidInput},
	} {
		cfg := testConfig()
		if tc.secret != "" {
			cfg.Privy.WebhookSecret = tc.secret
		}
		_, err := clientWith(cfg, nil, clk).VerifyWebhook(tc.header, []byte(tc.body))
		if errs.CodeOf(err) != tc.want {
			t.Fatalf("%s: err = %v, want %s", name, err, tc.want)
		}
	}
}
