package agents

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestVerifierKey_usesTheFlagOrTheHomeDefault(t *testing.T) {
	t.Parallel()
	env := &Env{Home: "/home/dev"}
	got, err := env.verifierKey("explicit.pem")
	if err != nil || got != "explicit.pem" {
		t.Fatalf("flag: %q %v", got, err)
	}
	got, err = env.verifierKey("")
	want := filepath.Join("/home/dev", ".config", "monaco", "verifier.pem")
	if err != nil || got != want {
		t.Fatalf("default: %q %v", got, err)
	}
	env.Home = ""
	if _, err = env.verifierKey(""); err == nil || !strings.Contains(cliText(err), "HOME is unset") {
		t.Fatalf("home: %v", err)
	}
}

func TestSignJWT_setsTheIssuedAndExpiryWindow(t *testing.T) {
	t.Parallel()
	now := time.Unix(1_700_000_000, 0)
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	token, err := signJWT(key, "99", now)
	if err != nil {
		t.Fatal(err)
	}
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		t.Fatalf("token=%q", token)
	}
	raw, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		t.Fatal(err)
	}
	var payload struct {
		Iat int64  `json:"iat"`
		Exp int64  `json:"exp"`
		Iss string `json:"iss"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Iss != "99" || payload.Iat != now.Unix()-60 || payload.Exp != now.Unix()+540 {
		t.Fatalf("payload=%s", raw)
	}
}

func TestSignJWT_rejectsAShortKey(t *testing.T) {
	t.Parallel()
	key, err := rsa.GenerateKey(rand.Reader, 256)
	if err != nil {
		t.Fatal(err)
	}
	_, err = signJWT(key, "99", time.Unix(1_700_000_000, 0))
	if err == nil || !strings.Contains(err.Error(), "sign verifier jwt") {
		t.Fatalf("err=%v", err)
	}
}

func TestLoadKey_acceptsPKCS1AndPKCS8AndRejectsTheRest(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	rsaKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	pkcs1 := filepath.Join(dir, "pkcs1.pem")
	writeFile(t, pkcs1, pemBlock("RSA PRIVATE KEY", x509.MarshalPKCS1PrivateKey(rsaKey)))
	if _, err = loadKey(pkcs1); err != nil {
		t.Fatal(err)
	}
	pkcs8Bytes, err := x509.MarshalPKCS8PrivateKey(rsaKey)
	if err != nil {
		t.Fatal(err)
	}
	pkcs8 := filepath.Join(dir, "pkcs8.pem")
	writeFile(t, pkcs8, pemBlock("PRIVATE KEY", pkcs8Bytes))
	if _, err = loadKey(pkcs8); err != nil {
		t.Fatal(err)
	}
	ec, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	ecBytes, err := x509.MarshalPKCS8PrivateKey(ec)
	if err != nil {
		t.Fatal(err)
	}
	ecPath := filepath.Join(dir, "ec.pem")
	writeFile(t, ecPath, pemBlock("PRIVATE KEY", ecBytes))
	if _, err = loadKey(ecPath); err == nil || !strings.Contains(cliText(err), "not RSA") {
		t.Fatalf("ec: %v", err)
	}
	bad := filepath.Join(dir, "bad.pem")
	writeFile(t, bad, "not pem\n")
	if _, err = loadKey(bad); err == nil || !strings.Contains(cliText(err), "not PEM") {
		t.Fatalf("pem: %v", err)
	}
	junk := filepath.Join(dir, "junk.pem")
	writeFile(t, junk, pemBlock("PRIVATE KEY", []byte("nope")))
	if _, err = loadKey(junk); err == nil || !strings.Contains(cliText(err), "not PKCS1 or PKCS8") {
		t.Fatalf("junk: %v", err)
	}
	if _, err = loadKey(
		filepath.Join(dir, "missing.pem"),
	); err == nil ||
		!strings.Contains(err.Error(), "read verifier key") {
		t.Fatalf("missing: %v", err)
	}
}

func TestStatusAuth_fallsBackSignsAndReportsTokenFailures(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	env := f.Env(t)
	env.Home = t.TempDir()
	got, err := env.statusAuth(context.Background(), "")
	if err != nil || got != "token tok" {
		t.Fatalf("fallback: %q %v", got, err)
	}
	blocked := filepath.Join(env.Home, "not-a-dir")
	writeFile(t, blocked, "x")
	env.Home = blocked
	_, err = env.statusAuth(context.Background(), "")
	if err == nil || !strings.Contains(err.Error(), "verifier key") {
		t.Fatalf("stat: %v", err)
	}

	key := writeRSAKey(t, f)
	f.hub.on("POST /app/installations/100/access_tokens", `{"token":"ghs_test"}`)
	got, err = env.statusAuth(context.Background(), key)
	if err != nil || got != "token ghs_test" {
		t.Fatalf("signed: %q %v", got, err)
	}
	auth := f.hub.authOf("POST /app/installations/100/access_tokens")
	if !strings.HasPrefix(auth, "Bearer ") || strings.Count(auth, ".") != 2 {
		t.Fatalf("jwt=%q", auth)
	}

	f.hub.on("POST /app/installations/100/access_tokens", `{"token":""}`)
	_, err = env.statusAuth(context.Background(), key)
	if err == nil || !strings.Contains(cliText(err), "installation token was empty") {
		t.Fatalf("empty: %v", err)
	}
	f.hub.on("POST /app/installations/100/access_tokens", "")
	_, err = env.statusAuth(context.Background(), key)
	if err == nil || !strings.Contains(err.Error(), "access_tokens") {
		t.Fatalf("post: %v", err)
	}

	env.Home = ""
	_, err = env.statusAuth(context.Background(), "")
	if err == nil || !strings.Contains(cliText(err), "HOME is unset") {
		t.Fatalf("home: %v", err)
	}
	short := writeShortKey(t, f)
	_, err = env.statusAuth(context.Background(), short)
	if err == nil || !strings.Contains(err.Error(), "sign verifier jwt") {
		t.Fatalf("short: %v", err)
	}
	_, err = env.statusAuth(context.Background(), t.TempDir())
	if err == nil || !strings.Contains(err.Error(), "read verifier key") {
		t.Fatalf("read: %v", err)
	}
	f.env = []string{f.env[0]}
	f.run = func(ctx context.Context, dir, stdin, name string, args ...string) ([]byte, error) {
		if name == "gh" {
			return nil, fmt.Errorf("gh: not logged in")
		}
		return Exec(ctx, dir, stdin, name, args...)
	}
	bare := f.Env(t)
	bare.Home = t.TempDir()
	_, err = bare.statusAuth(context.Background(), "")
	if err == nil || !strings.Contains(err.Error(), "not logged in") {
		t.Fatalf("token: %v", err)
	}
}

func writeRSAKey(t *testing.T, f *fixture) string {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(f.home, "verifier.pem")
	writeFile(t, path, pemBlock("RSA PRIVATE KEY", x509.MarshalPKCS1PrivateKey(key)))
	return path
}

func writeShortKey(t *testing.T, f *fixture) string {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 256)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(f.home, "short.pem")
	writeFile(t, path, pemBlock("RSA PRIVATE KEY", x509.MarshalPKCS1PrivateKey(key)))
	return path
}
