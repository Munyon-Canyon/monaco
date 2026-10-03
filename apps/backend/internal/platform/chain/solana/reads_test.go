package solana_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/solana"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

func TestSOLBalance_readsLamportsAsNineDecimalBaseUnits(t *testing.T) {
	t.Parallel()
	c, u, _ := overFakes(t)
	got, err := c.SOLBalance(t.Context(), member)
	if err != nil || got != money.NewBaseUnits(50_000_000, 9) {
		t.Fatalf("SOLBalance = %v, %v", got, err)
	}
	req := u.requests()[0]
	if req.method != "getBalance" || req.path != "/rpc/" || req.query != "api-key="+hiddenPart {
		t.Fatalf("request = %+v, want getBalance at the configured URL", req)
	}
}

func TestTokenBalance_sumsEveryAccountForTheMint(t *testing.T) {
	t.Parallel()
	c, u, _ := overFakes(t)
	got, err := c.TokenBalance(t.Context(), member, usdc())
	if err != nil || got != money.NewBaseUnits(25_500_000, 6) {
		t.Fatalf("TokenBalance = %v, %v", got, err)
	}
	if p := string(u.requests()[0].params[1]); p != `{"mint":"`+string(usdcMint)+`"}` {
		t.Fatalf("filter = %s", p)
	}
}

func TestTokenBalance_rejectsBadAmounts(t *testing.T) {
	t.Parallel()
	account := func(amount string, decimals string) string {
		return `{"account":{"data":{"parsed":{"info":{"tokenAmount":{"amount":"` + amount + `","decimals":` + decimals + `}}}}}}`
	}
	for name, body := range map[string]string{
		"not a number":     `{"value":[` + account("x", "6") + `]}`,
		"decimals differ":  `{"value":[` + account("1", "9") + `]}`,
		"overflow the sum": `{"value":[` + account("18446744073709551615", "6") + `,` + account("1", "6") + `]}`,
	} {
		got, err := client(result(body)).TokenBalance(t.Context(), member, usdc())
		if errs.CodeOf(err) != errs.CodeDecodeFailed || !got.IsZero() {
			t.Fatalf("%s: TokenBalance = %v, %v, want decode_failed", name, got, err)
		}
	}
	_, err := client(replying(503, "")).TokenBalance(t.Context(), member, usdc())
	wantCode(t, err, errs.CodeRPCUnavailable)
}

func TestSignatureStatuses_mapsFinalizedProcessingAndNotFound(t *testing.T) {
	t.Parallel()
	c, u, _ := overFakes(t)
	sigs := []chain.Signature{"a", "b", "c", "d"}
	got, err := c.SignatureStatuses(t.Context(), sigs)
	if err != nil {
		t.Fatal(err)
	}
	const height = 380_000_000
	want := []solana.Status{
		{Signature: "a", State: solana.StateFinalized, BlockHeight: height},
		{Signature: "b", State: solana.StateProcessing, BlockHeight: height},
		{Signature: "c", State: solana.StateNotFound, BlockHeight: height},
		{Signature: "d", State: solana.StateFinalized, Failed: true, BlockHeight: height},
	}
	if !slices.Equal(got, want) {
		t.Fatalf("SignatureStatuses = %+v, want %+v", got, want)
	}
	if m := u.methods(); !slices.Equal(m, []string{"getBlockHeight", "getSignatureStatuses"}) {
		t.Fatalf("calls = %v, want the finalized height read before the statuses", m)
	}
}

func TestStatus_blockhashExpiresOnlyWhenNotFoundPastTheLastValidHeight(t *testing.T) {
	t.Parallel()
	missing := solana.Status{State: solana.StateNotFound, BlockHeight: 101}
	if !missing.BlockhashExpired(100) || missing.BlockhashExpired(101) {
		t.Fatal("a missing signature expires once the finalized height passes lastValidBlockHeight")
	}
	if (solana.Status{State: solana.StateProcessing, BlockHeight: 500}).BlockhashExpired(100) {
		t.Fatal("a seen signature never expires")
	}
}

