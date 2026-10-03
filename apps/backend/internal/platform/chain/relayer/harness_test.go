package relayer_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/privy"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/relayer"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/solana"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/config"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpclient"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/testkit/fakes"
)

const (
	memberWallet = chain.SolanaAddress("Dht9c9YfstFWkNYXgqr8HZbhqVn563bCpNU6zL32Ftqf")
	treasury     = chain.SolanaAddress("9ixcyg5nNxGCJLtSyJYibP7EgQBw4BfpNLbDe7GQ14eh")
	usdcMint     = chain.SolanaAddress("EPjFWdd5AufqSSqeM2qN1xzybapC8G4wEGGkZwyTDt1v")
	feeMint      = chain.SolanaAddress("FHZNBei86FjdpSzU786ECJuqEAVYyfqCctt4aXWZh91M")
)

func usdc() chain.Mint { return chain.Mint{Address: usdcMint, Decimals: 6} }

func member() chain.Wallet {
	return chain.Wallet{ID: "wallet-member", Address: memberWallet, HasAppSigner: true}
}

func fund(amount uint64) relayer.TransferSpec {
	return relayer.TransferSpec{FromWallet: member(), To: treasury, Mint: usdc(), Amount: money.NewBaseUnits(amount, 6)}
}

type recorder struct {
	handler http.Handler
	mu      sync.Mutex
	methods []string
}

func (r *recorder) RoundTrip(req *http.Request) (*http.Response, error) {
	body, _ := io.ReadAll(req.Body)
	req.Body = io.NopCloser(bytes.NewReader(body))
	var call struct {
		Method string `json:"method"`
	}
	_ = json.Unmarshal(body, &call)
	r.mu.Lock()
	r.methods = append(r.methods, call.Method)
	r.mu.Unlock()
	rec := httptest.NewRecorder()
	r.handler.ServeHTTP(rec, req)
	return rec.Result(), nil
}

func (r *recorder) calls() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.methods...)
}

func keyConfig(label string) config.Config {
	return config.Config{
		Env: config.EnvStaging,
		Privy: config.Privy{
			AppID:                   "app-fixture",
			AppSecret:               "privy-app-5ecret",
			BaseURL:                 "http://privy.test/privy",
			VerificationKey:         fakes.PrivyVerificationKey(),
			AuthorizationPrivateKey: fakes.PrivyAuthorizationKeyConfig(),
			AuthorizationKeyID:      fakes.PrivyAuthorizationKeyID,
		},
		Solana:   config.Solana{RPCURL: "http://rpc.test/rpc/"},
		Relayer:  config.Relayer{PrivateKey: chain.EncodeBase58(fakes.FixtureKey(label))},
		Timeouts: config.Timeouts{RPC: 5 * time.Second, Privy: 10 * time.Second},
	}
}

type stack struct {
	relayer   *relayer.Relayer
	transfers *relayer.Transfers
	rpc       *recorder
	srv       *fakes.Server
}

func overFakes(t *testing.T, label string) stack {
	t.Helper()
	srv := fakes.New()
	rpc := &recorder{handler: srv}
	cfg := keyConfig(label)
	r, err := relayer.New(cfg, solana.New(cfg, clock.Real{}, httpclient.WithTransport(rpc)))
	if err != nil {
		t.Fatal(err)
	}
	signer, err := privy.New(cfg, clock.Real{}, httpclient.WithTransport(&recorder{handler: srv}))
	if err != nil {
		t.Fatal(err)
	}
	return stack{relayer: r, transfers: relayer.NewTransfers(r, signer), rpc: rpc, srv: srv}
}

func script(t *testing.T, srv *fakes.Server, step fakes.Step) {
	t.Helper()
	raw, _ := json.Marshal(step)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/_script", bytes.NewReader(raw)))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("script %s = %d %q", raw, rec.Code, rec.Body.String())
	}
}

type signerFunc func(ctx context.Context, walletID string, unsigned []byte) ([]byte, error)

func (f signerFunc) SignTransaction(ctx context.Context, walletID string, unsigned []byte) ([]byte, error) {
	return f(ctx, walletID, unsigned)
}

func wantCode(t *testing.T, err error, code errs.Code) {
	t.Helper()
	if err == nil || errs.CodeOf(err) != code {
		t.Fatalf("err = %v, want %s", err, code)
	}
}
