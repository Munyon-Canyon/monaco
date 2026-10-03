package solana_test

import (
	"log/slog"
	"slices"
	"strings"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/solana"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

func TestInboundTransfers_readsTopLevelAndInnerTransfersIntoTheOwner(t *testing.T) {
	t.Parallel()
	c, u, _ := overFakes(t)
	got, err := c.InboundTransfersForMint(t.Context(), deposit, member, usdcMint)
	if err != nil {
		t.Fatal(err)
	}
	want := []solana.Transfer{
		classic(sender, 25_000_000),
		classic(sender, 2_500_000),
	}
	if !slices.Equal(got, want) {
		t.Fatalf("InboundTransfers = %+v, want %+v", got, want)
	}
	opts := string(u.requests()[0].params[1])
	if opts != `{"commitment":"finalized","encoding":"jsonParsed","maxSupportedTransactionVersion":0}` {
		t.Fatalf("options = %s", opts)
	}
}

func TestInboundTransfers_perspectiveOfTheOwnerAndNothingForAFailedTransaction(t *testing.T) {
	t.Parallel()
	c, _, _ := overFakes(t)
	got, err := c.InboundTransfers(t.Context(), deposit, sender)
	back := []solana.Transfer{classic(member, 1)}
	if err != nil || !slices.Equal(got, back) {
		t.Fatalf("sender view = %+v, %v; want only the 1 unit member sent back", got, err)
	}
	failed := `{"meta":{"err":{"InstructionError":[0,"Custom"]}},"transaction":{"message":{}}}`
	got, err = client(result(failed)).InboundTransfers(t.Context(), deposit, member)
	if err != nil || got != nil {
		t.Fatalf("failed tx = %+v, %v", got, err)
	}
}

func TestInboundTransfers_authorityStandsInForAnUnknownSource(t *testing.T) {
	t.Parallel()
	body := `{"meta":{"err":null,"postTokenBalances":[{"accountIndex":0,"mint":"` + string(usdcMint) +
		`","owner":"` + string(member) + `","uiTokenAmount":{"amount":"7","decimals":6}},{"accountIndex":9}]},` +
		`"transaction":{"message":{"accountKeys":[{"pubkey":"dest"}],"instructions":[` +
		`{"program":"spl-token-2022","parsed":{"type":"transfer","info":{"source":"elsewhere","destination":"dest",` +
		`"authority":"` + string(sender) + `","amount":"7"}}},` +
		`{"program":"spl-token","parsed":{"type":"transferChecked","info":{"source":"dest","destination":"dest",` +
		`"tokenAmount":{"amount":"3"}}}},` +
		`{"program":"spl-token","parsed":{"type":"burn","info":{"destination":"dest"}}}]}}}`
	got, err := client(result(body)).InboundTransfers(t.Context(), deposit, member)
	want := []solana.Transfer{classic(sender, 7)}
	if err != nil || !slices.Equal(got, want) {
		t.Fatalf("InboundTransfers = %+v, %v", got, err)
	}
}

func TestInboundTransfers_failures(t *testing.T) {
	t.Parallel()
	_, err := client(result(`null`)).InboundTransfers(t.Context(), deposit, member)
	wantCode(t, err, errs.CodeNotFound)
	bad := `{"meta":{"err":null,"postTokenBalances":[{"accountIndex":0,"owner":"` + string(member) + `"}]},` +
		`"transaction":{"message":{"accountKeys":[{"pubkey":"dest"}],"instructions":[` +
		`{"program":"spl-token","parsed":{"type":"transfer","info":{"destination":"dest","authority":"x","amount":"-1"}}}]}}}`
	_, err = client(result(bad)).InboundTransfers(t.Context(), deposit, member)
	wantCode(t, err, errs.CodeDecodeFailed)
	_, err = client(replying(503, "")).InboundTransfers(t.Context(), deposit, member)
	wantCode(t, err, errs.CodeRPCUnavailable)
	_, err = client(replying(200, "")).InboundTransfers(t.Context(), deposit, "bad")
	wantCode(t, err, errs.CodeInvalidAddress)
	_, err = client(replying(200, "")).MintConfig(t.Context(), "bad")
	wantCode(t, err, errs.CodeInvalidAddress)
}

func TestInboundTransfers_ignoresATokenBalanceIndexedExactlyOnePastTheAccountKeys(t *testing.T) {
	t.Parallel()
	body := `{"meta":{"err":null,"postTokenBalances":[{"accountIndex":0,"mint":"` + string(usdcMint) +
		`","owner":"` + string(member) + `","uiTokenAmount":{"decimals":6}},` +
		`{"accountIndex":1,"mint":"` + string(usdcMint) + `","owner":"` + string(sender) + `"},` +
		`{"accountIndex":-1,"mint":"` + string(usdcMint) + `","owner":"` + string(sender) + `"}]},` +
		`"transaction":{"message":{"accountKeys":[{"pubkey":"dest"}],"instructions":[` +
		`{"program":"spl-token","parsed":{"type":"transfer","info":{"source":"elsewhere","destination":"dest",` +
		`"authority":"` + string(sender) + `","amount":"7"}}}]}}}`
	got, err := client(result(body)).InboundTransfers(t.Context(), deposit, member)
	want := []solana.Transfer{classic(sender, 7)}
	if err != nil || !slices.Equal(got, want) {
		t.Fatalf("InboundTransfers = %+v, %v; want %+v", got, err, want)
	}
}

func classic(from chain.SolanaAddress, units uint64) solana.Transfer {
	amount := money.NewBaseUnits(units, 6)
	return solana.Transfer{
		Signature: deposit, From: from, Mint: usdc(), Amount: amount, Fee: money.NewBaseUnits(0, 6), Net: amount,
	}
}

const xStockMint = chain.SolanaAddress("XsDoVfqeBukxuZHWhdvWHBhgEHjGNst4MLodqsJHzoB")

func token2022Tx(pre, post string, ixs ...string) string {
	balance := func(amount string) string {
		return `[{"accountIndex":0,"mint":"` + string(xStockMint) + `","owner":"` + string(member) +
			`","uiTokenAmount":{"amount":"` + amount + `","decimals":8}},` +
			`{"accountIndex":1,"mint":"` + string(xStockMint) + `","owner":"` + string(sender) + `"}]`
	}
	pres := `[]`
	if pre != "" {
		pres = balance(pre)
	}
	return `{"meta":{"err":null,"preTokenBalances":` + pres + `,"postTokenBalances":` + balance(post) + `},` +
		`"transaction":{"message":{"accountKeys":[{"pubkey":"dest"},{"pubkey":"src"}],"instructions":[` +
		strings.Join(ixs, ",") + `]}}}`
}

func ix2022(typ, source, destination, amount, fee string) string {
	feeAmount := ""
	if fee != "" {
		feeAmount = `,"feeAmount":{"amount":"` + fee + `"}`
	}
	return `{"program":"spl-token-2022","parsed":{"type":"` + typ + `","info":{"source":"` + source +
		`","destination":"` + destination + `","tokenAmount":{"amount":"` + amount + `"}` + feeAmount + `}}}`
}

func xStock(gross, fee, net uint64) solana.Transfer {
	return solana.Transfer{
		Signature: deposit, From: sender, Mint: chain.Mint{Address: xStockMint, Decimals: 8},
		Amount: money.NewBaseUnits(gross, 8), Fee: money.NewBaseUnits(fee, 8), Net: money.NewBaseUnits(net, 8),
	}
}

func TestInboundTransfers_netsTheToken2022FeeOutOfWhatTheAccountReceived(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct {
		body string
		want solana.Transfer
	}{
		"feeAmount on transferCheckedWithFee": {
			token2022Tx("100", "995100", ix2022("transferCheckedWithFee", "src", "dest", "1000000", "5000")),
			xStock(1_000_000, 5_000, 995_000),
		},
		"balance change on transferChecked into a new account": {
			token2022Tx("", "990", ix2022("transferChecked", "src", "dest", "1000", "")),
			xStock(1_000, 10, 990),
		},
		"no fee on transferChecked": {
			token2022Tx("40", "1040", ix2022("transferChecked", "src", "dest", "1000", "")),
			xStock(1_000, 0, 1_000),
		},
	} {
		got, err := client(result(tc.body)).InboundTransfers(t.Context(), deposit, member)
		if err != nil || !slices.Equal(got, []solana.Transfer{tc.want}) {
			t.Fatalf("%s: InboundTransfers = %+v, %v; want %+v", name, got, err, tc.want)
		}
	}
}

func TestInboundTransfers_refusesAToken2022FeeItCannotAttributeOrThatDoesNotAddUp(t *testing.T) {
	t.Parallel()
	in := func(amount string) string { return ix2022("transferChecked", "src", "dest", amount, "") }
	for name, body := range map[string]string{
		"two transfers into one account": token2022Tx("", "1000", in("1000"), in("1000")),
		"a transfer out of the same account": token2022Tx("500", "1300", in("1000"),
			ix2022("transferChecked", "dest", "elsewhere", "200", "")),
		"bad feeAmount":        token2022Tx("", "990", ix2022("transferCheckedWithFee", "src", "dest", "1000", "x")),
		"fee above the amount": token2022Tx("", "0", ix2022("transferCheckedWithFee", "src", "dest", "1000", "1001")),
		"bad pre balance":      token2022Tx("x", "990", in("1000")),
		"bad post balance":     token2022Tx("", "x", in("1000")),
		"post below pre":       token2022Tx("990", "500", in("1000")),
		"received above sent":  token2022Tx("", "1001", in("1000")),
	} {
		got, err := client(result(body)).InboundTransfers(t.Context(), deposit, member)
		named := slices.ContainsFunc(errs.Detail(err), slog.String("signature", string(deposit)).Equal)
		if got != nil || errs.CodeOf(err) != errs.CodeDecodeFailed || !named {
			t.Fatalf("%s: InboundTransfers = %+v, %v; want decode_failed naming the signature", name, got, err)
		}
	}
}
