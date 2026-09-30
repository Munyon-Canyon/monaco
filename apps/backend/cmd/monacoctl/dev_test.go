package main

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/platform/auth"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/privy"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/config"
)

func devConfig(env config.Env) config.Config {
	return config.Config{Env: env, Auth: config.Auth{DevTokenKey: "test-key"}}
}

func TestDevToken_mintsATokenTheDevVerifierAccepts(t *testing.T) {
	t.Parallel()
	var stdout, stderr bytes.Buffer
	if code := devCmd(devConfig(config.EnvLocal), []string{"token", "--user", "u-42", "--ttl", "1h"}, &stdout,
		&stderr); code != 0 || stderr.Len() != 0 {
		t.Fatalf("exit %d, stderr %q", code, stderr.String())
	}
	verifier, err := auth.NewDevVerifier(devConfig(config.EnvLocal), clock.Real{})
	if err != nil {
		t.Fatal(err)
	}
	actor, err := verifier.Verify(t.Context(), strings.TrimSuffix(stdout.String(), "\n"))
	if err != nil || actor != (auth.Actor{Kind: auth.ActorUser, ID: "u-42", Standing: auth.StandingActive}) {
		t.Fatalf("Verify = %+v, %v", actor, err)
	}
	wrongKey, err := auth.NewDevVerifier(config.Config{Env: config.EnvLocal, Auth: config.Auth{DevTokenKey: "other"}},
		clock.Real{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := wrongKey.Verify(t.Context(), strings.TrimSpace(stdout.String())); err == nil {
		t.Fatal("a verifier with another key accepted the token")
	}
}

func TestDevToken_refusesProductionAndBadArguments(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct {
		cfg  config.Config
		args []string
		code int
		want string
	}{
		"production":   {devConfig(config.EnvProduction), []string{"token", "--user", "u"}, 1, "monacoctl dev token: auth.NewDevVerifier: invalid_input\n"},
		"no key":       {config.Config{Env: config.EnvLocal}, []string{"token", "--user", "u"}, 1, "monacoctl dev token: auth.NewDevVerifier: invalid_input\n"},
		"no user":      {devConfig(config.EnvLocal), []string{"token"}, 2, devUsage + "\n"},
		"zero ttl":     {devConfig(config.EnvLocal), []string{"token", "--user", "u", "--ttl", "0s"}, 2, devUsage + "\n"},
		"extra arg":    {devConfig(config.EnvLocal), []string{"token", "--user", "u", "x"}, 2, devUsage + "\n"},
		"bad flag":     {devConfig(config.EnvLocal), []string{"token", "--nope"}, 2, devUsage + "\n"},
		"unknown verb": {devConfig(config.EnvLocal), []string{"mint"}, 2, devUsage + "\n"},
		"no verb":      {devConfig(config.EnvLocal), nil, 2, devUsage + "\n"},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			var stdout, stderr bytes.Buffer
			code := devCmd(tc.cfg, tc.args, &stdout, &stderr)
			if code != tc.code || stderr.String() != tc.want || stdout.Len() != 0 {
				t.Fatalf("exit %d stderr %q stdout %q, want %d %q", code, stderr.String(), stdout.String(), tc.code,
					tc.want)
			}
		})
	}
}

func TestDevToken_defaultTTLIsADay(t *testing.T) {
	t.Parallel()
	var stdout, stderr bytes.Buffer
	if code := devCmd(devConfig(config.EnvLocal), []string{"token", "--user", "u"}, &stdout, &stderr); code != 0 {
		t.Fatalf("exit %d, stderr %q", code, stderr.String())
	}
	later := testClock{at: time.Now().Add(23 * time.Hour)}
	verifier, err := auth.NewDevVerifier(devConfig(config.EnvLocal), later)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := verifier.Verify(t.Context(), strings.TrimSpace(stdout.String())); err != nil {
		t.Fatalf("token expired before 24h: %v", err)
	}
	expired, _ := auth.NewDevVerifier(devConfig(config.EnvLocal), testClock{at: time.Now().Add(25 * time.Hour)})
	if _, err := expired.Verify(t.Context(), strings.TrimSpace(stdout.String())); err == nil {
		t.Fatal("token still valid after 25h")
	}
}

type testClock struct {
	clock.Real
	at time.Time
}

func (c testClock) Now() time.Time { return c.at }

func TestDevPrivyToken_mintsAnHourLongTokenThatThePrintedKeyVerifies(t *testing.T) {
	t.Parallel()
	cfg := devConfig(config.EnvLocal)
	cfg.Privy.AppID = "app-local"
	var key, token, stderr bytes.Buffer
	if code := devCmd(
		cfg,
		[]string{"privy-token", "--print-public-key"},
		&key,
		&stderr,
	); code != 0 ||
		stderr.Len() != 0 {
		t.Fatalf("--print-public-key exit %d, stderr %q", code, stderr.String())
	}
	if code := devCmd(cfg, []string{"privy-token", "--sub", "did:privy:qa-1"}, &token, &stderr); code != 0 ||
		stderr.Len() != 0 {
		t.Fatalf("--sub exit %d, stderr %q", code, stderr.String())
	}
	verifyAt := func(at time.Time) (privy.UserID, error) {
		c := privy.New(config.Config{
			Privy: config.Privy{
				AppID: "app-local", BaseURL: "http://privy.test", VerificationKey: strings.TrimSpace(key.String()),
			},
			Timeouts: config.Timeouts{Privy: time.Second},
		}, testClock{at: at})
		return c.VerifyAccessToken(t.Context(), strings.TrimSpace(token.String()))
	}
	if sub, err := verifyAt(time.Now().Add(59 * time.Minute)); err != nil || sub != "did:privy:qa-1" {
		t.Fatalf("VerifyAccessToken after 59m = %q, %v", sub, err)
	}
	if _, err := verifyAt(time.Now().Add(61 * time.Minute)); err == nil {
		t.Fatal("token still valid after 61m")
	}
}

func TestDevPrivyToken_refusesProductionAndBadArguments(t *testing.T) {
	t.Parallel()
	refused := "monacoctl dev privy-token: refused with MONACO_ENV=production\n"
	for name, tc := range map[string]struct {
		cfg  config.Config
		args []string
		code int
		want string
	}{
		"production token": {devConfig(config.EnvProduction), []string{"privy-token", "--sub", "did:privy:x"}, 1, refused},
		"production key":   {devConfig(config.EnvProduction), []string{"privy-token", "--print-public-key"}, 1, refused},
		"neither":          {devConfig(config.EnvLocal), []string{"privy-token"}, 2, devUsage + "\n"},
		"both": {
			devConfig(config.EnvLocal), []string{"privy-token", "--sub", "x", "--print-public-key"}, 2, devUsage + "\n",
		},
		"extra arg": {devConfig(config.EnvLocal), []string{"privy-token", "--sub", "x", "y"}, 2, devUsage + "\n"},
		"bad flag":  {devConfig(config.EnvLocal), []string{"privy-token", "--user", "x"}, 2, devUsage + "\n"},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			var stdout, stderr bytes.Buffer
			code := devCmd(tc.cfg, tc.args, &stdout, &stderr)
			if code != tc.code || stderr.String() != tc.want || stdout.Len() != 0 {
				t.Fatalf("exit %d stderr %q stdout %q, want %d %q", code, stderr.String(), stdout.String(), tc.code,
					tc.want)
			}
		})
	}
}
