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
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

const (
	devUserV7          = "01890a5d-ac96-774b-bcce-b302099a8057"
	devUserV4          = "01890a5d-ac96-474b-bcce-b302099a8057"
	devVerifierRefused = "monacoctl dev token: auth.NewDevVerifier: invalid_input\n"
)

func devConfig(env config.Env) config.Config {
	return config.Config{Env: env, Auth: config.Auth{DevTokenKey: "test-key"}}
}

func TestDevToken_mintsATokenTheDevVerifierAccepts(t *testing.T) {
	t.Parallel()
	var stdout, stderr bytes.Buffer
	if code := devCmd(devConfig(config.EnvLocal), []string{"token", "--user", devUserV7, "--ttl", "1h"}, &stdout,
		&stderr); code != 0 || stderr.Len() != 0 {
		t.Fatalf("exit %d, stderr %q", code, stderr.String())
	}
	verifier, err := auth.NewDevVerifier(devConfig(config.EnvLocal), clock.Real{})
	if err != nil {
		t.Fatal(err)
	}
	actor, err := verifier.Verify(t.Context(), strings.TrimSuffix(stdout.String(), "\n"))
	if err != nil || actor != (auth.Actor{Kind: auth.ActorUser, ID: devUserV7, Standing: auth.StandingActive}) {
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
		"production": {devConfig(config.EnvProduction), []string{"token", "--user", devUserV7}, 1, devVerifierRefused},
		"no key":     {config.Config{Env: config.EnvLocal}, []string{"token", "--user", devUserV7}, 1, devVerifierRefused},
		"no user":    {devConfig(config.EnvLocal), []string{"token"}, 2, devUsage + "\n"},
		"zero ttl": {
			devConfig(config.EnvLocal), []string{"token", "--user", devUserV7, "--ttl", "0s"}, 2, devUsage + "\n",
		},
		"extra arg":    {devConfig(config.EnvLocal), []string{"token", "--user", devUserV7, "x"}, 2, devUsage + "\n"},
		"version 4":    {devConfig(config.EnvLocal), []string{"token", "--user", devUserV4}, 2, devUserSubjectLine + "\n"},
		"alice":        {devConfig(config.EnvLocal), []string{"token", "--user", "alice"}, 2, devUserSubjectLine + "\n"},
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
	if code := devCmd(devConfig(config.EnvLocal), []string{"token", "--user", devUserV7}, &stdout, &stderr); code != 0 {
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

func TestDevToken_newUserPrintsTheTokenThenTheID(t *testing.T) {
	t.Parallel()
	var stdout, stderr bytes.Buffer
	if code := devCmd(devConfig(config.EnvLocal), []string{"token", "--new-user"}, &stdout, &stderr); code != 0 ||
		stderr.Len() != 0 {
		t.Fatalf("exit %d, stderr %q", code, stderr.String())
	}
	lines := strings.Split(strings.TrimSuffix(stdout.String(), "\n"), "\n")
	if len(lines) != 2 {
		t.Fatalf("stdout = %q, want a token and a user id", stdout.String())
	}
	if _, err := ids.ParseUserID(lines[1]); err != nil {
		t.Fatalf("user id %q: %v", lines[1], err)
	}
	verifier, err := auth.NewDevVerifier(devConfig(config.EnvLocal), clock.Real{})
	if err != nil {
		t.Fatal(err)
	}
	actor, err := verifier.Verify(t.Context(), lines[0])
	if err != nil || actor != (auth.Actor{Kind: auth.ActorUser, ID: lines[1], Standing: auth.StandingActive}) {
		t.Fatalf("Verify = %+v, %v", actor, err)
	}
}

func TestDevToken_userAndNewUserPrintsUsage(t *testing.T) {
	t.Parallel()
	var stdout, stderr bytes.Buffer
	code := devCmd(devConfig(config.EnvLocal), []string{"token", "--user", devUserV7, "--new-user"}, &stdout, &stderr)
	if code != 2 || stderr.String() != devUsage+"\n" || stdout.Len() != 0 {
		t.Fatalf("exit %d stderr %q stdout %q", code, stderr.String(), stdout.String())
	}
}

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
		c, err := privy.New(config.Config{
			Privy: config.Privy{
				AppID: "app-local", BaseURL: "http://privy.test", VerificationKey: strings.TrimSpace(key.String()),
			},
			Timeouts: config.Timeouts{Privy: time.Second},
		}, testClock{at: at})
		if err != nil {
			return "", err
		}
		return c.VerifyAccessToken(t.Context(), strings.TrimSpace(token.String()))
	}
	if sub, err := verifyAt(time.Now().Add(59 * time.Minute)); err != nil || sub != "did:privy:qa-1" {
		t.Fatalf("VerifyAccessToken after 59m = %q, %v", sub, err)
	}
	if _, err := verifyAt(time.Now().Add(61 * time.Minute)); err == nil {
		t.Fatal("token still valid after 61m")
	}
}

func TestDevPrivyToken_refusesDeployedEnvsAndBadArguments(t *testing.T) {
	t.Parallel()
	refused := func(env config.Env) string {
		return "monacoctl dev privy-token: refused with MONACO_ENV=" + string(env) + "\n"
	}
	staging, production := devConfig(config.EnvStaging), devConfig(config.EnvProduction)
	for name, tc := range map[string]struct {
		cfg  config.Config
		args []string
		code int
		want string
	}{
		"staging token":    {staging, []string{"privy-token", "--sub", "did:privy:x"}, 1, refused(config.EnvStaging)},
		"staging key":      {staging, []string{"privy-token", "--print-public-key"}, 1, refused(config.EnvStaging)},
		"production token": {production, []string{"privy-token", "--sub", "did:privy:x"}, 1, refused(config.EnvProduction)},
		"production key":   {production, []string{"privy-token", "--print-public-key"}, 1, refused(config.EnvProduction)},
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
