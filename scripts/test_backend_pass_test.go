package scripts_test

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const passGoStub = `#!/bin/sh
case "$1" in
list) printf 'm/a\nm/b\nm/c\nm/d\n' ;;
test)
  echo "$@" >>"$STUB_LOG"
  for a; do
    case "$a" in -coverprofile=*) echo 'mode: atomic' >"${a#-coverprofile=}" ;; esac
  done
  echo '{"Time":"2026-09-27T10:00:00Z","Action":"pass","Package":"m/a","Elapsed":1}'
  ;;
run)
  echo "$@" >>"$STUB_LOG"
  prev=""
  for a; do
    case "$prev" in --from) cat "$a" >"$STUB_LOG.from" ;; --budget-exempt) cat "$a" >"$STUB_LOG.exempt" ;; esac
    prev="$a"
  done
  ;;
esac
`

func runPass(t *testing.T, pass, out, log string) (string, int) {
	t.Helper()
	return runPassFrom(t, pass, out, log, "", "")
}

// runPassFrom runs the script as script from the repo-relative directory dir; an empty script is the absolute path from a temp repo.
func runPassFrom(t *testing.T, pass, out, log, dir, script string) (string, int) {
	t.Helper()
	stub := t.TempDir()
	writeExecutable(t, filepath.Join(stub, "go"), passGoStub)
	cmd := exec.Command(filepath.Join(repoRoot(t), "scripts", "test-backend.sh"))
	cmd.Dir = gitRepo(t)
	if script != "" {
		cmd = exec.Command(script)
		cmd.Dir = filepath.Join(repoRoot(t), dir)
	}
	cmd.Env = append(os.Environ(),
		"PATH="+stub+string(os.PathListSeparator)+os.Getenv("PATH"),
		"MONACO_TEST_BACKEND_CACHE="+t.TempDir(),
		"TEST_BACKEND_CACHE_EXEC=",
		"TEST_BACKEND_PASS="+pass,
		"TEST_BACKEND_OUT="+out,
		"STUB_LOG="+log,
	)
	body, err := cmd.CombinedOutput()
	var exit *exec.ExitError
	switch {
	case err == nil:
		return string(body), 0
	case errors.As(err, &exit):
		return string(body), exit.ExitCode()
	}
	t.Fatal(err)
	return "", 0
}

func readOrEmpty(t *testing.T, path string) string {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		t.Fatal(err)
	}
	return string(body)
}

func TestTestBackendPass_shardsSplitThePackagesAndEachWritesItsOwnFiles(t *testing.T) {
	t.Parallel()
	out, log := t.TempDir(), filepath.Join(t.TempDir(), "log")
	for _, pass := range []string{"short:1/2", "short:2/2"} {
		if body, code := runPass(t, pass, out, log); code != 0 {
			t.Fatalf("%s exited %d:\n%s", pass, code, body)
		}
	}
	calls := strings.Split(strings.TrimSpace(readOrEmpty(t, log)), "\n")
	if len(calls) != 2 || !strings.HasSuffix(calls[0], " m/a m/c") || !strings.HasSuffix(calls[1], " m/b m/d") {
		t.Fatalf("go test calls = %q, want shard 1 on m/a m/c and shard 2 on m/b m/d", calls)
	}
	for _, name := range []string{"short-1-of-2.json", "short-1-of-2.cover", "short-2-of-2.json", "short-2-of-2.cover"} {
		if _, err := os.Stat(filepath.Join(out, name)); err != nil {
			t.Fatalf("shard output missing: %v", err)
		}
	}
}

