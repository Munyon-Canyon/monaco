package fakes_test

import (
	"crypto/ed25519"
	"net/http"
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/jupiter"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/solana"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/config"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpclient"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/testkit/fakes"
)

type swapHarness struct {
	t       *testing.T
	jup     *jupiter.Client
	rpc     *solana.Client
	control *httpclient.Client
}

func newSwapHarness(t *testing.T) swapHarness {
	t.Helper()
	transport := inProcess{fakes.New()}
	return swapHarness{
		t: t,
		jup: jupiter.New(config.Config{
			Jupiter: config.Jupiter{
				SwapBaseURL: "http://fakes.test/jupiter/swap/v2", PriceBaseURL: "http://fakes.test/jupiter/price/v3",
			},
			Timeouts: config.Timeouts{
				JupiterQuote: 5 * time.Second, JupiterExecute: 5 * time.Second,
			},
		}, clock.Real{}, httpclient.WithTransport(transport)),
		rpc: solana.New(config.Config{
			Solana:   config.Solana{RPCURL: "http://fakes.test/rpc"},
			Timeouts: config.Timeouts{RPC: 5 * time.Second},
		}, clock.Real{}, httpclient.WithTransport(transport)),
		control: httpclient.New("fakes", httpclient.WithBaseURL("http://fakes.test"),
			httpclient.WithTimeout(time.Minute), httpclient.WithTransport(transport)),
	}
}

func (h swapHarness) post(path, body string) {
	h.t.Helper()
	if got := mustCall(h.t.Context(), h.t, h.control, http.MethodPost, path, body); got.status != http.StatusNoContent {
		h.t.Fatalf("POST %s %s = %d %q, want 204", path, body, got.status, got.body)
	}
}

func (h swapHarness) order(taker chain.SolanaAddress, amount uint64) jupiter.Order {
	h.t.Helper()
	got, err := h.jup.Order(h.t.Context(), jupiter.OrderSpec{
		Taker: jupiter.SolanaAddress(taker), In: jupiter.Mint{Address: string(usdcMint), Decimals: 6},
		Out:    jupiter.Mint{Address: "XsbEhLAtcf6HdfpFZ5xEMdqW8nfAvcsP5bdudRLJzJp", Decimals: 8},
		Amount: money.NewBaseUnits(amount, 6), SlippageBps: 100,
	})
	if err != nil {
		h.t.Fatalf("Order(%s): %v", taker, err)
	}
	return got
}

func signAs(t *testing.T, walletID string, unsigned []byte) ([]byte, chain.Signature) {
	t.Helper()
	tx, err := chain.DecodeTransaction(unsigned)
	if err != nil {
		t.Fatalf("decode the order transaction: %v", err)
	}
	if err := tx.Sign(fakes.PrivyWalletKey(walletID)); err != nil {
		t.Fatalf("sign as %s: %v", walletID, err)
	}
	return tx.Encode(), chain.SignatureOf(tx.Signatures[0])
}

func TestJupiter_anOrderForAHeldWalletExecutesUnderItsOwnSignature(t *testing.T) {
	t.Parallel()
	h := newSwapHarness(t)
	h.post("/_wallet", `{"id":"treasury-a"}`)
	taker := fakes.PrivyWalletAddress("treasury-a")

	first := h.order(taker, 50_000_000)
	if first.InAmount.Uint64() != 50_000_000 || first.OutAmount.Uint64() != 22_000_000 {
		t.Fatalf("order amounts = %v in, %v out, want 50 USDC in at the fixture's 11/25 rate", first.InAmount,
			first.OutAmount)
	}
	signed, signature := signAs(t, "treasury-a", first.Transaction)
	got, err := h.jup.Execute(t.Context(), first.RequestID, signed)
	want := jupiter.ExecuteResult{
		Status: jupiter.StatusSuccess, Signature: string(signature), InAmount: 50_000_000, OutAmount: 22_000_000,
	}
	if err != nil || got != want {
		t.Fatalf("Execute = %+v, %v, want %+v", got, err, want)
	}

	second := h.order(taker, 50_000_000)
	if second.RequestID == first.RequestID || string(second.Transaction) == string(first.Transaction) {
		t.Fatalf("a second order reused request %s or its transaction: a retry needs a fresh signature",
			first.RequestID)
	}
}

