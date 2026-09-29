package privy_test

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/base64"
	"net/http"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/privy"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/testkit/fakes"
)

func unsignedFor(signers ...ed25519.PublicKey) []byte {
	msg := append(chain.CompactU16(len(signers)), 0, 0)
	msg = append(msg, chain.CompactU16(len(signers))...)
	for _, s := range signers {
		msg = append(msg, s...)
	}
	msg = append(append(msg, make([]byte, 32)...), 0)
	return append(append(chain.CompactU16(len(signers)), make([]byte, len(signers)*64)...), msg...)
}

func walletPub(id string) ed25519.PublicKey {
	return fakes.PrivyWalletKey(id).Public().(ed25519.PublicKey)
}

func TestSignTransaction_theWalletSignsItsSlotUnderTheAppAuthorization(t *testing.T) {
	t.Parallel()
	c, u, _ := overFakes(t)
	payer := ed25519.NewKeyFromSeed(bytes.Repeat([]byte{9}, 32)).Public().(ed25519.PublicKey)
	signed, err := c.SignTransaction(t.Context(), "wallet-member", unsignedFor(payer, walletPub("wallet-member")))
	if err != nil {
		t.Fatal(err)
	}
	tx, err := chain.DecodeTransaction(signed)
	if err != nil || tx.Signed(0) || !tx.Signed(1) {
		t.Fatalf("signed = %v; payer slot signed %v, wallet slot signed %v", err, tx.Signed(0), tx.Signed(1))
	}
	req := u.requests()[0]
	if req.path != "/privy/v1/wallets/wallet-member/rpc" || req.header.Get("privy-authorization-signature") == "" {
		t.Fatalf("request = %s with signature %q", req.path, req.header.Get("privy-authorization-signature"))
	}
}

func TestSignTransaction_refusals(t *testing.T) {
	t.Parallel()
	c, _, _ := overFakes(t)
	_, err := c.SignTransaction(t.Context(), "wallet-member", unsignedFor(walletPub("wallet-legacy")))
	wantCode(t, err, errs.CodeInvalidInput)
	_, err = c.SignTransaction(t.Context(), "wallet-none", unsignedFor(walletPub("wallet-none")))
	wantCode(t, err, errs.CodeNotFound)
	cfg := testConfig()
	cfg.Privy.AuthorizationPrivateKey = "wallet-auth:" + base64.StdEncoding.EncodeToString([]byte("junk"))
	for _, key := range []string{"", "wallet-auth:!!", cfg.Privy.AuthorizationPrivateKey} {
		cfg.Privy.AuthorizationPrivateKey = key
		_, err = clientWith(cfg, nil, clock.Real{}).SignTransaction(t.Context(), "wallet-member", []byte{0})
		wantCode(t, err, errs.CodeInternal)
	}
	for body, want := range map[string]errs.Code{
		`{"data":{"signed_transaction":"!!"}}`: errs.CodeDecodeFailed,
		`{"data":{}}`:                          errs.CodeDecodeFailed,
		`{}`:                                   errs.CodeDecodeFailed,
	} {
		_, err := client(replying(http.StatusOK, body)).SignTransaction(t.Context(), "w", []byte{1})
		wantCode(t, err, want)
	}
}

func TestSignTransaction_aForgedAuthorizationIsRefusedByTheFake(t *testing.T) {
	t.Parallel()
	forger, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	der, _ := x509.MarshalPKCS8PrivateKey(forger)
	cfg := testConfig()
	cfg.Privy.AuthorizationPrivateKey = "wallet-auth:" + base64.StdEncoding.EncodeToString(der)
	u := &upstream{handler: fakes.New()}
	_, err := clientWith(
		cfg,
		u,
		clock.Real{},
	).SignTransaction(t.Context(), "wallet-member", unsignedFor(walletPub("wallet-member")))
	wantCode(t, err, errs.CodeInternal)
	if n := len(u.requests()); n != 1 {
		t.Fatalf("%d requests; a 401 is not retried", n)
	}
}

func TestCanonical_sortsKeysWithoutEscaping(t *testing.T) {
	t.Parallel()
	got := string(privy.Canonical(map[string]any{"b": "<&>", "a": map[string]int{"z": 1, "y": 2}}))
	if got != `{"a":{"y":2,"z":1},"b":"<&>"}` {
		t.Fatalf("Canonical = %s", got)
	}
}
