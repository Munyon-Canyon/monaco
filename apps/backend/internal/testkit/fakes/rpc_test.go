package fakes_test

import (
	"net/http"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/solana"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/config"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpclient"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/testkit/fakes"
)

const (
	usdcMint  = chain.SolanaAddress("EPjFWdd5AufqSSqeM2qN1xzybapC8G4wEGGkZwyTDt1v")
	stockMint = chain.SolanaAddress("XsDoVfqeBukxuZHWhdvWHBhgEHjGNst4MLodqsJHzoB")
	funded    = chain.SolanaAddress("F4nbnZw67wKUQGW46MSRwN23VAKQRhYUMEXt88GQwD9z")
	unset     = chain.SolanaAddress("9ixcyg5nNxGCJLtSyJYibP7EgQBw4BfpNLbDe7GQ14eh")
)

func TestSetBalance_ReadsBackThroughTokenBalance(t *testing.T) {
	t.Parallel()
	transport := inProcess{fakes.New()}
	control := httpclient.New("fakes", httpclient.WithBaseURL("http://fakes.test"),
		httpclient.WithTimeout(time.Minute), httpclient.WithTransport(transport))
	rpc := solana.New(config.Config{
		Solana:   config.Solana{RPCURL: "http://fakes.test/rpc"},
		Timeouts: config.Timeouts{RPC: 5 * time.Second},
	}, clock.Real{}, httpclient.WithTransport(transport))
	usdc := chain.Mint{Address: usdcMint, Decimals: 6}
	stock := chain.Mint{Address: stockMint, Decimals: 8}
	balance := func(owner chain.SolanaAddress, mint chain.Mint) money.BaseUnits {
		t.Helper()
		got, err := rpc.TokenBalance(t.Context(), owner, mint)
		if err != nil {
			t.Fatalf("TokenBalance(%s, %s): %v", owner, mint.Address, err)
		}
		return got
	}

	if got := balance(unset, usdc); got != money.NewBaseUnits(25_500_000, 6) {
		t.Fatalf("unset owner balance = %v, want the recorded fixture's 25.5 USDC", got)
	}
	set := `{"owner":"` + string(funded) + `","mint":"` + string(usdcMint) + `","amount":"25000000","decimals":6}`
	if got := mustCall(t.Context(), t, control, http.MethodPost, "/_balance", set); got.status != http.StatusNoContent {
		t.Fatalf("POST /_balance = %d %q, want 204", got.status, got.body)
	}
	if got := balance(funded, usdc); got != money.NewBaseUnits(25_000_000, 6) {
		t.Fatalf("funded USDC balance = %v, want 25 USDC", got)
	}
	if got := balance(funded, stock); got != money.NewBaseUnits(0, 8) {
		t.Fatalf("funded stock balance = %v, want 0: only USDC was set", got)
	}
	zero := `{"owner":"` + string(funded) + `","mint":"` + string(usdcMint) + `","amount":"0","decimals":6}`
	mustCall(t.Context(), t, control, http.MethodPost, "/_balance", zero)
	if got := balance(funded, usdc); got != money.NewBaseUnits(0, 6) {
		t.Fatalf("funded USDC balance after reset = %v, want 0", got)
	}
}

func TestSetBalance_RejectsBadInput(t *testing.T) {
	t.Parallel()
	for name, body := range map[string]string{
		"owner":   `{"owner":"not-base58!","mint":"` + string(usdcMint) + `","amount":"1","decimals":6}`,
		"mint":    `{"owner":"` + string(funded) + `","mint":"","amount":"1","decimals":6}`,
		"amount":  `{"owner":"` + string(funded) + `","mint":"` + string(usdcMint) + `","amount":"-1","decimals":6}`,
		"unknown": `{"owner":"` + string(funded) + `","mint":"` + string(usdcMint) + `","usdc":"1"}`,
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			got := mustCall(t.Context(), t, inProc(), http.MethodPost, "/_balance", body)
			if got.status != http.StatusBadRequest {
				t.Fatalf("POST /_balance %s = %d %q, want 400", body, got.status, got.body)
			}
		})
	}
}

func statusClient(srv http.Handler) *solana.Client {
	return solana.New(config.Config{
		Solana:   config.Solana{RPCURL: "http://fakes.test/rpc"},
		Timeouts: config.Timeouts{RPC: 5 * time.Second},
	}, clock.Real{}, httpclient.WithTransport(inProcess{srv}))
}

