package domain

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"strings"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/errs"
)

func TestAdminServiceToken_newTokenIsPrefixedBase64URLOf32BytesWithMatchingHash(t *testing.T) {
	t.Parallel()
	random := bytes.NewReader(bytes.Repeat([]byte{0xfb}, 32))
	token, hash, err := NewServiceToken(random)
	if err != nil {
		t.Fatal(err)
	}
	body, ok := strings.CutPrefix(token, ServiceTokenPrefix)
	if !ok || !IsServiceToken(token) {
		t.Fatalf("token %q lacks the %q prefix", token, ServiceTokenPrefix)
	}
	raw, err := base64.RawURLEncoding.DecodeString(body)
	if err != nil || len(raw) != 32 {
		t.Fatalf("body decodes to %d bytes, err %v; want 32", len(raw), err)
	}
	if sum := sha256.Sum256([]byte(token)); !bytes.Equal(hash, sum[:]) || !bytes.Equal(hash, HashServiceToken(token)) {
		t.Fatal("hash is not the SHA-256 of the whole token")
	}
	other, _, err := NewServiceToken(bytes.NewReader(bytes.Repeat([]byte{0x01}, 32)))
	if err != nil || other == token || bytes.Equal(HashServiceToken(other), hash) {
		t.Fatalf("different randomness gave %q, err %v", other, err)
	}
}

func TestAdminServiceToken_newTokenFailsOnShortRandomness(t *testing.T) {
	t.Parallel()
	if token, hash, err := NewServiceToken(
		bytes.NewReader(make([]byte, 31)),
	); err == nil || token != "" ||
		hash != nil {
		t.Fatalf("short read = %q, %x, %v", token, hash, err)
	}
}

func TestAdminServiceToken_namesAreLowercaseSlugs(t *testing.T) {
	t.Parallel()
	for _, ok := range []string{"grafana", "grafana-prod", "a", "g_1", strings.Repeat("a", 63)} {
		if got, err := ParseServiceTokenName(ok); err != nil || got != ok {
			t.Errorf("ParseServiceTokenName(%q) = %q, %v", ok, got, err)
		}
	}
	for _, bad := range []string{"", "Grafana", "-grafana", "gra fana", "gra:fana", strings.Repeat("a", 64)} {
		if _, err := ParseServiceTokenName(bad); errs.CodeOf(err) != errs.CodeInvalidInput {
			t.Errorf("ParseServiceTokenName(%q) err = %v, want invalid_input", bad, err)
		}
	}
}

func TestAdminServiceToken_onlyMstPrefixedBearersAreServiceTokens(t *testing.T) {
	t.Parallel()
	if IsServiceToken("eyJhbGciOi") || IsServiceToken("") || IsServiceToken("MST_x") || !IsServiceToken("mst_x") {
		t.Fatal("IsServiceToken misclassifies a bearer")
	}
}
