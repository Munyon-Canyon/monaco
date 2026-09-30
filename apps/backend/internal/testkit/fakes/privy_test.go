package fakes_test

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/x509"
	"encoding/pem"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/testkit/fakes"
)

func TestPrivyVerificationKey_isThePublicHalfOfThePrivyTokenKey(t *testing.T) {
	t.Parallel()
	der, err := x509.MarshalPKIXPublicKey(&fakes.PrivyTokenKey().PublicKey)
	if err != nil {
		t.Fatal(err)
	}
	want := string(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: der}))
	if got := fakes.PrivyVerificationKey(); got != want {
		t.Fatalf("privy.FixtureVerificationKey is not the public half of PrivyTokenKey. Set it to:\n%s", want)
	}
}

func TestOtherPrivyVerificationKey_isAnotherValidES256Key(t *testing.T) {
	t.Parallel()
	block, _ := pem.Decode([]byte(fakes.OtherPrivyVerificationKey()))
	if block == nil {
		t.Fatal("OtherPrivyVerificationKey is not PEM")
	}
	parsed, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		t.Fatal(err)
	}
	key, ok := parsed.(*ecdsa.PublicKey)
	if !ok {
		t.Fatalf("OtherPrivyVerificationKey parses to %T, want *ecdsa.PublicKey", parsed)
	}
	if key.Curve != elliptic.P256() {
		t.Fatalf("OtherPrivyVerificationKey is on %s, want P-256", key.Curve.Params().Name)
	}
	if key.Equal(&fakes.PrivyTokenKey().PublicKey) {
		t.Fatal("OtherPrivyVerificationKey is the fixture key")
	}
}
