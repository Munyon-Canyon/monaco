package agents_test

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"io"
	"slices"
	"strings"
	"testing"
	"testing/iotest"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/agents/adapters"
)

func filled(b byte, n int) []byte { return bytes.Repeat([]byte{b}, n) }

func newBox(t *testing.T, raw []byte) adapters.KeyBox {
	t.Helper()
	box, err := adapters.NewKeyBox(base64.StdEncoding.EncodeToString(raw))
	if err != nil {
		t.Fatal(err)
	}
	return box
}

func stdlibGCM(t *testing.T, raw []byte) cipher.AEAD {
	t.Helper()
	block, err := aes.NewCipher(raw)
	if err != nil {
		t.Fatal(err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		t.Fatal(err)
	}
	return gcm
}

func TestNewKeyBox_requiresExactlyThirtyTwoBytesOfBase64(t *testing.T) {
	t.Parallel()
	encode := base64.StdEncoding.EncodeToString
	for name, raw := range map[string]string{
		"an empty value": "",
		"not base64":     "not base64!",
		"16 bytes":       encode(filled(1, 16)),
		"24 bytes":       encode(filled(1, 24)),
		"31 bytes":       encode(filled(1, 31)),
		"33 bytes":       encode(filled(1, 33)),
	} {
		if _, err := adapters.NewKeyBox(raw); errs.CodeOf(err) != errs.CodeInvalidConfig || err == nil {
			t.Errorf("%s: NewKeyBox(%q) err = %v, want invalid_config", name, raw, err)
		}
	}
	if _, err := adapters.NewKeyBox(encode(filled(1, 32))); err != nil {
		t.Fatalf("NewKeyBox(32 bytes) err = %v", err)
	}
}

func TestKeyBox_sealThenOpenReturnsTheSameKeyAndHidesIt(t *testing.T) {
	t.Parallel()
	box, key := newBox(t, filled(1, 32)), mintZeroKey(t)
	sealed, err := box.Seal(rand.Reader, key)
	if err != nil {
		t.Fatal(err)
	}
	opened, err := box.Open(sealed)
	if err != nil || opened.String() != key.String() {
		t.Fatalf("Open(Seal(key)) = %q, %v; want %q", opened.String(), err, key.String())
	}
	if bytes.Contains(sealed, []byte(key.String())) {
		t.Fatal("the sealed bytes contain the plaintext key")
	}
}

func TestKeyBox_sealIsTheNonceThenTheAES256GCMCiphertext(t *testing.T) {
	t.Parallel()
	box, key := newBox(t, filled(1, 32)), mintZeroKey(t)
	nonce := filled(7, 12)
	first, err := box.Seal(bytes.NewReader(nonce), key)
	if err != nil {
		t.Fatal(err)
	}
	second, err := box.Seal(bytes.NewReader(nonce), key)
	want := stdlibGCM(t, filled(1, 32)).Seal(slices.Clone(nonce), nonce, []byte(key.String()), nil)
	if err != nil || !bytes.Equal(first, second) || !bytes.Equal(first, want) {
		t.Fatalf("Seal with nonce %x = %x then %x, %v; want %x", nonce, first, second, err, want)
	}
	opened, err := box.Open(want)
	if err != nil || opened.String() != key.String() {
		t.Fatalf("Open(a stdlib AES-GCM seal) = %q, %v; want %q", opened.String(), err, key.String())
	}
}

func TestKeyBox_sealFailsInternalWhenTheNonceSourceFails(t *testing.T) {
	t.Parallel()
	box, key := newBox(t, filled(1, 32)), mintZeroKey(t)
	boom := errs.New(errs.CodeInternal, "test.entropy")
	for name, tt := range map[string]struct {
		source io.Reader
		cause  error
	}{
		"an empty source":  {strings.NewReader(""), io.EOF},
		"a short source":   {strings.NewReader("short"), io.ErrUnexpectedEOF},
		"a failing source": {iotest.ErrReader(boom), boom},
	} {
		sealed, err := box.Seal(tt.source, key)
		if errs.CodeOf(err) != errs.CodeInternal || !errors.Is(err, tt.cause) || sealed != nil {
			t.Errorf("%s: Seal() = %x, %v; want nil and internal wrapping %v", name, sealed, err, tt.cause)
		}
	}
}

func TestKeyBox_openRefusesAnythingItDidNotSeal(t *testing.T) {
	t.Parallel()
	box, other, key := newBox(t, filled(1, 32)), newBox(t, filled(2, 32)), mintZeroKey(t)
	sealed, err := box.Seal(rand.Reader, key)
	if err != nil {
		t.Fatal(err)
	}
	flipped := slices.Clone(sealed)
	flipped[len(flipped)-1] ^= 1
	nonce := filled(3, 12)
	notAKey := stdlibGCM(t, filled(1, 32)).Seal(slices.Clone(nonce), nonce, []byte("not an agent key"), nil)
	for name, tt := range map[string]struct {
		box   adapters.KeyBox
		input []byte
	}{
		"a flipped byte":                   {box, flipped},
		"a truncated input":                {box, sealed[:len(sealed)-1]},
		"an input shorter than nonce, tag": {box, sealed[:27]},
		"no input":                         {box, nil},
		"a box built from another key":     {other, sealed},
		"a plaintext that is not a key":    {box, notAKey},
	} {
		opened, err := tt.box.Open(tt.input)
		if errs.CodeOf(err) != errs.CodeDecodeFailed || err == nil || opened.String() != "" {
			t.Errorf("%s: Open() = %q, %v; want an empty key and decode_failed", name, opened.String(), err)
		}
	}
}
