package auth

import (
	"encoding/base64"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/platform/config"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

func devVerifier(t *testing.T, key string, now time.Time) *DevVerifier {
	t.Helper()
	cfg := config.Config{Env: config.EnvTest, Auth: config.Auth{DevTokenKey: key}}
	v, err := NewDevVerifier(cfg, testkit.NewClock(now))
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func reason(err error) string {
	var e *errs.Error
	if !errors.As(err, &e) {
		return ""
	}
	for _, a := range e.Attrs {
		if a.Key == "reason" {
			return a.Value.String()
		}
	}
	return ""
}

func TestDevVerifier_acceptsItsOwnTokenUntilExpiry(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	v := devVerifier(t, "k1", now)
	token := v.Mint("u-1", now.Add(time.Minute))
	if strings.Count(token, ".") != 2 || !strings.HasPrefix(token, "eyJ") {
		t.Fatalf("token %q is not a compact JWT", token)
	}
	actor, err := v.Verify(t.Context(), token)
	if err != nil || actor != (Actor{Kind: ActorUser, ID: "u-1", Standing: StandingActive}) {
		t.Fatalf("Verify = %+v, %v", actor, err)
	}
	late := devVerifier(t, "k1", now.Add(time.Minute))
	if _, err := late.Verify(
		t.Context(),
		token,
	); errs.CodeOf(err) != errs.CodeUnauthorized ||
		reason(err) != "expired" {
		t.Fatalf("Verify at exp = %v, want unauthorized expired", err)
	}
}

func TestDevVerifier_rejectsEveryForgery(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	v := devVerifier(t, "k1", now)
	other := devVerifier(t, "k2", now)
	valid := v.Mint("u-1", now.Add(time.Hour))
	swap := func(token string, part int, with string) string {
		parts := strings.Split(token, ".")
		parts[part] = with
		return strings.Join(parts, ".")
	}
	seg := func(s string) string { return base64.RawURLEncoding.EncodeToString([]byte(s)) }
	resign := func(signing string) string { return signing + "." + seg(string(v.sign(signing))) }
	for name, tc := range map[string]struct{ token, reason string }{
		"wrong key":          {other.Mint("u-1", now.Add(time.Hour)), "bad_signature"},
		"tampered claims":    {swap(valid, 1, seg(`{"sub":"admin","exp":4102444800}`)), "bad_signature"},
		"tampered signature": {swap(valid, 2, seg("nope")), "bad_signature"},
		"alg none":           {swap(valid, 0, seg(`{"alg":"none","typ":"JWT"}`)), "unsupported_header"},
		"alg RS256":          {swap(valid, 0, seg(`{"alg":"RS256","typ":"JWT"}`)), "unsupported_header"},
		"bad base64 header":  {"!.!.!", "unsupported_header"},
		"bad base64 payload": {resign(seg(hs256Header) + ".!!!"), "malformed"},
		"bad base64 sig":     {swap(valid, 2, "!!!"), "bad_signature"},
		"not a jwt":          {"opaque", "malformed"},
		"four parts":         {valid + ".x", "malformed"},
		"empty":              {"", "malformed"},
		"claims not json":    {resign(seg(hs256Header) + "." + seg("sub=u")), "no_subject"},
		"no subject":         {v.Mint("", now.Add(time.Hour)), "no_subject"},
		"expired":            {v.Mint("u-1", now.Add(-time.Second)), "expired"},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			actor, err := v.Verify(t.Context(), tc.token)
			if errs.CodeOf(err) != errs.CodeUnauthorized || reason(err) != tc.reason || actor != (Actor{}) {
				t.Fatalf("Verify(%q) = %+v, %v, want unauthorized %s", tc.token, actor, err, tc.reason)
			}
			if strings.Contains(err.Error(), tc.token) && tc.token != "" {
				t.Fatalf("error %q carries the token", err)
			}
		})
	}
}

func TestCheckDevTokenKey_refusesThePlaceholderWhenDeployed(t *testing.T) {
	t.Parallel()
	const want = "change-me-dev-only"
	if PlaceholderDevTokenKey != want {
		t.Fatalf("PlaceholderDevTokenKey = %q", PlaceholderDevTokenKey)
	}
	for _, env := range []config.Env{config.EnvStaging, config.EnvProduction} {
		t.Run(string(env), func(t *testing.T) {
			t.Parallel()
			cfg := config.Config{Env: env, Auth: config.Auth{DevTokenKey: PlaceholderDevTokenKey}}
			err := CheckDevTokenKey(cfg)
			if errs.CodeOf(err) != errs.CodeInvalidConfig || reason(err) != "placeholder dev token key" ||
				!strings.HasPrefix(err.Error(), "auth.CheckDevTokenKey: ") ||
				strings.Contains(err.Error(), PlaceholderDevTokenKey) {
				t.Fatalf("CheckDevTokenKey = %v, want invalid_config placeholder dev token key", err)
			}
		})
	}
}

func TestCheckDevTokenKey_leavesOtherKeysAndUndeployedEnvsAlone(t *testing.T) {
	t.Parallel()
	cases := []config.Config{
		{Env: config.EnvStaging, Auth: config.Auth{DevTokenKey: "another-key"}},
		{Env: config.EnvProduction, Auth: config.Auth{DevTokenKey: "another-key"}},
		{Env: config.EnvStaging},
		{Env: config.EnvProduction},
		{Env: config.EnvLocal, Auth: config.Auth{DevTokenKey: PlaceholderDevTokenKey}},
		{Env: config.EnvTest, Auth: config.Auth{DevTokenKey: PlaceholderDevTokenKey}},
		{Auth: config.Auth{DevTokenKey: PlaceholderDevTokenKey}},
		{Env: config.EnvLocal, Auth: config.Auth{DevTokenKey: "another-key"}},
		{Env: config.EnvTest, Auth: config.Auth{DevTokenKey: "another-key"}},
		{Env: config.EnvLocal},
		{Env: config.EnvTest},
		{},
	}
	for _, cfg := range cases {
		t.Run(string(cfg.Env)+"/"+keyName(cfg.Auth.DevTokenKey), func(t *testing.T) {
			t.Parallel()
			if err := CheckDevTokenKey(cfg); err != nil {
				t.Fatalf("CheckDevTokenKey = %v, want nil", err)
			}
		})
	}
}

func keyName(key string) string {
	switch key {
	case PlaceholderDevTokenKey:
		return "placeholder"
	case "":
		return "empty"
	default:
		return "other"
	}
}

func TestNewDevVerifier_refusesProductionAndAnEmptyKey(t *testing.T) {
	t.Parallel()
	for name, cfg := range map[string]config.Config{
		"production": {Env: config.EnvProduction, Auth: config.Auth{DevTokenKey: "k"}},
		"empty key":  {Env: config.EnvLocal},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			v, err := NewDevVerifier(cfg, testkit.NewClock(time.Time{}))
			if v != nil || errs.CodeOf(err) != errs.CodeInvalidInput {
				t.Fatalf("NewDevVerifier = %v, %v, want invalid_input", v, err)
			}
		})
	}
	for _, env := range []config.Env{config.EnvLocal, config.EnvTest, config.EnvStaging} {
		cfg := config.Config{Env: env, Auth: config.Auth{DevTokenKey: "k"}}
		if _, err := NewDevVerifier(cfg, testkit.NewClock(time.Time{})); err != nil {
			t.Fatalf("%s: %v", env, err)
		}
	}
}
