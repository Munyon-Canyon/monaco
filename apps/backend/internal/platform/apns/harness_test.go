package apns_test

import (
	"crypto/ecdsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"io"
	"math/big"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/platform/apns"
	"github.com/monaco/monaco/apps/backend/internal/platform/config"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

const (
	testToken = "a1b2c3d4e5f60718293a4b5c6d7e8f9001122334455667788990aabbccddeeff"
	keyID     = "ABC123DEFG"
	teamID    = "TEAM123456"
	topic     = "com.monaco.app"
	apnsID    = "eabeae54-14a8-11e5-b60b-1697f925ec7b"
)

type reply func(*http.Request) (*http.Response, error)

type sent struct {
	url    *url.URL
	header http.Header
	body   []byte
}

type upstream struct {
	mu      sync.Mutex
	replies []reply
	sent    []sent
}

func (u *upstream) RoundTrip(r *http.Request) (*http.Response, error) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, err
	}
	u.mu.Lock()
	n := len(u.sent)
	u.sent = append(u.sent, sent{url: r.URL, header: r.Header.Clone(), body: body})
	next := u.replies[min(n, len(u.replies)-1)]
	u.mu.Unlock()
	return next(r)
}

func (u *upstream) requests() []sent {
	u.mu.Lock()
	defer u.mu.Unlock()
	return append([]sent(nil), u.sent...)
}

func (u *upstream) count() int { return len(u.requests()) }

func respond(status int, body string, header ...string) reply {
	return func(r *http.Request) (*http.Response, error) {
		h := http.Header{}
		for i := 0; i+1 < len(header); i += 2 {
			h.Set(header[i], header[i+1])
		}
		return &http.Response{
			StatusCode: status, Status: http.StatusText(status), Header: h,
			Body: io.NopCloser(strings.NewReader(body)), Request: r,
		}, nil
	}
}

func accepted() reply { return respond(http.StatusOK, "", "apns-id", apnsID) }

func rejected(status int, reason string) reply {
	return respond(status, `{"reason":"`+reason+`"}`)
}

func unreachable(*http.Request) (*http.Response, error) {
	return nil, errs.New(errs.CodeInternal, "test.dial")
}

func hang(r *http.Request) (*http.Response, error) {
	<-r.Context().Done()
	return nil, r.Context().Err()
}

func testConfig() config.Config {
	return config.Config{
		Env:      config.EnvTest,
		APNs:     config.APNs{KeyP8: testkit.APNsKeyP8(), KeyID: keyID, TeamID: teamID, Topic: topic},
		Timeouts: config.Timeouts{APNs: 10 * time.Second},
	}
}

func newClient(t *testing.T, u *upstream, opts ...apns.Option) *apns.Client {
	t.Helper()
	c, err := apns.New(testConfig(), append([]apns.Option{apns.WithTransport(u)}, opts...)...)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func push(env apns.Environment) apns.Push {
	return apns.Push{
		Token:       testToken,
		Environment: env,
		CollapseID:  "trade-42",
		Title:       "Trade filled",
		Body:        "Your cabal bought $50.00 of Tesla",
		Data:        map[string]string{"cabal_id": "c1", "txn_id": "42"},
	}
}

func mustSend(t *testing.T, c *apns.Client, p apns.Push) apns.Result {
	t.Helper()
	res, err := c.Send(t.Context(), p)
	if err != nil {
		t.Fatalf("Send = %v, want an answer", err)
	}
	return res
}

func wantCode(t *testing.T, err error, code errs.Code) {
	t.Helper()
	if errs.CodeOf(err) != code {
		t.Fatalf("err = %v, want %s", err, code)
	}
}

func verifyBearer(t *testing.T, authorization string) {
	t.Helper()
	raw, ok := strings.CutPrefix(authorization, "bearer ")
	parts := strings.Split(raw, ".")
	if !ok || len(parts) != 3 {
		t.Fatalf("authorization = %q, want a bearer JWT", authorization)
	}
	var header struct{ Alg, Kid string }
	var claims struct {
		Iss string
		Iat int64
	}
	decodePart(t, parts[0], &header)
	decodePart(t, parts[1], &claims)
	if header.Alg != "ES256" || header.Kid != keyID || claims.Iss != teamID || claims.Iat == 0 {
		t.Fatalf("JWT header %+v claims %+v, want ES256 with kid %s and iss %s", header, claims, keyID, teamID)
	}
	sig, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil || len(sig) != 64 {
		t.Fatalf("JWT signature %q is not a 64 byte ES256 signature: %v", parts[2], err)
	}
	block, _ := pem.Decode([]byte(testkit.APNsKeyP8()))
	parsed, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		t.Fatal(err)
	}
	key, ok := parsed.(*ecdsa.PrivateKey)
	if !ok {
		t.Fatalf("test key is %T, want an EC key", parsed)
	}
	digest := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	if !ecdsa.Verify(&key.PublicKey, digest[:], new(big.Int).SetBytes(sig[:32]), new(big.Int).SetBytes(sig[32:])) {
		t.Fatal("JWT signature does not verify against the test key")
	}
}

func decodePart(t *testing.T, part string, into any) {
	t.Helper()
	raw, err := base64.RawURLEncoding.DecodeString(part)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, into); err != nil {
		t.Fatal(err)
	}
}