func TestJupiter_aScriptedOwnerFailsOrPendsWithoutTouchingOthers(t *testing.T) {
	t.Parallel()
	h := newSwapHarness(t)
	for _, id := range []string{"failing", "pending", "fine"} {
		h.post("/_wallet", `{"id":"`+id+`"}`)
	}
	h.post("/_swap", `{"owner":"`+string(fakes.PrivyWalletAddress("failing"))+`","status":"Failed","code":6001}`)
	h.post("/_swap", `{"owner":"`+string(fakes.PrivyWalletAddress("pending"))+`","status":"Pending"}`)

	for id, want := range map[string]jupiter.Status{
		"failing": jupiter.StatusFailed, "pending": jupiter.StatusPending, "fine": jupiter.StatusSuccess,
	} {
		o := h.order(fakes.PrivyWalletAddress(id), 25_000_000)
		signed, _ := signAs(t, id, o.Transaction)
		got, err := h.jup.Execute(t.Context(), o.RequestID, signed)
		if err != nil || got.Status != want {
			t.Fatalf("Execute for %s = %+v, %v, want status %v", id, got, err, want)
		}
		if id == "failing" && got.ErrorCode != 6001 {
			t.Fatalf("failed execute code = %d, want the scripted 6001", got.ErrorCode)
		}
	}
}

func TestJupiter_anExecuteNotSignedByTheTakerFails(t *testing.T) {
	t.Parallel()
	h := newSwapHarness(t)
	h.post("/_wallet", `{"id":"taker"}`)
	o := h.order(fakes.PrivyWalletAddress("taker"), 25_000_000)
	got, err := h.jup.Execute(t.Context(), o.RequestID, o.Transaction)
	if err != nil || got.Status != jupiter.StatusFailed {
		t.Fatalf("Execute of the unsigned order = %+v, %v, want Failed", got, err)
	}
}

func TestJupiter_anUnknownTakerStillGetsTheRecordedOrder(t *testing.T) {
	t.Parallel()
	h := newSwapHarness(t)
	got := h.order(funded, 25_000_000)
	if got.RequestID != "req-buy-aaplx-1" || string(got.Transaction) != "unsigned-buy-tx" {
		t.Fatalf("order for a taker with no fake wallet = %+v, want the recorded fixture", got)
	}
}

func TestSetSwapAndWallet_RejectBadInput(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ path, body string }{
		{"/_swap", `{"owner":"not-base58!","status":"Failed"}`},
		{"/_swap", `{"owner":"` + string(funded) + `","status":"Lost"}`},
		{"/_swap", `{"owner":"` + string(funded) + `","status":"Failed","extra":1}`},
		{"/_wallet", `{"id":""}`},
		{"/_wallet", `{"wallet":"x"}`},
	} {
		got := mustCall(t.Context(), t, inProc(), http.MethodPost, tc.path, tc.body)
		if got.status != http.StatusBadRequest {
			t.Fatalf("POST %s %s = %d %q, want 400", tc.path, tc.body, got.status, got.body)
		}
	}
}

func TestJupiter_anOrderWithAPayerExecutesOnlyWhenThePayerCoSigns(t *testing.T) {
	t.Parallel()
	h := newSwapHarness(t)
	h.post("/_wallet", `{"id":"treasury-p"}`)
	taker := fakes.PrivyWalletAddress("treasury-p")
	payerKey := fakes.FixtureKey("verify-relayer")
	payer := chain.AddressOf(payerKey.Public().(ed25519.PublicKey))
	order, err := h.jup.Order(t.Context(), jupiter.OrderSpec{
		Taker: jupiter.SolanaAddress(taker), Payer: jupiter.SolanaAddress(payer),
		In:     jupiter.Mint{Address: string(usdcMint), Decimals: 6},
		Out:    jupiter.Mint{Address: "XsbEhLAtcf6HdfpFZ5xEMdqW8nfAvcsP5bdudRLJzJp", Decimals: 8},
		Amount: money.NewBaseUnits(25_000_000, 6), SlippageBps: 100,
	})
	if err != nil {
		t.Fatal(err)
	}
	walletOnly, _ := signAs(t, "treasury-p", order.Transaction)
	tx, _ := chain.DecodeTransaction(walletOnly)
	if len(tx.Signers) != 2 || tx.Signers[0] != payer || tx.Signers[1] != taker {
		t.Fatalf("order signers = %v, want the payer first and the treasury second", tx.Signers)
	}
	if got, err := h.jup.Execute(
		t.Context(),
		order.RequestID,
		walletOnly,
	); err != nil ||
		got.Status == jupiter.StatusSuccess {
		t.Fatalf("Execute(treasury signature only) = %+v, %v; want a failed swap", got, err)
	}
	if err := tx.Sign(payerKey); err != nil {
		t.Fatal(err)
	}
	got, err := h.jup.Execute(t.Context(), order.RequestID, tx.Encode())
	if err != nil || got.Status != jupiter.StatusSuccess ||
		got.Signature != string(chain.SignatureOf(tx.Signatures[0])) {
		t.Fatalf("Execute(co-signed) = %+v, %v; want success under the payer's signature", got, err)
	}
}

