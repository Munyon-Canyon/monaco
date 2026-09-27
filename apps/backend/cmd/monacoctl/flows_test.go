package main

import (
	"bytes"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
	"testing/iotest"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/tools/flows"
)

const pingRow = "01\tPing\tsystem\tpoller:ping\tPing\tsystem.pinged\t\tok;Internal\tbuilt\tdocs/flows.md#ping"

func envWith(tsv string) flows.Env {
	repo := fstest.MapFS{
		"apps/backend/flows.tsv":                          {Data: []byte(tsv)},
		"apps/backend/internal/modules/system/app/app.go": {Data: []byte("package app\n")},
		"docs/flows.md":                                   {Data: []byte("## Ping\n")},
	}
	return liveEnv(repo, func(string, string) (bool, error) { return true, nil })
}

func pass(names ...string) string {
	var b strings.Builder
	for _, n := range names {
		b.WriteString(`{"Action":"pass","Package":"p","Test":"` + n + "\"}\n")
	}
	return b.String()
}

func TestFlowsCheck(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name   string
		tsv    string
		tests  string
		code   int
		stderr string
	}{
		{"header only", flows.Header + "\n", "", 0, ""},
		{"built row with every test passing", flows.Header + "\n" + pingRow + "\n", pass("TestFlow01_Ping_OK", "TestFlow01_Ping_Internal"), 0, ""},
		{
			"built row missing a test", flows.Header + "\n" + pingRow + "\n", pass("TestFlow01_Ping_OK"), 1,
			"flows.tsv:2: outcome Internal has no test TestFlow01_Ping_Internal in the go test -json input\n",
		},
		{
			"live registry and errs table", flows.Header + "\n" +
				strings.Replace(strings.Replace(pingRow, "system.pinged", "system.pinged;system.exploded", 1), "ok;Internal\tbuilt", "ok;Internal;NoSuchCode;crash:before-commit;crash:after-lunch\tplanned", 1) + "\n",
			"", 1,
			"flows.tsv:2: event system.exploded is not in the events registry\n" +
				"flows.tsv:2: outcome NoSuchCode is not an errs code name\n" +
				"flows.tsv:2: outcome crash:after-lunch is not a registered faultpoint\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var stderr bytes.Buffer
			code := flowsCheck(envWith(tc.tsv), strings.NewReader(tc.tests), &stderr)
			if code != tc.code || stderr.String() != tc.stderr {
				t.Fatalf("code=%d stderr=\n%s\nwant code=%d stderr=\n%s", code, stderr.String(), tc.code, tc.stderr)
			}
		})
	}
}

func TestFlowsCheck_missingFileFails(t *testing.T) {
	t.Parallel()
	env := envWith("")
	env.Repo = fstest.MapFS{}
	var stderr bytes.Buffer
	if code := flowsCheck(env, nil, &stderr); code != 1 || !strings.Contains(stderr.String(), "flows.tsv") {
		t.Fatalf("code=%d stderr=%q", code, stderr.String())
	}
}

func TestFlowsRejectsUnknownArguments(t *testing.T) {
	t.Parallel()
	for _, args := range [][]string{{"flows"}, {"flows", "lint"}, {"flows", "check", "--from"}, {"flows", "check", "-x", "f"}} {
		var stdout, stderr bytes.Buffer
		code := run(commands(), tools(nil), nil, args, &stdout, &stderr)
		if code != 2 || stderr.String() != flowsUsage+"\n" {
			t.Fatalf("%q: code=%d stderr=%q", args, code, stderr.String())
		}
	}
}

func TestFlowsCheck_fromAMissingFileFails(t *testing.T) {
	t.Parallel()
	var stdout, stderr bytes.Buffer
	missing := filepath.Join(t.TempDir(), "go-test.json")
	if code := run(
		commands(),
		tools(nil),
		nil,
		[]string{"flows", "check", "--from", missing},
		&stdout,
		&stderr,
	); code != 1 {
		t.Fatalf("code=%d stderr=%q", code, stderr.String())
	}
}

func TestGitFresh_comparesTheStampWithTheModulesNewestCommit(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	git := func(args ...string) string {
		t.Helper()
		cmd := exec.CommandContext(t.Context(), "git", args...)
		cmd.Dir = dir
		cmd.Env = append(
			os.Environ(),
			"GIT_AUTHOR_NAME=t",
			"GIT_AUTHOR_EMAIL=t@t",
			"GIT_COMMITTER_NAME=t",
			"GIT_COMMITTER_EMAIL=t@t",
		)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
		return strings.TrimSpace(string(out))
	}
	commit := func(file string) string {
		t.Helper()
		full := filepath.Join(dir, file)
		if err := os.MkdirAll(filepath.Dir(full), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(file+git("rev-list", "--all", "--count")), 0o600); err != nil {
			t.Fatal(err)
		}
		git("add", ".")
		git("commit", "-q", "-m", file)
		return git("rev-parse", "HEAD")
	}
	git("init", "-q")
	before := commit("internal/modules/treasury/app/fund.go")
	moduleHead := commit("internal/modules/treasury/app/fund.go")
	after := commit("test/evidence/07.json")

	fresh := gitFresh(t.Context(), dir)
	for _, tc := range []struct {
		module, sha string
		want        bool
	}{
		{"treasury", before, false},
		{"treasury", moduleHead, true},
		{"treasury", after, true},
		{"ghost", before, true},
	} {
		if got, err := fresh(tc.module, tc.sha); err != nil || got != tc.want {
			t.Errorf("fresh(%s, %s) = %v, %v; want %v", tc.module, tc.sha, got, err, tc.want)
		}
	}
	if _, err := fresh("treasury", strings.Repeat("0", 40)); err == nil {
		t.Error("fresh(unknown sha) err = nil, want an error")
	}
}

func TestGitFresh_failsWhenGitCannotRun(t *testing.T) {
	t.Parallel()
	fresh, err := gitFresh(t.Context(), filepath.Join(t.TempDir(), "missing"))("treasury", "abc")
	if fresh || errs.CodeOf(err) != errs.CodeInternal {
		t.Fatalf("gitFresh = %v, %v; want false and an internal error", fresh, err)
	}
}

func TestFlowsCheck_failsWhenTheTestResultsCannotBeRead(t *testing.T) {
	t.Parallel()
	var stderr bytes.Buffer
	code := flowsCheck(envWith(flows.Header+"\n"), iotest.ErrReader(io.ErrUnexpectedEOF), &stderr)
	if code != 1 || !strings.HasPrefix(stderr.String(), "monacoctl flows check: ") ||
		!strings.Contains(stderr.String(), io.ErrUnexpectedEOF.Error()) {
		t.Fatalf("code=%d stderr=%q", code, stderr.String())
	}
}
