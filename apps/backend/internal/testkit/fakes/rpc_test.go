package fakes_test

import (
	"net/http"
	"testing"
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
