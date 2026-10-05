package fakes

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
)

const (
	privyWebhookKey    = "bW9uYWNvLWZha2VzL3ByaXZ5LXdlYmhvb2s="
	PrivyWebhookSecret = "whsec_" + privyWebhookKey
)

type PrivyDeposit struct {
	Recipient chain.SolanaAddress
	Sender    chain.SolanaAddress
	Mint      chain.SolanaAddress
	Amount    uint64
	Signature chain.Signature
}

func PrivyFundsDeposited(d PrivyDeposit) []byte {
	body, _ := json.Marshal(map[string]any{
		"type": "wallet.funds_deposited", "wallet_id": "wallet-" + string(d.Recipient),
		"recipient": d.Recipient, "sender": d.Sender, "amount": strconv.FormatUint(d.Amount, 10),
		"transaction_hash": d.Signature, "asset": map[string]string{"type": "spl", "address": string(d.Mint)},
	})
	return body
}

func SignPrivyWebhook(id string, at time.Time, body []byte) http.Header {
	key, _ := base64.StdEncoding.DecodeString(privyWebhookKey)
	ts := strconv.FormatInt(at.Unix(), 10)
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(id + "." + ts + "."))
	mac.Write(body)
	h := http.Header{}
	h.Set("svix-id", id)
	h.Set("svix-timestamp", ts)
	h.Set("svix-signature", "v1,"+base64.StdEncoding.EncodeToString(mac.Sum(nil)))
	return h
}
