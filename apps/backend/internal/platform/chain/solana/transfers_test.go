package solana_test

import (
	"slices"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/solana"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

func TestInboundTransfers_readsTopLevelAndInnerTransfersIntoTheOwner(t *testing.T) {
	t.Parallel()
	c, u, _ := overFakes(t)
	got, err := c.InboundTransfers(t.Context(), deposit, member)
	if err != nil {
		t.Fatal(err)
	}
	want := []solana.Transfer{
		{Signature: deposit, From: sender, Mint: usdc(), Amount: money.NewBaseUnits(25_000_000, 6)},
		{Signature: deposit, From: sender, Mint: usdc(), Amount: money.NewBaseUnits(2_500_000, 6)},
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
	back := []solana.Transfer{{Signature: deposit, From: member, Mint: usdc(), Amount: money.NewBaseUnits(1, 6)}}
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
		`","owner":"` + string(member) + `","uiTokenAmount":{"decimals":6}},{"accountIndex":9}]},` +
		`"transaction":{"message":{"accountKeys":[{"pubkey":"dest"}],"instructions":[` +
		`{"program":"spl-token-2022","parsed":{"type":"transfer","info":{"source":"elsewhere","destination":"dest",` +
		`"authority":"` + string(sender) + `","amount":"7"}}},` +
		`{"program":"spl-token","parsed":{"type":"transferChecked","info":{"source":"dest","destination":"dest",` +
		`"tokenAmount":{"amount":"3"}}}},` +
		`{"program":"spl-token","parsed":{"type":"burn","info":{"destination":"dest"}}}]}}}`
	got, err := client(result(body)).InboundTransfers(t.Context(), deposit, member)
	want := []solana.Transfer{{Signature: deposit, From: sender, Mint: usdc(), Amount: money.NewBaseUnits(7, 6)}}
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
