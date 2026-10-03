package privy_test

import (
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"strings"
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/privy"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
	"github.com/monaco/monaco/apps/backend/internal/testkit/fakes"
)

func token(claims map[string]any) string {
	raw, _ := json.Marshal(claims)
	return fakes.SignES256(fakes.PrivyTokenKey(), `{"alg":"ES256","typ":"JWT"}`, string(raw))
}

func TestVerifyAccessToken_acceptsAFixtureToken(t *testing.T) {
	t.Parallel()
	now := clock.Real{}.Now()
	got, err := client(
		nil,
	).VerifyAccessToken(t.Context(), fakes.PrivyAccessToken(appID, "did:privy:member", now, time.Hour))
	if err != nil || got != "did:privy:member" {
		t.Fatalf("VerifyAccessToken = %q, %v", got, err)
	}
	many := token(
		map[string]any{
			"iss": "privy.io",
			"aud": []string{"other", appID},
			"sub": "s",
			"exp": now.Add(time.Minute).Unix(),
		},
	)
	if got, err := client(nil).VerifyAccessToken(t.Context(), many); err != nil || got != "s" {
		t.Fatalf("audience list = %q, %v", got, err)
	}
}

func TestVerifyAccessToken_refusals(t *testing.T) {
	t.Parallel()
	clk := testkit.NewClock(clock.Real{}.Now())
	now := clk.Now()
	ok := func(edit func(map[string]any)) string {
		c := map[string]any{"iss": "privy.io", "aud": appID, "sub": "did:privy:x", "exp": now.Add(time.Hour).Unix()}
		edit(c)
		return token(c)
	}
	other, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	good := ok(func(map[string]any) {})
	for name, raw := range map[string]string{
		"expired":         ok(func(c map[string]any) { c["exp"] = now.Unix() }),
		"not yet valid":   ok(func(c map[string]any) { c["nbf"] = now.Add(time.Minute).Unix() }),
		"other issuer":    ok(func(c map[string]any) { c["iss"] = "evil.io" }),
		"other audience":  ok(func(c map[string]any) { c["aud"] = "other-app" }),
		"audience number": ok(func(c map[string]any) { c["aud"] = 7 }),
		"no subject":      ok(func(c map[string]any) { c["sub"] = "" }),
		"other key":       fakes.SignES256(other, `{"alg":"ES256"}`, `{"iss":"privy.io","aud":"app-fixture","sub":"x","exp":9999999999}`),
		"hs256":           fakes.SignES256(fakes.PrivyTokenKey(), `{"alg":"HS256"}`, `{"sub":"x"}`),
		"claims not json": fakes.SignES256(fakes.PrivyTokenKey(), `{"alg":"ES256"}`, `not json`),
		"header not json": "bm90." + strings.SplitN(good, ".", 2)[1],
		"two parts":       "a.b",
		"bad base64":      "!!.!!.!!",
		"short signature": strings.Join(strings.Split(good, ".")[:2], ".") + ".AAAA",
		"empty":           "",
	} {
		_, err := clientWith(testConfig(), nil, clk).VerifyAccessToken(t.Context(), raw)
		if errs.CodeOf(err) != errs.CodeUnauthorized {
			t.Fatalf("%s: err = %v, want unauthorized", name, err)
		}
		if strings.Contains(err.Error(), raw) && raw != "" {
			t.Fatalf("%s: error echoes the token", name)
		}
	}
}

func TestNew_refusesAMissingOrUnusableVerificationKey(t *testing.T) {
	t.Parallel()
	edPub, _, _ := ed25519.GenerateKey(rand.Reader)
	edDER, _ := x509.MarshalPKIXPublicKey(edPub)
	for name, key := range map[string]string{
		"missing":   "",
		"not PEM":   "MFkwEwYHKoZIzj0CAQYIKoZIzj0DAQcDQgAE",
		"not a key": "-----BEGIN PUBLIC KEY-----\nAAAA\n-----END PUBLIC KEY-----\n",
		"not ecdsa": string(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: edDER})),
	} {
		cfg := testConfig()
		cfg.Privy.VerificationKey = key
		c, err := privy.New(cfg, clock.Real{})
		if c != nil || errs.CodeOf(err) != errs.CodeInvalidInput {
			t.Fatalf("%s: New = %v, %v, want invalid_input", name, c, err)
		}
	}
}

func TestVerifyAccessToken_acceptsAKeyWrittenOnOneLine(t *testing.T) {
	t.Parallel()
	raw := fakes.PrivyAccessToken(appID, "s", clock.Real{}.Now(), time.Hour)
	cfg := testConfig()
	cfg.Privy.VerificationKey = strings.ReplaceAll(fakes.PrivyVerificationKey(), "\n", `\n`)
	if _, err := clientWith(cfg, nil, clock.Real{}).VerifyAccessToken(t.Context(), raw); err != nil {
		t.Fatalf("a PEM written on one line with literal \\n: %v", err)
	}
}