func TestSignatureStatuses_edges(t *testing.T) {
	t.Parallel()
	c, u, _ := overFakes(t)
	if got, err := c.SignatureStatuses(t.Context(), nil); got != nil || err != nil || len(u.requests()) != 0 {
		t.Fatalf("no signatures = %v, %v with %d calls", got, err, len(u.requests()))
	}
	_, err := c.SignatureStatuses(t.Context(), make([]chain.Signature, 257))
	wantCode(t, err, errs.CodeInvalidInput)
	_, err = c.SignatureStatuses(t.Context(), []chain.Signature{"only-one"})
	wantCode(t, err, errs.CodeDecodeFailed)
	_, err = client(replying(503, "")).SignatureStatuses(t.Context(), []chain.Signature{"a"})
	wantCode(t, err, errs.CodeRPCUnavailable)
	_, err = client(result(`7`)).SignatureStatuses(t.Context(), []chain.Signature{"a"})
	wantCode(t, err, errs.CodeDecodeFailed)
}

func TestSignaturesFor_pagesBackFromBefore(t *testing.T) {
	t.Parallel()
	c, u, _ := overFakes(t)
	got, err := c.SignaturesFor(t.Context(), member, olderSig, 50)
	want := []solana.SignatureInfo{
		{Signature: deposit, Slot: 450_999_500},
		{Signature: olderSig, Slot: 450_999_400, Failed: true},
	}
	if err != nil || !slices.Equal(got, want) {
		t.Fatalf("SignaturesFor = %+v, %v", got, err)
	}
	if p := string(
		u.requests()[0].params[1],
	); p != `{"before":"`+string(
		olderSig,
	)+`","commitment":"finalized","limit":50}` {
		t.Fatalf("options = %s", p)
	}
	if _, err := c.SignaturesFor(t.Context(), member, "", 10); err != nil {
		t.Fatal(err)
	}
	if p := string(u.requests()[1].params[1]); p != `{"commitment":"finalized","limit":10}` {
		t.Fatalf("options without before = %s", p)
	}
	_, err = client(replying(503, "")).SignaturesFor(t.Context(), member, "", 1)
	wantCode(t, err, errs.CodeRPCUnavailable)
}

func TestSignaturesFor_acceptsOneToAThousandAndRefusesOtherLimitsWithoutACall(t *testing.T) {
	t.Parallel()
	c, u, _ := overFakes(t)
	for _, limit := range []int{0, -1, 1001} {
		_, err := c.SignaturesFor(t.Context(), member, "", limit)
		wantCode(t, err, errs.CodeInvalidInput)
	}
	if n := len(u.requests()); n != 0 {
		t.Fatalf("%d RPC calls for out-of-range limits", n)
	}
	for _, limit := range []int{1, 1000} {
		if _, err := c.SignaturesFor(t.Context(), member, "", limit); err != nil {
			t.Fatalf("limit %d: %v", limit, err)
		}
	}
	if n := len(u.requests()); n != 2 {
		t.Fatalf("%d RPC calls for limits 1 and 1000, want 2", n)
	}
}

func TestReads_refuseInvalidAddressesWithoutACall(t *testing.T) {
	t.Parallel()
	c, u, _ := overFakes(t)
	ctx := t.Context()
	_, err := c.SOLBalance(ctx, "not-an-address")
	wantCode(t, err, errs.CodeInvalidAddress)
	_, err = c.TokenBalance(ctx, member, chain.Mint{Address: "bad"})
	wantCode(t, err, errs.CodeInvalidAddress)
	_, err = c.SignaturesFor(ctx, "bad", "", 1)
	wantCode(t, err, errs.CodeInvalidAddress)
	if n := len(u.requests()); n != 0 {
		t.Fatalf("%d RPC calls for invalid addresses", n)
	}
}

func TestSignatureStatuses_acceptsExactlyTheMostItAsksForAndRefusesOneMore(t *testing.T) {
	t.Parallel()
	const most = 256
	body := func(method string) string {
		if method == "getBlockHeight" {
			return `100`
		}
		return `{"value":[` + strings.TrimSuffix(strings.Repeat("null,", most), ",") + `]}`
	}
	got, err := client(byMethod(body)).SignatureStatuses(t.Context(), make([]chain.Signature, most))
	if err != nil || len(got) != most || got[most-1].State != solana.StateNotFound {
		t.Fatalf("%d signatures = %d statuses, %v; want one not-found status each", most, len(got), err)
	}
	_, err = client(byMethod(body)).SignatureStatuses(t.Context(), make([]chain.Signature, most+1))
	wantCode(t, err, errs.CodeInvalidInput)
}
