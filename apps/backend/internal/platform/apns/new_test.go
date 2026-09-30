package apns_test

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/platform/apns"
	"github.com/monaco/monaco/apps/backend/internal/platform/config"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

func TestNew_refusesAConfigItCannotSendWith(t *testing.T) {
	t.Parallel()
	const keyMaterial = "garbage-key-material-not-a-pem"
	tests := map[string]struct {
		edit   func(*config.Config)
		reason string
	}{
		"key that is not a PEM": {
			func(c *config.Config) { c.APNs.KeyP8 = keyMaterial },
			"APNS_KEY_P8",
		},
		"key that cannot sign es256": {
			func(c *config.Config) { c.APNs.KeyP8 = p384KeyPEM(t) },
			"APNS_KEY_P8",
		},
		"base url over cleartext to a remote host": {
			func(c *config.Config) { c.APNs.BaseURL = "http://fakes.internal/apns" },
			"APNS_BASE_URL",
		},
		"no timeout": {
			func(c *config.Config) { c.Timeouts.APNs = 0 },
			"MONACO_TIMEOUT_APNS",
		},
		"base url without a scheme": {
			func(c *config.Config) { c.APNs.BaseURL = "localhost:8099" },
			"APNS_BASE_URL",
		},
		"base url that is not http": {
			func(c *config.Config) { c.APNs.BaseURL = "ftp://fakes.test/apns" },
			"APNS_BASE_URL",
		},
		"base url without a host": {
			func(c *config.Config) { c.APNs.BaseURL = "http://" },
			"APNS_BASE_URL",
		},
		"base url that does not parse": {
			func(c *config.Config) { c.APNs.BaseURL = "http://%zz" },
			"APNS_BASE_URL",
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			cfg := testConfig()
			tt.edit(&cfg)

			c, err := apns.New(cfg)

			wantCode(t, err, errs.CodeInvalidConfig)
			if c != nil || !strings.Contains(fmt.Sprint(errs.Detail(err)), tt.reason) {
				t.Fatalf("New = %v, %v, want no client and an error that names %s", c, err, tt.reason)
			}
			if strings.Contains(err.Error(), keyMaterial) ||
				strings.Contains(fmt.Sprint(errs.Detail(err)), keyMaterial) {
				t.Fatalf("error %v echoes the key material", err)
			}
		})
	}
}

func TestNew_withTimeoutStandsInForAMissingConfiguredDeadline(t *testing.T) {
	t.Parallel()
	cfg := testConfig()
	cfg.Timeouts.APNs = 0
	if _, err := apns.New(cfg, apns.WithTimeout(time.Second)); err != nil {
		t.Fatal(err)
	}
}

func TestNew_readsAKeyWrittenOnOneLineWithLiteralNewlines(t *testing.T) {
	t.Parallel()
	cfg := testConfig()
	cfg.APNs.KeyP8 = strings.ReplaceAll(strings.TrimSpace(testkit.APNsKeyP8()), "\n", `\n`)
	u := &upstream{replies: []reply{accepted()}}
	c, err := apns.New(cfg, apns.WithTransport(u))
	if err != nil {
		t.Fatal(err)
	}

	mustSend(t, c, push(apns.Sandbox))

	verifyBearer(t, u.requests()[0].header.Get("Authorization"))
}

func p384KeyPEM(t *testing.T) string {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P384(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	der, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}))
}

func TestNew_acceptsABaseURLThatIsHTTPSOrOnALoopbackHost(t *testing.T) {
	t.Parallel()
	for _, base := range []string{
		"https://push.example.test/apns",
		"http://127.0.0.1:8099/apns",
		"http://localhost:8099/apns",
		"http://[::1]:8099/apns",
	} {
		t.Run(base, func(t *testing.T) {
			t.Parallel()
			cfg := testConfig()
			cfg.APNs.BaseURL = base
			if _, err := apns.New(cfg); err != nil {
				t.Fatal(err)
			}
		})
	}
}
