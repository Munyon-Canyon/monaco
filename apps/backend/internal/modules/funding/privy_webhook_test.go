package funding_test

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	cabalport "github.com/monaco/monaco/apps/backend/internal/modules/cabal/port"
	"github.com/monaco/monaco/apps/backend/internal/modules/funding"
	"github.com/monaco/monaco/apps/backend/internal/modules/funding/adapters"
	"github.com/monaco/monaco/apps/backend/internal/modules/funding/app"
	"github.com/monaco/monaco/apps/backend/internal/platform/bus"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/privy"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/config"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpx"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpx/api"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/platform/poller"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
	"github.com/monaco/monaco/apps/backend/internal/testkit/fakes"
)

type treasuryWallets struct {
	wallets []cabalport.TreasuryWallet
	loads   atomic.Int32
	err     error
}

func (w *treasuryWallets) TreasuryWallets(context.Context) ([]cabalport.TreasuryWallet, error) {
	w.loads.Add(1)
	return w.wallets, w.err
}

type webhookEnv struct {
	*watchEnv
	wallets *treasuryWallets
	handler http.Handler
}

func newWebhookEnv(t *testing.T) *webhookEnv {
	t.Helper()
	watch := newWatchEnv(t)
	verifier, err := privy.New(config.Config{
		Privy: config.Privy{
			BaseURL: "http://privy.test", VerificationKey: fakes.PrivyVerificationKey(),
			WebhookSecret: fakes.PrivyWebhookSecret,
		},
		Timeouts: config.Timeouts{Privy: time.Second},
	}, watch.clock)
	if err != nil {
		t.Fatal(err)
	}
	wallets := &treasuryWallets{wallets: []cabalport.TreasuryWallet{
		{CabalID: watch.cabal.ID, Address: watch.cabal.TreasuryAddress},
	}}
	env := &webhookEnv{watchEnv: watch, wallets: wallets}
	env.handler = adapters.PrivyWebhook{
		Verifier: verifier, Treasuries: app.NewTreasuryMap(wallets, watch.clock),
		Detect: detectorFunc(func(ctx context.Context, cmd app.DetectExternalDeposit) (app.DetectResult, error) {
			return app.NewDetectExternalDepositHandler(env.deps).Handle(ctx, cmd)
		}),
		Problem: httpx.Problem,
	}
	return env
}

type detectorFunc func(context.Context, app.DetectExternalDeposit) (app.DetectResult, error)

func (f detectorFunc) Handle(ctx context.Context, cmd app.DetectExternalDeposit) (app.DetectResult, error) {
	return f(ctx, cmd)
}

func (e *webhookEnv) post(t *testing.T, header http.Header, body []byte) int {
	t.Helper()
	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/v1/webhooks/privy", bytes.NewReader(body))
	for k, v := range header {
		req.Header[k] = v
	}
	rec := httptest.NewRecorder()
	e.handler.ServeHTTP(rec, req)
	return rec.Code
}

func (e *webhookEnv) deposit(t *testing.T, to chain.SolanaAddress) []byte {
	t.Helper()
	const micros = 5_000_000
	sig := e.inbound(t, e.usdcMint(), micros, randomAddress(t))
	return fakes.PrivyFundsDeposited(fakes.PrivyDeposit{
		Recipient: to, Sender: randomAddress(t), Mint: e.usdc, Amount: micros, Signature: sig,
	})
}

func TestPrivyWebhook_DepositToATreasuryIsDetectedOnce(t *testing.T) {
	t.Parallel()
	env := newWebhookEnv(t)
	body := env.deposit(t, env.cabal.TreasuryAddress)
	header := fakes.SignPrivyWebhook("msg_1", env.clock.Now(), body)

	first, again := env.post(t, header, body), env.post(t, header, body)

	if first != http.StatusOK || again != http.StatusOK {
		t.Fatalf("status = %d then %d, want 200 twice", first, again)
	}
	if n := len(externalDeposits(t, env.pool)); n != 1 || externalPauses(t, env.pool) != 1 {
		t.Fatalf("rows = %d, want one detected row and one pause", n)
	}
}

func TestPrivyWebhook_RefusesUnverifiedRequests(t *testing.T) {
	t.Parallel()
	env := newWebhookEnv(t)
	body := env.deposit(t, env.cabal.TreasuryAddress)
	tampered := bytes.Replace(body, []byte("5000000"), []byte("9000000"), 1)
	cases := map[string]struct {
		header http.Header
		body   []byte
	}{
		"tampered body":       {fakes.SignPrivyWebhook("msg_1", env.clock.Now(), body), tampered},
		"six minutes old":     {fakes.SignPrivyWebhook("msg_2", env.clock.Now().Add(-6*time.Minute), body), body},
		"six minutes ahead":   {fakes.SignPrivyWebhook("msg_3", env.clock.Now().Add(6*time.Minute), body), body},
		"no svix headers":     {http.Header{}, body},
		"empty unsigned body": {http.Header{}, []byte("{}")},
		"oversized body":      {http.Header{}, bytes.Repeat([]byte("x"), 65<<10)},
	}
	for name, c := range cases {
		if got := env.post(t, c.header, c.body); got != http.StatusUnauthorized {
			t.Errorf("%s: status = %d, want 401", name, got)
		}
	}
	if len(externalDeposits(t, env.pool)) != 0 || env.chain.calls.Load() != 0 {
		t.Fatal("an unverified webhook reached the classifier")
	}
}

