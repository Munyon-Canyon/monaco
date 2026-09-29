package privy_test

import (
	"crypto/ed25519"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/testkit/fakes"
)

func creates(u *upstream) []sent {
	var out []sent
	for _, r := range u.requests() {
		if r.method == http.MethodPost && r.path == "/privy/v1/wallets" {
			out = append(out, r)
		}
	}
	return out
}

func TestFindOrCreateMemberWallet_ReusesExisting(t *testing.T) {
	t.Parallel()
	c, u, _ := overFakes(t)
	got, err := c.FindOrCreateMemberWallet(t.Context(), "did:privy:member-with-wallet")
	want := chain.Wallet{
		ID:           "wallet-member",
		Address:      "Dht9c9YfstFWkNYXgqr8HZbhqVn563bCpNU6zL32Ftqf",
		HasAppSigner: true,
	}
	if err != nil || got != want {
		t.Fatalf("FindOrCreateMemberWallet = %+v, %v", got, err)
	}
	if n := len(creates(u)); n != 0 {
		t.Fatalf("%d create calls for a user who already has a Solana wallet, want 0", n)
	}
	if q := u.requests()[0].query; q != "chain_type=solana&user_id=did%3Aprivy%3Amember-with-wallet" {
		t.Fatalf("list query = %q", q)
	}
	legacy, err := c.FindOrCreateMemberWallet(t.Context(), "did:privy:member-legacy")
	if err != nil || legacy.HasAppSigner || legacy.ID != "wallet-legacy" || len(creates(u)) != 0 {
		t.Fatalf("legacy wallet = %+v, %v; it is reused as is, never recreated", legacy, err)
	}
}

func TestFindOrCreateMemberWallet_createsOnceWithTheUserAsOwnerAndTheAppSigner(t *testing.T) {
	t.Parallel()
	c, u, _ := overFakes(t)
	first, err := c.FindOrCreateMemberWallet(t.Context(), "did:privy:member-new")
	if err != nil || !first.HasAppSigner {
		t.Fatalf("FindOrCreateMemberWallet = %+v, %v", first, err)
	}
	again, err := c.FindOrCreateMemberWallet(t.Context(), "did:privy:member-new")
	if err != nil || again != first {
		t.Fatalf("second sign-in = %+v, %v, want %+v", again, err, first)
	}
	made := creates(u)
	if len(made) != 1 {
		t.Fatalf("%d create calls, want exactly 1", len(made))
	}
	var body map[string]any
	_ = json.Unmarshal([]byte(made[0].body), &body)
	want := `{"additional_signers":[{"signer_id":"fixture-key-quorum"}],"chain_type":"solana","owner":{"user_id":"did:privy:member-new"}}`
	if got, _ := json.Marshal(body); string(got) != want {
		t.Fatalf("create body = %s, want %s", got, want)
	}
	if key := made[0].header.Get("privy-idempotency-key"); key != "member-wallet:did:privy:member-new" {
		t.Fatalf("idempotency key = %q", key)
	}
}

func TestCreateAppWallet_SameKeyReturnsSameWallet(t *testing.T) {
	t.Parallel()
	c, u, _ := overFakes(t)
	first, err := c.CreateAppWallet(t.Context(), "cabal-treasury:7")
	if err != nil || !first.HasAppSigner {
		t.Fatalf("CreateAppWallet = %+v, %v", first, err)
	}
	again, err := c.CreateAppWallet(t.Context(), "cabal-treasury:7")
	if err != nil || again != first {
		t.Fatalf("retry = %+v, %v, want the same wallet %+v", again, err, first)
	}
	other, err := c.CreateAppWallet(t.Context(), "cabal-treasury:8")
	if err != nil || other.ID == first.ID || other.Address == first.Address {
		t.Fatalf("another key = %+v, %v, want a different wallet", other, err)
	}
	made := creates(u)
	if made[0].body != `{"chain_type":"solana","owner_id":"fixture-key-quorum"}` ||
		made[0].header.Get("privy-idempotency-key") != "cabal-treasury:7" {
		t.Fatalf("create = %s with key %q", made[0].body, made[0].header.Get("privy-idempotency-key"))
	}
	wantAddr := chain.AddressOf(fakes.PrivyWalletKey(first.ID).Public().(ed25519.PublicKey))
	if first.Address != wantAddr {
		t.Fatalf("address %s is not the fake's key for %s", first.Address, first.ID)
	}
}

func TestWallets_refusals(t *testing.T) {
	t.Parallel()
	c, u, _ := overFakes(t)
	_, err := c.CreateAppWallet(t.Context(), "")
	wantCode(t, err, errs.CodeInvalidInput)
	cfg := testConfig()
	cfg.Privy.AuthorizationKeyID = ""
	_, err = clientWith(cfg, u, clock.Real{}).CreateAppWallet(t.Context(), "k")
	wantCode(t, err, errs.CodeInternal)
	_, err = clientWith(cfg, u, clock.Real{}).FindOrCreateMemberWallet(t.Context(), "did:privy:member-new")
	wantCode(t, err, errs.CodeInternal)
	if n := len(creates(u)); n != 0 {
		t.Fatalf("%d creates without an app signer", n)
	}
	_, err = client(
		replying(http.StatusOK, `{"data":[{"id":"w","address":"nope"}]}`),
	).FindOrCreateMemberWallet(t.Context(), "u")
	wantCode(t, err, errs.CodeDecodeFailed)
	_, err = client(replying(http.StatusServiceUnavailable, ``)).FindOrCreateMemberWallet(t.Context(), "u")
	wantCode(t, err, errs.CodePrivyUnavailable)
	_, err = client(replying(http.StatusBadRequest, `{}`)).CreateAppWallet(t.Context(), "k")
	wantCode(t, err, errs.CodeInvalidInput)
}
