package privy_test

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"strings"
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/privy"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/config"
	"github.com/monaco/monaco/apps/backend/internal/testkit/fakes"
)

func fixtureKeyParts() (header, body, footer string) {
	lines := strings.Split(strings.TrimSpace(fakes.PrivyVerificationKey()), "\n")
	return lines[0], strings.Join(lines[1:len(lines)-1], ""), lines[len(lines)-1]
}

func fixtureKeyForms() map[string]string {
	canonical := fakes.PrivyVerificationKey()
	header, body, footer := fixtureKeyParts()
	wrapped := func(width int) string {
		var rows []string
		for rest := body; rest != ""; {
			n := min(width, len(rest))
			rows, rest = append(rows, rest[:n]), rest[n:]
		}
		return header + "\n" + strings.Join(rows, "\n") + "\n" + footer + "\n"
	}
	return map[string]string{
		"as the fakes print it":     canonical,
		"one line with literal \\n": strings.ReplaceAll(canonical, "\n", `\n`),
		"body on one line":          wrapped(len(body)),
		"wrapped at 16 columns":     wrapped(16),
		"wrapped at 76 columns":     wrapped(76),
		"CRLF line ends":            strings.ReplaceAll(canonical, "\n", "\r\n"),
		"blank lines around it":     "\n\n" + canonical + "\n\n",
		"no final newline":          strings.TrimSpace(canonical),
	}
}

func TestCheckVerificationKey_refusesEveryFormOfTheFixtureKeyInStagingAndProduction(t *testing.T) {
	t.Parallel()
	token := fakes.PrivyAccessToken(appID, "did:privy:member", clock.Real{}.Now(), time.Hour)
	for name, key := range fixtureKeyForms() {
		for _, env := range []config.Env{config.EnvStaging, config.EnvProduction} {
			t.Run(name+" in "+string(env), func(t *testing.T) {
				t.Parallel()
				cfg := testConfig()
				cfg.Env, cfg.Privy.VerificationKey = env, key
				if _, err := clientWith(cfg, nil, clock.Real{}).VerifyAccessToken(t.Context(), token); err != nil {
					t.Fatalf("the verifier rejects this form of the fixture key, so the case proves nothing: %v", err)
				}
				wantCode(t, privy.CheckVerificationKey(cfg), errs.CodeInvalidConfig)
			})
		}
	}
}

func TestCheckVerificationKey_leavesEveryOtherKeyAndEnvAlone(t *testing.T) {
	t.Parallel()
	edPublic, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	edDER, err := x509.MarshalPKIXPublicKey(edPublic)
	if err != nil {
		t.Fatal(err)
	}
	fixture, other := fakes.PrivyVerificationKey(), fakes.OtherPrivyVerificationKey()
	for name, tc := range map[string]struct {
		env config.Env
		key string
	}{
		"fixture key in local":          {config.EnvLocal, fixture},
		"fixture key in test":           {config.EnvTest, fixture},
		"fixture key with no env":       {"", fixture},
		"another key in staging":        {config.EnvStaging, other},
		"another key in production":     {config.EnvProduction, other},
		"no key in production":          {config.EnvProduction, ""},
		"a malformed key in production": {config.EnvProduction, "-----BEGIN PUBLIC KEY-----\nAAAA\n-----END PUBLIC KEY-----\n"},
		"an ed25519 key in production": {
			config.EnvProduction, string(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: edDER})),
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			cfg := testConfig()
			cfg.Env, cfg.Privy.VerificationKey = tc.env, tc.key
			if err := privy.CheckVerificationKey(cfg); err != nil {
				t.Fatalf("CheckVerificationKey = %v, want nil", err)
			}
		})
	}
}

func TestCheckVerificationKey_refusalNamesTheVariableAndCarriesNoKeyMaterial(t *testing.T) {
	t.Parallel()
	cfg := testConfig()
	cfg.Env = config.EnvProduction
	err := privy.CheckVerificationKey(cfg)
	wantCode(t, err, errs.CodeInvalidConfig)
	var shown strings.Builder
	shown.WriteString(err.Error())
	for _, attr := range errs.Detail(err) {
		shown.WriteString("\n" + attr.Key + "\n" + attr.Value.String())
	}
	text := shown.String()
	for _, want := range []string{"PRIVY_VERIFICATION_KEY", "tests only"} {
		if !strings.Contains(text, want) {
			t.Errorf("refusal %q does not say %q", text, want)
		}
	}
	const window = 16
	_, body, _ := fixtureKeyParts()
	for i := 0; i+window <= len(body); i++ {
		if strings.Contains(text, body[i:i+window]) {
			t.Fatalf("refusal %q carries part of the key: %q", text, body[i:i+window])
		}
	}
}
