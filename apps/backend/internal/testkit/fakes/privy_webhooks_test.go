package fakes_test

import (
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/platform/chain/privy"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/config"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
	"github.com/monaco/monaco/apps/backend/internal/testkit/fakes"
)

func TestSignPrivyWebhook_VerifiesWithTheFakeSecret(t *testing.T) {
	t.Parallel()
	clk := testkit.NewClock(clock.Real{}.Now())
	c, err := privy.New(config.Config{Privy: config.Privy{
		BaseURL:         "http://privy.test",
		VerificationKey: fakes.PrivyVerificationKey(),
		WebhookSecret:   fakes.PrivyWebhookSecret,
	}, Timeouts: config.Timeouts{Privy: time.Second}}, clk)
	if err != nil {
		t.Fatal(err)
	}
	want := fakes.PrivyDeposit{Recipient: "treasury", Sender: "sender", Mint: "mint", Amount: 7, Signature: "sig"}
	body := fakes.PrivyFundsDeposited(want)
	got, err := c.VerifyWebhook(fakes.SignPrivyWebhook("msg_1", clk.Now(), body), body)
	if err != nil || got.Deposit == nil || got.Deposit.Recipient != want.Recipient || got.Deposit.Amount != 7 ||
		got.Deposit.Signature != want.Signature || got.Deposit.Sender != want.Sender || got.Deposit.Mint != want.Mint {
		t.Fatalf("VerifyWebhook = %+v, %v", got, err)
	}
}
