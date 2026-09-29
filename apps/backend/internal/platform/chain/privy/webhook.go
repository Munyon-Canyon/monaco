package privy

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
)

const (
	FundsDeposited  = "wallet.funds_deposited"
	webhookTolerate = 5 * time.Minute
)

type WebhookEvent struct {
	ID      string
	Type    string
	Deposit *Deposit
}

type Deposit struct {
	WalletID  string
	Recipient chain.SolanaAddress
	Sender    chain.SolanaAddress
	Mint      chain.SolanaAddress
	Amount    uint64
	Signature chain.Signature
}

type depositWire struct {
	Type            string `json:"type"`
	WalletID        string `json:"wallet_id"`
	Recipient       string `json:"recipient"`
	Sender          string `json:"sender"`
	Amount          string `json:"amount"`
	TransactionHash string `json:"transaction_hash"`
	Asset           struct {
		Address string `json:"address"`
	} `json:"asset"`
}

func (c *Client) VerifyWebhook(header http.Header, body []byte) (WebhookEvent, error) {
	const op = "privy.VerifyWebhook"
	id, ts := header.Get("svix-id"), header.Get("svix-timestamp")
	if reason := c.webhookReason(id, ts, header.Get("svix-signature"), body); reason != "" {
		return WebhookEvent{}, errs.New(errs.CodeUnauthorized, op, slog.String("reason", reason))
	}
	var w depositWire
	if err := json.Unmarshal(body, &w); err != nil {
		return WebhookEvent{}, errs.Wrap(err, errs.CodeInvalidInput, op)
	}
	ev := WebhookEvent{ID: id, Type: w.Type}
	if w.Type != FundsDeposited {
		return ev, nil
	}
	amount, err := strconv.ParseUint(w.Amount, 10, 64)
	if err != nil {
		return WebhookEvent{}, errs.Wrap(err, errs.CodeInvalidInput, op, slog.String("amount", w.Amount))
	}
	ev.Deposit = &Deposit{
		WalletID: w.WalletID, Recipient: chain.SolanaAddress(w.Recipient), Sender: chain.SolanaAddress(w.Sender),
		Mint: chain.SolanaAddress(w.Asset.Address), Amount: amount, Signature: chain.Signature(w.TransactionHash),
	}
	return ev, nil
}

func (c *Client) webhookReason(id, ts, signatures string, body []byte) string {
	secret, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(c.cfg.WebhookSecret, "whsec_"))
	if err != nil || len(secret) == 0 {
		return "secret"
	}
	sent, err := strconv.ParseInt(ts, 10, 64)
	if id == "" || err != nil {
		return "headers"
	}
	if skew := c.clock.Now().Sub(time.Unix(sent, 0)).Abs(); skew > webhookTolerate {
		return "timestamp"
	}
	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte(id + "." + ts + "."))
	mac.Write(body)
	want := mac.Sum(nil)
	for _, sig := range strings.Fields(signatures) {
		version, value, _ := strings.Cut(sig, ",")
		got, err := base64.StdEncoding.DecodeString(value)
		if version == "v1" && err == nil && hmac.Equal(got, want) {
			return ""
		}
	}
	return "signature"
}