func TestPrivyWebhook_IgnoresOtherEventsAndOtherWallets(t *testing.T) {
	t.Parallel()
	env := newWebhookEnv(t)
	other := []byte(`{"type":"user.created"}`)
	member := env.deposit(t, randomAddress(t))

	if got := env.post(t, fakes.SignPrivyWebhook("msg_1", env.clock.Now(), other), other); got != http.StatusOK {
		t.Fatalf("other event status = %d, want 200", got)
	}
	if got := env.post(t, fakes.SignPrivyWebhook("msg_2", env.clock.Now(), member), member); got != http.StatusOK {
		t.Fatalf("member wallet deposit status = %d, want 200", got)
	}
	if len(externalDeposits(t, env.pool)) != 0 || env.chain.calls.Load() != 0 {
		t.Fatal("a non-treasury webhook reached the classifier")
	}
}

func TestPrivyWebhook_FailuresReturnAProblemSoPrivyRetries(t *testing.T) {
	t.Parallel()
	for name, arrange := range map[string]func(*webhookEnv){
		"treasury lookup fails": func(e *webhookEnv) { e.wallets.err = errs.New(errs.CodeInternal, "test") },
		"chain read fails":      func(e *webhookEnv) { e.deps.Chain = failing{} },
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			env := newWebhookEnv(t)
			arrange(env)
			body := env.deposit(t, env.cabal.TreasuryAddress)
			if got := env.post(t, fakes.SignPrivyWebhook("msg_1", env.clock.Now(), body), body); got < 500 {
				t.Fatalf("status = %d, want a 5xx", got)
			}
		})
	}
}

func TestTreasuryMap_CachesForAMinuteAndReloadsOnAMiss(t *testing.T) {
	t.Parallel()
	clk := testkit.NewClock(time.Unix(1_800_000_000, 0))
	known := cabalport.TreasuryWallet{Address: "treasury-a"}
	wallets := &treasuryWallets{wallets: []cabalport.TreasuryWallet{known}}
	m := app.NewTreasuryMap(wallets, clk)
	lookup := func(addr chain.SolanaAddress) bool {
		t.Helper()
		_, ok, err := m.CabalFor(t.Context(), addr)
		if err != nil {
			t.Fatal(err)
		}
		return ok
	}

	first, second := lookup("treasury-a"), lookup("treasury-a")
	if !first || !second || wallets.loads.Load() != 1 {
		t.Fatalf("loads = %d after two hits, want 1", wallets.loads.Load())
	}
	if lookup("member-wallet") || wallets.loads.Load() != 2 {
		t.Fatalf("loads = %d after a miss, want 2", wallets.loads.Load())
	}
	clk.Advance(app.TreasuryMapTTL)
	if !lookup("treasury-a") || wallets.loads.Load() != 3 {
		t.Fatalf("loads = %d after the TTL, want 3", wallets.loads.Load())
	}
}

func TestModule_MountsTheWebhookAndRefusesWithoutPrivyConfig(t *testing.T) {
	t.Parallel()
	mux := http.NewServeMux()
	funding.New(module.Deps{Config: config.Config{}, Clock: testkit.NewClock(time.Unix(0, 0))}).
		Mount(api.Mount{Mux: mux, Problem: httpx.Problem})
	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/v1/webhooks/privy",
		bytes.NewReader([]byte("{}")))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}

type ownerModule struct{ owned chain.Signature }

func (ownerModule) Name() string              { return "owner" }
func (ownerModule) Mount(api.Mount)           {}
func (ownerModule) Consumers() []bus.Consumer { return nil }
func (ownerModule) Pollers() []poller.Poller  { return nil }
func (o ownerModule) OwnsSignature(_ context.Context, sig chain.Signature) (bool, error) {
	return sig == o.owned, nil
}

func TestModule_WebhookSkipsASignatureAWiredModuleOwns(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	clk := testkit.NewClock(clock.Real{}.Now())
	cabal := testkit.NewCabal(t, pool)
	cfg := config.Config{
		Privy: config.Privy{
			BaseURL: "http://privy.test", VerificationKey: fakes.PrivyVerificationKey(),
			WebhookSecret: fakes.PrivyWebhookSecret,
		},
		Solana:   config.Solana{RPCURL: "http://127.0.0.1:1/rpc"},
		Timeouts: config.Timeouts{Privy: time.Second, RPC: time.Second},
	}
	f := funding.New(module.Deps{Config: cfg, Clock: clk, Pool: pool, UoW: db.New(pool, testkit.NewIDs(70), clk)})
	sig := chain.Signature("owned-by-another-module")
	module.NewSet(f, ownerModule{owned: sig})
	mux := http.NewServeMux()
	f.Mount(api.Mount{Mux: mux, Problem: httpx.Problem})
	body := fakes.PrivyFundsDeposited(fakes.PrivyDeposit{
		Recipient: cabal.TreasuryAddress, Sender: "sender", Mint: "mint", Amount: 5_000_000, Signature: sig,
	})
	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/v1/webhooks/privy", bytes.NewReader(body))
	for k, v := range fakes.SignPrivyWebhook("msg_1", clk.Now(), body) {
		req.Header[k] = v
	}
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || len(externalDeposits(t, pool)) != 0 {
		t.Fatalf("status = %d with %d rows, want 200 and no row", rec.Code, len(externalDeposits(t, pool)))
	}
}
