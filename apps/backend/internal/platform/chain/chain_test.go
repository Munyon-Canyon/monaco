package chain_test

import (
	"bytes"
	"crypto/ed25519"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
)

const (
	owner    = chain.SolanaAddress("9WzDXwBbmkg8ZTbNMqUxvQRAyrZzDsGYdLVL9zYtAWWM")
	usdcMint = chain.SolanaAddress("EPjFWdd5AufqSSqeM2qN1xzybapC8G4wEGGkZwyTDt1v")
)

func TestBase58RoundTripsAndKeepsLeadingZeros(t *testing.T) {
	t.Parallel()
	for _, raw := range [][]byte{{}, {0}, {0, 0, 1}, {0xff, 0xfe}, bytes.Repeat([]byte{7}, 64)} {
		enc := chain.EncodeBase58(raw)
		got, ok := chain.DecodeBase58(enc)
		if !ok || !bytes.Equal(got, raw) {
			t.Fatalf("round trip %x -> %q -> %x, %v", raw, enc, got, ok)
		}
	}
	if got := chain.EncodeBase58([]byte{0, 0, 1}); got != "112" {
		t.Fatalf("EncodeBase58 = %q, want 112", got)
	}
	if _, ok := chain.DecodeBase58("0OIl"); ok {
		t.Fatal("DecodeBase58 accepted characters outside the alphabet")
	}
}

func TestParseAddress(t *testing.T) {
	t.Parallel()
	if got, err := chain.ParseAddress(string(owner)); err != nil || got != owner {
		t.Fatalf("ParseAddress = %q, %v", got, err)
	}
	for _, bad := range []string{"", "short", "0" + string(owner)[1:], string(owner) + "1"} {
		if _, err := chain.ParseAddress(bad); errs.CodeOf(err) != errs.CodeInvalidAddress {
			t.Fatalf("ParseAddress(%q) err = %v, want invalid_address", bad, err)
		}
	}
	if _, err := chain.SolanaAddress("nope").Bytes(); errs.CodeOf(err) != errs.CodeInvalidAddress {
		t.Fatalf("Bytes err = %v", err)
	}
}

func TestAddressAndSignatureOfEncodeBase58(t *testing.T) {
	t.Parallel()
	key := ed25519.NewKeyFromSeed(bytes.Repeat([]byte{1}, 32))
	pub := key.Public().(ed25519.PublicKey)
	addr := chain.AddressOf(pub)
	if b, err := addr.Bytes(); err != nil || !bytes.Equal(b, pub) {
		t.Fatalf("AddressOf round trip = %x, %v", b, err)
	}
	sig := ed25519.Sign(key, []byte("m"))
	if got, _ := chain.DecodeBase58(string(chain.SignatureOf(sig))); !bytes.Equal(got, sig) {
		t.Fatal("SignatureOf does not encode the signature bytes")
	}
}

func TestAssociatedTokenAccountMatchesMainnet(t *testing.T) {
	t.Parallel()
	got, err := chain.AssociatedTokenAccount(owner, usdcMint, chain.SPLProgram)
	if err != nil || got != "FGETo8T8wMcN2wCjav8VK6eh3dLk63evNDPxzLSJra8B" {
		t.Fatalf("ATA = %q, %v", got, err)
	}
	b, _ := got.Bytes()
	if chain.OnCurve(b) {
		t.Fatal("an ATA must be off the curve")
	}
	if _, err := chain.AssociatedTokenAccount(
		"bad",
		usdcMint,
		chain.SPLProgram,
	); errs.CodeOf(
		err,
	) != errs.CodeInvalidAddress {
		t.Fatalf("bad owner err = %v", err)
	}
}

func TestOnCurve(t *testing.T) {
	t.Parallel()
	pub, _ := owner.Bytes()
	if !chain.OnCurve(pub) {
		t.Fatal("a wallet public key is on the curve")
	}
	identity := make([]byte, 32)
	identity[0] = 1
	if !chain.OnCurve(identity) {
		t.Fatal("the identity point (y=1, x=0) is on the curve")
	}
}