func TestSignatureStatuses_OneFinalizedStatusPerSignatureInOrder(t *testing.T) {
	t.Parallel()
	rpc := statusClient(fakes.New())
	sigs := []chain.Signature{
		"5VERv8NMvzbJMEkV8xnrLkEaWRtSz9CosKDYjCJjBRnbJLgp8uirBgmQpjKhoR4tjF3ZpRzrFmBV6UjKdiSZkQUW",
		"4hXTCkRzt9WyecNzV1XPgCDfGAZzQKNxLXgynz5QDuWWPSAZBZSHptvWRL3BjCvzUXRdKvHL2b81mGVnHSDpXaMV",
		"3nN7WkPoNY6S9dPzJb8Vj6Jgd9hFYMcvi4M6uBTaFXJWo2TjbhcJHB7rRfmTCKcGgS3bDzKrDHTcqnFQxyzKa5Np",
	}
	for n := 1; n <= len(sigs); n++ {
		statuses, err := rpc.SignatureStatuses(t.Context(), sigs[:n])
		if err != nil || len(statuses) != n {
			t.Fatalf("SignatureStatuses(%d sigs) = %+v, %v; want %d statuses", n, statuses, err, n)
		}
		for i, st := range statuses {
			if st.State != solana.StateFinalized || st.Failed {
				t.Fatalf("SignatureStatuses(%d sigs)[%d] = %+v; want finalized without error", n, i, st)
			}
		}
	}
}

func TestSignatureStatuses_AMalformedSignatureIsNotFound(t *testing.T) {
	t.Parallel()
	statuses, err := statusClient(fakes.New()).SignatureStatuses(t.Context(), []chain.Signature{"sig-seeded"})
	if err != nil || len(statuses) != 1 || statuses[0].State != solana.StateNotFound {
		t.Fatalf(
			"SignatureStatuses(sig-seeded) = %+v, %v; want not found: no chain lands a malformed signature",
			statuses,
			err,
		)
	}
}

func TestSignatureStatuses_ARecordedFailureKeepsItsError(t *testing.T) {
	t.Parallel()
	failed := chain.Signature(
		"5VERv8NMvzbJMEkV8xnrLkEaWRtSz9CosKDYjCJjBRnbJLgp8uirBgmQpjKhoR4tjF3ZpRzrFmBV6UjKdiSZkQUW",
	)
	other := chain.Signature("4hXTCkRzt9WyecNzV1XPgCDfGAZzQKNxLXgynz5QDuWWPSAZBZSHptvWRL3BjCvzUXRdKvHL2b81mGVnHSDpXaMV")
	rpc := statusClient(fakes.NewFrom(fstest.MapFS{
		"fx/rpc/getBlockHeight.json": {Data: []byte(`{"status":200,"body":{"jsonrpc":"2.0","id":1,"result":5}}`)},
		"fx/rpc/getSignatureStatuses/" + string(failed) + ".json": {
			Data: []byte(`{"status":200,"body":{"jsonrpc":"2.0","id":1,` +
				`"result":{"context":{"slot":7},"value":[{"slot":6,"confirmations":null,` +
				`"err":{"InstructionError":[0,"Custom"]},"confirmationStatus":"finalized"}]}}}`),
		},
	}, "fx"))
	statuses, err := rpc.SignatureStatuses(t.Context(), []chain.Signature{other, failed})
	if err != nil || len(statuses) != 2 || statuses[0].Failed || !statuses[1].Failed {
		t.Fatalf(
			"SignatureStatuses = %+v, %v; want the recorded signature failed and the other finalized",
			statuses,
			err,
		)
	}
}

func TestRPC_malformedParamsAreRefused(t *testing.T) {
	t.Parallel()
	control := inProc()
	for _, method := range []string{"getSignatureStatuses", "isBlockhashValid", "getTransaction"} {
		got := mustCall(t.Context(), t, control, http.MethodPost, "/rpc",
			`{"jsonrpc":"2.0","id":1,"method":"`+method+`","params":[{}]}`)
		if !strings.Contains(got.body, "-32602") {
			t.Fatalf("%s with object params = %d %q, want the invalid params fault", method, got.status, got.body)
		}
	}
}

func TestSignatureStatuses_ARecordedFixtureWithoutOneStatusIsIgnored(t *testing.T) {
	t.Parallel()
	sig := chain.Signature("5VERv8NMvzbJMEkV8xnrLkEaWRtSz9CosKDYjCJjBRnbJLgp8uirBgmQpjKhoR4tjF3ZpRzrFmBV6UjKdiSZkQUW")
	rpc := statusClient(fakes.NewFrom(fstest.MapFS{
		"fx/rpc/getBlockHeight.json": {Data: []byte(`{"status":200,"body":{"jsonrpc":"2.0","id":1,"result":5}}`)},
		"fx/rpc/getSignatureStatuses/" + string(sig) + ".json": {Data: []byte(`{"status":200,"body":{"jsonrpc":"2.0",` +
			`"id":1,"result":{"context":{"slot":7},"value":[null,null]}}}`)},
	}, "fx"))
	statuses, err := rpc.SignatureStatuses(t.Context(), []chain.Signature{sig})
	if err != nil || len(statuses) != 1 || statuses[0].State != solana.StateFinalized {
		t.Fatalf("SignatureStatuses = %+v, %v; want finalized, since the fixture names no single status", statuses, err)
	}
}
