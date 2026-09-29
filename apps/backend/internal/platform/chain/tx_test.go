package chain_test

import (
	"bytes"
	"crypto/ed25519"
	"slices"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
)

func key(b byte) ed25519.PrivateKey { return ed25519.NewKeyFromSeed(bytes.Repeat([]byte{b}, 32)) }

func pub(k ed25519.PrivateKey) []byte { return k.Public().(ed25519.PublicKey) }

func message(prefix []byte, required byte, keys ...[]byte) []byte {
	msg := append(append([]byte{}, prefix...), required, 0, 1)
	msg = append(msg, chain.CompactU16(len(keys))...)
	for _, k := range keys {
		msg = append(msg, k...)
	}
	return append(append(msg, make([]byte, 32)...), 0)
}

func unsigned(n int, msg []byte) []byte {
	return append(append(chain.CompactU16(n), make([]byte, n*ed25519.SignatureSize)...), msg...)
}

func TestDecodeTransaction_signsTheSlotOfTheMatchingSigner(t *testing.T) {
	t.Parallel()
	a, b, c := key(1), key(2), key(3)
	for _, prefix := range [][]byte{nil, {0x80}} {
		raw := unsigned(2, message(prefix, 2, pub(a), pub(b), pub(c)))
		tx, err := chain.DecodeTransaction(raw)
		if err != nil {
			t.Fatal(err)
		}
		if want := []chain.SolanaAddress{
			chain.AddressOf(pub(a)),
			chain.AddressOf(pub(b)),
		}; !slices.Equal(
			tx.Signers,
			want,
		) {
			t.Fatalf("Signers = %v, want %v", tx.Signers, want)
		}
		if !bytes.Equal(tx.Encode(), raw) {
			t.Fatal("Encode does not round trip")
		}
		if err := tx.Sign(b); err != nil || tx.Signed(0) || !tx.Signed(1) {
			t.Fatalf("Sign(b) = %v; signed slots %v %v", err, tx.Signed(0), tx.Signed(1))
		}
		if err := tx.Sign(c); errs.CodeOf(err) != errs.CodeInvalidInput {
			t.Fatalf("Sign by a non-signer = %v", err)
		}
	}
}

func TestDecodeTransaction_refusesMalformedBytes(t *testing.T) {
	t.Parallel()
	a := pub(key(1))
	for name, raw := range map[string][]byte{
		"empty":             nil,
		"count overflows":   {0xff, 0xff, 0xff},
		"short signatures":  append([]byte{1}, make([]byte, 10)...),
		"no header":         unsigned(1, []byte{1}),
		"header disagrees":  unsigned(1, message(nil, 2, a, a)),
		"too few keys":      unsigned(2, message(nil, 2, a)),
		"keys cut short":    unsigned(1, append(message(nil, 1)[:4], 1, 2, 3)),
		"key count missing": unsigned(1, []byte{1, 0, 0}),
		"no signatures":     unsigned(0, message(nil, 0, a)),
	} {
		if _, err := chain.DecodeTransaction(raw); errs.CodeOf(err) != errs.CodeInvalidInput {
			t.Fatalf("%s: err = %v, want invalid_input", name, err)
		}
	}
}

func TestCompactU16RoundTrips(t *testing.T) {
	t.Parallel()
	for _, n := range []int{0, 1, 127, 128, 300, 16383, 16384, 65535} {
		got, rest, ok := chain.DecodeCompactU16(append(chain.CompactU16(n), 9))
		if !ok || got != n || !bytes.Equal(rest, []byte{9}) {
			t.Fatalf("compact-u16 %d -> %d %v %v", n, got, rest, ok)
		}
	}
	if b := chain.CompactU16(300); !bytes.Equal(b, []byte{0xac, 0x02}) {
		t.Fatalf("CompactU16(300) = %x", b)
	}
}
