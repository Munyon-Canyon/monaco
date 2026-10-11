package solana_test

import (
	"net/http"
	"os"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
)

const (
	sellTreasury = chain.SolanaAddress("6HUUQBkqjMRGWuqnp1t6wB8iHRKhY15pcTAq47ZX8ELx")
	sellSig      = chain.Signature(
		"2ZbDT86oYj6ZPHLnSK4fjziosuDxbjh9mhbXy6wLo4nMyGB1rAei1xWAA6WdRsG8VTqaW9eaoHXxcZq6HpyoGSob",
	)
)

func jupiterSell(t *testing.T) string {
	t.Helper()
	raw, err := os.ReadFile("testdata/jupiter_sell_fee_leg.json")
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func TestReceived_aJupiterSellNetsTheFeeLegTheTreasuryPaysBack(t *testing.T) {
	t.Parallel()
	u := result(jupiterSell(t))
	got, err := client(u).Received(t.Context(), sellSig, sellTreasury, usdc())
	if err != nil {
		t.Fatal(err)
	}
	if got.Uint64() != 4_880_982 || got.Decimals() != 6 {
		t.Fatalf("Received = %s (%d decimals), want 4880982, the chain's USDC delta", got, got.Decimals())
	}
	opts := string(u.requests()[0].params[1])
	if opts != `{"commitment":"confirmed","encoding":"jsonParsed","maxSupportedTransactionVersion":0}` {
		t.Fatalf("options = %s", opts)
	}
	inbound, err := client(result(jupiterSell(t))).InboundTransfersForMint(t.Context(), sellSig, sellTreasury, usdcMint)
	if err != nil || len(inbound) != 1 || inbound[0].Net.Uint64() != 4_885_867 {
		t.Fatalf("inbound = %+v, %v; the inbound leg alone is Jupiter's pre-fee 4885867", inbound, err)
	}
}

func TestReceived_aBuyCountsWhatLandedInTheTakersAccount(t *testing.T) {
	t.Parallel()
	body := `{"meta":{"err":null,` +
		`"preTokenBalances":[{"accountIndex":1,"mint":"` + string(xStockMint) + `","owner":"` + string(member) +
		`","uiTokenAmount":{"amount":"5","decimals":8}}],` +
		`"postTokenBalances":[{"accountIndex":1,"mint":"` + string(xStockMint) + `","owner":"` + string(member) +
		`","uiTokenAmount":{"amount":"105000005","decimals":8}},` +
		`{"accountIndex":2,"mint":"` + string(xStockMint) + `","owner":"` + string(sender) +
		`","uiTokenAmount":{"amount":"9","decimals":8}}]},"transaction":{"message":{}}}`
	got, err := client(
		result(body),
	).Received(t.Context(), sellSig, member, chain.Mint{Address: xStockMint, Decimals: 8})
	if err != nil || got.Uint64() != 105_000_000 {
		t.Fatalf("Received = %s, %v; want 105000000", got, err)
	}
}

func TestReceived_failures(t *testing.T) {
	t.Parallel()
	balance := func(amount string, decimals string) string {
		return `{"accountIndex":0,"mint":"` + string(usdcMint) + `","owner":"` + string(member) +
			`","uiTokenAmount":{"amount":"` + amount + `","decimals":` + decimals + `}}`
	}
	tx := func(pre, post string) string {
		return `{"meta":{"err":null,"preTokenBalances":[` + pre + `],"postTokenBalances":[` + post +
			`]},"transaction":{"message":{}}}`
	}
	for name, tc := range map[string]struct {
		body string
		code errs.Code
	}{
		"not visible yet":           {body: `null`, code: errs.CodeNotFound},
		"no meta":                   {body: `{"transaction":{"message":{}}}`, code: errs.CodeNotFound},
		"the balance fell":          {body: tx(balance("9", "6"), balance("8", "6")), code: errs.CodeDecodeFailed},
		"another decimals":          {body: tx(balance("1", "6"), balance("9", "8")), code: errs.CodeDecodeFailed},
		"an unreadable count":       {body: tx(balance("1", "6"), balance("x", "6")), code: errs.CodeDecodeFailed},
		"an unreadable pre balance": {body: tx(balance("1", "8"), balance("9", "6")), code: errs.CodeDecodeFailed},
		"balances past uint64": {
			body: tx(balance("1", "6"), balance("18446744073709551615", "6")+","+balance("1", "6")),
			code: errs.CodeDecodeFailed,
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			_, err := client(result(tc.body)).Received(t.Context(), sellSig, member, usdc())
			wantCode(t, err, tc.code)
		})
	}
	failed := `{"meta":{"err":{"InstructionError":[0,"Custom"]},"postTokenBalances":[` + balance("9", "6") +
		`]},"transaction":{"message":{}}}`
	got, err := client(result(failed)).Received(t.Context(), sellSig, member, usdc())
	if err != nil || !got.IsZero() {
		t.Fatalf("failed tx = %s, %v; want nothing received", got, err)
	}
	_, err = client(result(`null`)).Received(t.Context(), sellSig, "not-an-address", usdc())
	wantCode(t, err, errs.CodeInvalidAddress)
	_, err = client(replying(http.StatusBadGateway, "")).Received(t.Context(), sellSig, member, usdc())
	wantCode(t, err, errs.CodeRPCUnavailable)
}
