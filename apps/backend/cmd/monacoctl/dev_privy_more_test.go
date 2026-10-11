package main

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	identity "github.com/monaco/monaco/apps/backend/internal/modules/identity/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/config"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
	"github.com/monaco/monaco/apps/backend/internal/testkit/fakes"
)

func scriptedPrivyConfig(t *testing.T, transfer int, action string) config.Config {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			w.WriteHeader(transfer)
			_, _ = io.WriteString(w, `{"id":"a1","status":"pending"}`)
			return
		}
		_, _ = io.WriteString(w, `{"id":"a1","status":"`+action+`"}`)
	}))
	t.Cleanup(srv.Close)
	return config.Config{Env: config.EnvLocal, Privy: config.Privy{
		AppID: "app-fixture", AppSecret: "secret", BaseURL: srv.URL,
		VerificationKey:         fakes.PrivyVerificationKey(),
		AuthorizationPrivateKey: fakes.PrivyAuthorizationKeyConfig(),
		AuthorizationKeyID:      fakes.PrivyAuthorizationKeyID,
	}, Timeouts: config.Timeouts{Privy: 5 * time.Second}}
}

func TestDevPrivyTransfer_reportsEveryWayTheActionCanGoWrong(t *testing.T) {
	t.Parallel()
	send := []string{
		"privy-transfer", "--wallet", "w", "--to", "0x00000000000000000000000000000000000000aa", "--usdc", "1",
		"--key", "k", "--timeout", "500ms",
	}
	for name, tc := range map[string]struct {
		cfg  config.Config
		want string
		poll string
	}{
		"rejected":       {scriptedPrivyConfig(t, http.StatusOK, "rejected"), "did not succeed", "1ms"},
		"still pending":  {scriptedPrivyConfig(t, http.StatusOK, "pending"), "still pending", "1h"},
		"unknown status": {scriptedPrivyConfig(t, http.StatusOK, "exploded"), "decode_failed", "1ms"},
		"transfer 400":   {scriptedPrivyConfig(t, http.StatusBadRequest, "pending"), "invalid_input", "1ms"},
		"no privy keys":  {devConfig(config.EnvLocal), "privy.New", "1ms"},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			var stdout, stderr bytes.Buffer
			if code := devCmd(
				tc.cfg,
				send,
				&stdout,
				&stderr,
			); code != 1 ||
				!strings.Contains(stderr.String(), tc.want) {
				t.Fatalf("exit %d stderr %q, want %q", code, stderr.String(), tc.want)
			}
		})
	}
}

func TestDevPrivyEVMWallet_reportsAClientFailure(t *testing.T) {
	t.Parallel()
	var stdout, stderr bytes.Buffer
	code := devCmd(devConfig(config.EnvLocal), []string{"privy-evm-wallet", "--key", "k"}, &stdout, &stderr)
	if code != 1 || !strings.Contains(stderr.String(), "privy.New") || stdout.Len() != 0 {
		t.Fatalf("exit %d stderr %q", code, stderr.String())
	}
}

func TestDevPrivyTransfer_sendsFromTheUsersMemberWallet(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	q := identity.New(pool)
	user := uuid.New()
	now := clock.Real{}.Now().UTC()
	if _, err := q.CreateUser(t.Context(), identity.CreateUserParams{
		ID: user, PrivyUserID: "did:privy:dev-" + user.String(), LoginProvider: "sms", Now: now,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := q.AttachUserWallet(t.Context(), identity.AttachUserWalletParams{
		UserID: user, PrivyWalletID: "wallet-member", Address: string(qaMember), CreatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	loaded, err := config.Load([]string{
		"MONACO_ENV=test", "DATABASE_URL=" + pool.Config().ConnString(), "NATS_URL=nats://127.0.0.1:1",
	})
	if err != nil {
		t.Fatal(err)
	}
	cfg := privyDevConfig(t, config.EnvLocal)
	cfg.DB = loaded.DB
	args := func(user string) []string {
		return []string{
			"privy-transfer", "--user", user, "--to", "0x00000000000000000000000000000000000000aa", "--usdc", "1",
			"--key", "k", "--poll", "1ms",
		}
	}
	var stdout, stderr bytes.Buffer
	if code := devCmd(cfg, args(user.String()), &stdout, &stderr); code != 0 ||
		!strings.Contains(stdout.String(), `"wallet_id":"wallet-member"`) {
		t.Fatalf("exit %d stdout %s stderr %s", code, stdout.String(), stderr.String())
	}
	for raw, want := range map[string]string{uuid.NewString(): "member wallet of", "not-a-uuid": "--user"} {
		stderr.Reset()
		if code := devCmd(cfg, args(raw), io.Discard, &stderr); code != 1 || !strings.Contains(stderr.String(), want) {
			t.Fatalf("--user %s: exit %d stderr %q", raw, code, stderr.String())
		}
	}
	cfg.DB.URL = "postgres://127.0.0.1:1/monaco"
	stderr.Reset()
	if code := devCmd(cfg, args(user.String()), io.Discard, &stderr); code != 1 {
		t.Fatalf("unreachable database: exit %d stderr %q", code, stderr.String())
	}
}
