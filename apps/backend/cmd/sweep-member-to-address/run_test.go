package main

import (
	"bytes"
	"errors"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"strings"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/testkit"
	"github.com/monaco/monaco/apps/backend/internal/testkit/fakes"
)

func runEnv(t *testing.T) func(extra ...string) []string {
	t.Helper()
	base := []string{
		"MONACO_ENV=test",
		"NATS_URL=nats://unused",
		"PRIVY_VERIFICATION_KEY=" + fakes.PrivyVerificationKey(),
		"DATABASE_URL=" + testkit.DB(t).Config().ConnString(),
	}
	return func(extra ...string) []string { return append(append([]string(nil), base...), extra...) }
}

type ran struct {
	code           int
	stdout, stderr string
}

func runWith(t *testing.T, environ []string, stdin string, args ...string) ran {
	t.Helper()
	var stdout, stderr bytes.Buffer
	code := run(t.Context(), args, environ, strings.NewReader(stdin), &stdout, &stderr)
	return ran{code, stdout.String(), stderr.String()}
}

func TestMain_exitsTwoWithTheUsageWithoutADestination(t *testing.T) {
	t.Parallel()
	out, err := testkit.MainCommand(t, nil).CombinedOutput()
	var exit *exec.ExitError
	if !errors.As(err, &exit) || exit.ExitCode() != 2 || !strings.Contains(string(out), "--destination is required") {
		t.Fatalf("no args = %v\n%s", err, out)
	}
}

func TestRun_dryRunOnAnEmptyDatabaseListsNoSourcesAndAsksNothing(t *testing.T) {
	t.Parallel()
	got := runWith(t, runEnv(t)(), "", "--destination", string(dest), "--dry-run")
	if got.code != 0 || !strings.Contains(got.stdout, "DRY-RUN") || !strings.Contains(got.stdout, "sources=0") {
		t.Fatalf("dry run = %+v", got)
	}
}

func TestRun_stopsAtEachBrokenStep(t *testing.T) {
	t.Parallel()
	privyDown := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	}))
	t.Cleanup(privyDown.Close)
	destination := []string{"--destination", string(dest)}
	env := runEnv(t)
	for name, tc := range map[string]struct {
		environ []string
		args    []string
		code    int
		stderr  string
	}{
		"bad flags":     {nil, nil, 2, "--destination is required"},
		"no config":     {[]string{"PATH=/usr/bin"}, destination, 1, "config: "},
		"no privy key":  {env("PRIVY_VERIFICATION_KEY="), destination, 1, "privy: "},
		"no database":   {env("DATABASE_URL=postgres://monaco@127.0.0.1:1/monaco"), destination, 1, "database: "},
		"privy refuses": {env("PRIVY_BASE_URL=" + privyDown.URL), append(destination, "--all"), 1, "list wallets: "},
		"no confirm":    {env(), destination, 1, "abort: confirmation aborted"},
	} {
		got := runWith(t, tc.environ, "", tc.args...)
		if got.code != tc.code || !strings.Contains(got.stderr, tc.stderr) {
			t.Fatalf("%s: run = %+v, want exit %d and %q", name, got, tc.code, tc.stderr)
		}
	}
}
