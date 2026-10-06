package main

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"strings"
	"testing"
	"time"

	"go.opentelemetry.io/otel/metric/noop"

	openapi "github.com/monaco/monaco/apps/backend/api"
	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
	"github.com/monaco/monaco/apps/backend/internal/testkit/fakes"
)

const privyAppID = "app-fixture"

func TestRun_refusesToBootInStagingAndProductionWithTheFixtureVerificationKey(t *testing.T) {
	t.Parallel()
	fixture := fakes.PrivyVerificationKey()
	for name, tc := range map[string]struct{ env, key string }{
		"staging":                {"staging", fixture},
		"production":             {"production", fixture},
		"production on one line": {"production", strings.ReplaceAll(fixture, "\n", `\n`)},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			err := run(t.Context(), io.Discard, append([]string{
				"MONACO_ENV=" + tc.env, "DATABASE_URL=postgres://localhost/monaco", "NATS_URL=nats://localhost:4222",
				"PRIVY_VERIFICATION_KEY=" + tc.key, "POSTHOG_API_KEY=ph-key",
			}, testkit.DeployedEnv()...), openapi.Spec, noop.NewMeterProvider())
			if errs.CodeOf(err) != errs.CodeInvalidConfig ||
				!strings.HasPrefix(err.Error(), "privy.CheckVerificationKey: ") {
				t.Fatalf("run = %v, want invalid_config from privy.CheckVerificationKey", err)
			}
		})
	}
}

func TestMain_exitsOneAndLogsWhyTheFixtureKeyIsRefusedWithoutEchoingIt(t *testing.T) {
	t.Parallel()
	fixture := fakes.PrivyVerificationKey()
	out, err := testkit.MainCommand(t, append([]string{
		"MONACO_ENV=production", "DATABASE_URL=postgres://localhost/monaco", "NATS_URL=nats://localhost:4222",
		"PRIVY_VERIFICATION_KEY=" + fixture, "POSTHOG_API_KEY=ph-key",
	}, testkit.DeployedEnv()...)).CombinedOutput()
	var exit *exec.ExitError
	if !errors.As(err, &exit) || exit.ExitCode() != 1 {
		t.Fatalf("api with the fixture key in production = %v\n%s", err, out)
	}
	for _, want := range []string{
		`"msg":"boot.stopped"`, `"code":"invalid_config"`,
		"PRIVY_VERIFICATION_KEY is the fixture key, which is for tests only",
	} {
		if !strings.Contains(string(out), want) {
			t.Errorf("boot log lacks %s:\n%s", want, out)
		}
	}
	for _, line := range strings.Split(strings.TrimSpace(fixture), "\n")[1:3] {
		if strings.Contains(string(out), line) {
			t.Fatalf("boot log echoes the key:\n%s", out)
		}
	}
}

func TestMain_verifiesFixtureTokensOnlyWhereTheFixtureKeyIsAllowed(t *testing.T) {
	t.Parallel()
	rpc := httptest.NewServer(fakes.New())
	t.Cleanup(rpc.Close)
	deployed := append([]string{
		"SOLANA_RPC_URL=" + rpc.URL + "/rpc/",
		"RELAYER_PRIVATE_KEY=" + chain.EncodeBase58(fakes.FixtureKey("relayer")),
		"POSTHOG_API_KEY=ph-key",
	}, testkit.DeployedEnv()...)
	for name, tc := range map[string]struct {
		env, key, wantCode string
		extra              []string
	}{
		"the fixture key in test":   {"test", fakes.PrivyVerificationKey(), "session_required", nil},
		"another key in production": {"production", fakes.OtherPrivyVerificationKey(), "unauthorized", deployed},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			env := bootEnv(t, append([]string{
				"MONACO_ENV=" + tc.env, "MONACO_HTTP_ADDR=127.0.0.1:0", "PRIVY_APP_ID=" + privyAppID,
				"PRIVY_VERIFICATION_KEY=" + tc.key,
			}, tc.extra...)...)
			p := testkit.StartMain(t, env)
			token := fakes.PrivyAccessToken(privyAppID, "did:privy:member", clock.Real{}.Now(), time.Hour)
			ping := "http://" + p.Addr + "/v1/system/pings/00000000-0000-0000-0000-000000000000"
			status, code := problemOf(t, ping, token)
			if status != http.StatusUnauthorized || code != tc.wantCode {
				t.Fatalf("a fixture token got %d %q, want 401 %q", status, code, tc.wantCode)
			}
			stderr, err := p.Terminate()
			if err != nil {
				t.Fatalf("api after SIGTERM: %v\n%s", err, stderr)
			}
			if want := `"MONACO_ENV":"` + tc.env + `"`; !strings.Contains(stderr, want) {
				t.Fatalf("boot config lacks %s:\n%s", want, stderr)
			}
		})
	}
}

func problemOf(t *testing.T, url, token string) (int, string) {
	t.Helper()
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, url, nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
	defer func() { _ = resp.Body.Close() }()
	var problem struct {
		Code string `json:"code"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&problem); err != nil {
		t.Fatalf("GET %s answered %d with a body that is not a problem: %v", url, resp.StatusCode, err)
	}
	return resp.StatusCode, problem.Code
}