func TestTestBackendPass_reportMergesEveryFileUnderTheOutDir(t *testing.T) {
	t.Parallel()
	out, log := t.TempDir(), filepath.Join(t.TempDir(), "log")
	files := map[string]string{
		"short-1-of-2.json":  "a\n",
		"short-2-of-2.json":  "b\n",
		"rest.json":          "c\n",
		"full-1-of-2.json":   "exempt1\n",
		"full-2-of-2.json":   "exempt2\n",
		"short-1-of-2.cover": "mode: atomic\nx 1\n",
		"short-2-of-2.cover": "mode: atomic\ny 1\n",
		"full.cover":         "mode: atomic\nz 1\n",
	}
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(out, name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if body, code := runPass(t, "report", out, log); code != 0 {
		t.Fatalf("report exited %d:\n%s", code, body)
	}
	calls := strings.Split(strings.TrimSpace(readOrEmpty(t, log)), "\n")
	if len(calls) != 3 {
		t.Fatalf("go calls = %q, want test-report, flows check and coverage", calls)
	}
	if !strings.Contains(calls[0], "test-report --from ") || !strings.Contains(calls[0], " --budget-exempt ") {
		t.Fatalf("test-report call = %q, want --from and --budget-exempt", calls[0])
	}
	if got := readOrEmpty(t, log+".exempt"); got != "exempt1\nexempt2\n" {
		t.Fatalf("budget-exempt input = %q, want every full-*.json and nothing else", got)
	}
	if got := readOrEmpty(t, log+".from"); strings.Contains(got, "exempt") {
		t.Fatalf("--from input = %q, want no full-*.json in it", got)
	}
}

func TestTestBackendPass_restShardsSplitTheFullPackagesAndOnlyShardOneRunsFaultpoint(t *testing.T) {
	t.Parallel()
	out, log := t.TempDir(), filepath.Join(t.TempDir(), "log")
	for _, pass := range []string{"rest:1/2", "rest:2/2"} {
		if body, code := runPass(t, pass, out, log); code != 0 {
			t.Fatalf("%s exited %d:\n%s", pass, code, body)
		}
	}
	var full []string
	faultpoint := 0
	for _, call := range strings.Split(strings.TrimSpace(readOrEmpty(t, log)), "\n") {
		switch {
		case strings.Contains(call, "./internal/platform/faultpoint/"):
			faultpoint++
		case strings.Contains(call, "-coverpkg=./... "):
			full = append(full, call)
		}
	}
	if faultpoint != 1 {
		t.Fatalf("faultpoint pass ran %d times, want once, in shard 1", faultpoint)
	}
	if len(full) != 2 || !strings.HasSuffix(full[0], " m/a m/c") || !strings.HasSuffix(full[1], " m/b m/d") {
		t.Fatalf("full-pass calls = %q, want shard 1 on m/a m/c and shard 2 on m/b m/d", full)
	}
	for _, name := range []string{"rest.json", "full-1-of-2.json", "full-1-of-2.cover", "full-2-of-2.json", "full-2-of-2.cover"} {
		if _, err := os.Stat(filepath.Join(out, name)); err != nil {
			t.Fatalf("rest shard output missing: %v", err)
		}
	}
}

func TestTestBackendPass_rejectsAnUnknownPassAnAbsentShardAndAnEmptyReport(t *testing.T) {
	t.Parallel()
	log := filepath.Join(t.TempDir(), "log")
	for _, tc := range []struct{ pass, want string }{
		{"bogus", "is not short:<i>/<n>, rest, rest:<i>/<n> or report"},
		{"short:3/2", "past the shard count"},
		{"rest:3/2", "past the shard count"},
		{"report", "no test output under"},
	} {
		body, code := runPass(t, tc.pass, t.TempDir(), log)
		if code == 0 || !strings.Contains(body, tc.want) {
			t.Fatalf("%s: exit %d, output %q, want failure containing %q", tc.pass, code, body, tc.want)
		}
	}
}

func TestTestBackendPass_selectsPackagesWhenInvokedByARelativePath(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ name, dir, script string }{
		{"from the repo root", "", "scripts/test-backend.sh"},
		{"from apps/backend", "apps/backend", "../../scripts/test-backend.sh"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			log := filepath.Join(t.TempDir(), "log")
			if body, code := runPassFrom(t, "short:1/2", t.TempDir(), log, tc.dir, tc.script); code != 0 {
				t.Fatalf("exited %d:\n%s", code, body)
			}
			if calls := readOrEmpty(t, log); !strings.Contains(calls, " m/a m/c") {
				t.Fatalf("go test calls = %q, want shard 1 to select m/a m/c", calls)
			}
		})
	}
}
