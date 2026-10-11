package main

import (
	"bytes"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/platform/config"
	"github.com/monaco/monaco/apps/backend/internal/testkit/fakes"
)

func privyDevConfig(t *testing.T, env config.Env) config.Config {
	t.Helper()
	srv := httptest.NewServer(fakes.New())
	t.Cleanup(srv.Close)
	return config.Config{Env: env, Privy: config.Privy{
		AppID: "app-fixture", AppSecret: "secret", BaseURL: srv.URL + "/privy",
		VerificationKey:         fakes.PrivyVerificationKey(),
		AuthorizationPrivateKey: fakes.PrivyAuthorizationKeyConfig(),
		AuthorizationKeyID:      fakes.PrivyAuthorizationKeyID,
	}, Timeouts: config.Timeouts{Privy: 5 * time.Second}}
}

func TestDevPrivyTransfer_sendsAndPollsUntilTheActionSucceeds(t *testing.T) {
	t.Parallel()
	cfg := privyDevConfig(t, config.EnvLocal)
	var stdout, stderr bytes.Buffer
	if code := devCmd(cfg, []string{"privy-evm-wallet", "--key", "qa-dest"}, &stdout, &stderr); code != 0 {
		t.Fatalf("privy-evm-wallet exit %d: %s", code, stderr.String())
	}
	address := strings.TrimSpace(strings.SplitAfter(stdout.String(), "address ")[1])
	stdout.Reset()
	code := devCmd(cfg, []string{
		"privy-transfer", "--wallet", "wallet-member", "--to", address, "--usdc", "1", "--key", "qa-1",
	}, &stdout, &stderr)
	out := stdout.String()
	if code != 0 || !strings.Contains(out, "action ") || !strings.Contains(out, `"state":"succeeded"`) ||
		!strings.Contains(out, `"network":"solana"`) {
		t.Fatalf("exit %d stdout %s stderr %s", code, out, stderr.String())
	}
}

func TestDevPrivyCommands_refuseBadArgumentsAndProduction(t *testing.T) {
	t.Parallel()
	ok := []string{"--wallet", "w", "--to", "0x00000000000000000000000000000000000000aa", "--key", "k"}
	for name, tc := range map[string]struct {
		env  config.Env
		args []string
		code int
	}{
		"over five":       {config.EnvLocal, append([]string{"privy-transfer", "--usdc", "5.000001"}, ok...), 2},
		"zero":            {config.EnvLocal, append([]string{"privy-transfer", "--usdc", "0"}, ok...), 2},
		"no key":          {config.EnvLocal, []string{"privy-transfer", "--wallet", "w", "--to", "x", "--usdc", "1"}, 2},
		"both senders":    {config.EnvLocal, append([]string{"privy-transfer", "--usdc", "1", "--user", "u"}, ok...), 2},
		"bad network":     {config.EnvLocal, append([]string{"privy-transfer", "--usdc", "1", "--network", "mars"}, ok...), 2},
		"wallet no key":   {config.EnvLocal, []string{"privy-evm-wallet"}, 2},
		"production send": {config.EnvProduction, append([]string{"privy-transfer", "--usdc", "1"}, ok...), 1},
		"production make": {config.EnvProduction, []string{"privy-evm-wallet", "--key", "k"}, 1},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			var stdout, stderr bytes.Buffer
			if code := devCmd(devConfig(tc.env), tc.args, &stdout, &stderr); code != tc.code || stdout.Len() != 0 {
				t.Fatalf("exit %d stdout %q stderr %q, want %d", code, stdout.String(), stderr.String(), tc.code)
			}
		})
	}
}