func TestJupiter_anExecutedSwapLandsOnChainAndAnUnexecutedOneExpires(t *testing.T) {
	t.Parallel()
	h := newSwapHarness(t)
	h.post("/_wallet", `{"id":"treasury-b"}`)
	taker := fakes.PrivyWalletAddress("treasury-b")
	executed, abandoned := h.order(taker, 25_000_000), h.order(taker, 25_000_000)
	signed, landed := signAs(t, "treasury-b", executed.Transaction)
	if _, err := h.jup.Execute(t.Context(), executed.RequestID, signed); err != nil {
		t.Fatal(err)
	}
	_, unsent := signAs(t, "treasury-b", abandoned.Transaction)

	statuses, err := h.rpc.SignatureStatuses(t.Context(), []chain.Signature{landed, unsent})
	if err != nil || statuses[0].State != solana.StateFinalized || statuses[0].Failed ||
		statuses[1].State != solana.StateNotFound {
		t.Fatalf("SignatureStatuses = %+v, %v; want the executed swap finalized and the other not found", statuses, err)
	}
	transfers, err := h.rpc.InboundTransfersForMint(t.Context(), landed, taker,
		"XsbEhLAtcf6HdfpFZ5xEMdqW8nfAvcsP5bdudRLJzJp")
	if err != nil || len(transfers) != 1 || transfers[0].Net != money.NewBaseUnits(11_000_000, 8) {
		t.Fatalf("InboundTransfersForMint = %+v, %v; want 11000000 AAPLx base units at 8 decimals", transfers, err)
	}
}

func TestJupiter_anOrderBlockhashHasExpiredAndAnyOtherIsValid(t *testing.T) {
	t.Parallel()
	h := newSwapHarness(t)
	h.post("/_wallet", `{"id":"treasury-c"}`)
	taker := fakes.PrivyWalletAddress("treasury-c")
	hash, _ := chain.RecentBlockhash(h.order(taker, 25_000_000).Transaction)
	if valid, err := h.rpc.BlockhashValid(t.Context(), hash); err != nil || valid {
		t.Fatalf("BlockhashValid(order blockhash) = %v, %v; want false so the sweeper expires it", valid, err)
	}
	if valid, err := h.rpc.BlockhashValid(t.Context(), string(taker)); err != nil || !valid {
		t.Fatalf("BlockhashValid(unknown) = %v, %v; want true", valid, err)
	}
}

func TestJupiter_aPayerOrderNeedsAValidPayerAndAnyValidPayerCanPay(t *testing.T) {
	t.Parallel()
	h := newSwapHarness(t)
	h.post("/_wallet", `{"id":"treasury-q"}`)
	taker := string(fakes.PrivyWalletAddress("treasury-q"))
	query := "?inputMint=" + string(usdcMint) + "&outputMint=XsbEhLAtcf6HdfpFZ5xEMdqW8nfAvcsP5bdudRLJzJp" +
		"&amount=25000000&slippageBps=100&taker=" + taker
	if got := mustCall(
		t.Context(),
		t,
		h.control,
		http.MethodGet,
		"/jupiter/swap/v2/order"+query+"&payer=not-an-address",
		"",
	); got.status != http.StatusBadRequest {
		t.Fatalf("order with a bad payer = %d %q, want 400", got.status, got.body)
	}
	stranger := chain.AddressOf(fakes.FixtureKey("unknown-payer").Public().(ed25519.PublicKey))
	if got := mustCall(
		t.Context(),
		t,
		h.control,
		http.MethodGet,
		"/jupiter/swap/v2/order"+query+"&payer="+string(stranger),
		"",
	); got.status != http.StatusOK {
		t.Fatalf("order with a payer the fakes hold no key for = %d %q, want 200", got.status, got.body)
	}
}
