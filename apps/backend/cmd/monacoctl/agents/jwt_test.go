//go:debug rsa1024min=0

package agents

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestSignJWT_rejectsAKeyShorterThanTheDigest(t *testing.T) {
	t.Parallel()
	k, err := rsa.GenerateKey(rand.Reader, 256)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := signJWT(
		k,
		"99",
		time.Unix(1_700_000_000, 0),
	); err == nil ||
		!strings.Contains(err.Error(), "sign verifier jwt") {
		t.Fatalf("err=%v", err)
	}
	f := newFixture(t)
	der := x509.MarshalPKCS1PrivateKey(k)
	path := filepath.Join(f.home, "tiny.pem")
	writeFile(t, path, pemBlock("RSA PRIVATE KEY", der))
	sha := strings.Repeat("f", 40)
	f.owner(t, Record{Ticket: 40, Model: opus, State: Running})
	head := pr(5, "h", "fb-checkpoint-1", "Part of #40")
	head.Head.SHA = sha
	f.hub.on(get("/pulls/5"), head)
	f.hub.on(list("/pulls/5/files?"), []File{{Filename: "a.go", Additions: 1}})
	if code, _, stderr := f.agents(
		t,
		"verdict",
		"pass",
		"5",
		sha,
		"--kind",
		"light",
		"--model",
		"sonnet",
		"--report",
		f.report(t, "ok"),
		"--key",
		path,
	); code != 1 ||
		!strings.Contains(stderr, "sign verifier jwt") {
		t.Fatalf("code=%d stderr=%q", code, stderr)
	}
}
