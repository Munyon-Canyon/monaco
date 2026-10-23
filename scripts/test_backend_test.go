package scripts_test

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestTestBackend_exitsOneAndPrintsTheFailedTestsWhenGoTestFails(t *testing.T) {
	t.Parallel()
	stub := t.TempDir()
	goStub := "#!/bin/sh\necho '{\"Action\":\"fail\",\"Package\":\"p\",\"Test\":\"TestX\"}'\nexit 1\n"
	writeExecutable(t, filepath.Join(stub, "go"), goStub)
	cmd := exec.Command(filepath.Join(repoRoot(t), "scripts", "test-backend.sh"))
	cmd.Dir = gitRepo(t)
	cmd.Env = append(os.Environ(),
		"PATH="+stub+string(os.PathListSeparator)+os.Getenv("PATH"),
		"MONACO_TEST_BACKEND_CACHE="+t.TempDir(),
		"TEST_BACKEND_CACHE_EXEC=",
		"TEST_BACKEND_FORCE=",
	)
	out, err := cmd.CombinedOutput()
	var exit *exec.ExitError
	if !errors.As(err, &exit) || exit.ExitCode() != 1 {
		t.Fatalf("test-backend.sh returned %v, want exit 1:\n%s", err, out)
	}
	if !strings.Contains(string(out), "--- output of failed tests ---") {
		t.Fatalf("test-backend.sh printed no failed tests section:\n%s", out)
	}
}

func TestTestBackendPass_weightsRebuildsBothTablesFromTheArtifactJSON(t *testing.T) {
	t.Parallel()
	tree, backend := newBackendTree(t)
	out := t.TempDir()
	artifact := filepath.Join(out, "backend-test-short-1")
	if err := os.MkdirAll(artifact, 0o750); err != nil {
		t.Fatal(err)
	}
	write := func(path, body string) { writeFile(t, path, body) }
	write(filepath.Join(artifact, "short-1-of-2.json"), strings.Join([]string{
		`{"Action":"pass","Package":"m/a","Test":"TestX","Elapsed":9}`,
		`{"Action":"pass","Package":"m/a","Elapsed":1.25}`,
		`{"Action":"pass","Package":"m/b","Elapsed":2}`,
		`{"Action":"pass","Package":"m/b","Elapsed":3}`,
	}, "\n")+"\n")
	write(filepath.Join(artifact, "full-1-of-1.json"), `{"Action":"fail","Package":"m/c","Elapsed":7.04}`+"\n")
	if body, code := runScript(t, tree, "TEST_BACKEND_PASS=weights", "TEST_BACKEND_OUT="+out); code != 0 {
		t.Fatalf("weights exited %d:\n%s", code, body)
	}
	short := readOrEmpty(t, filepath.Join(backend, ".test-shards.tsv"))
	full := readOrEmpty(t, filepath.Join(backend, ".test-shards-full.tsv"))
	if !strings.HasPrefix(short, "# ") || !strings.HasSuffix(short, "\nm/a\t1.2\nm/b\t3.0\n") {
		t.Fatalf("short table = %q, want a # header, m/a at its own elapsed and m/b at its longest result", short)
	}
	if !strings.HasPrefix(full, "# ") || !strings.HasSuffix(full, "\nm/c\t7.0\n") {
		t.Fatalf("full table = %q, want a # header and m/c", full)
	}
}

func newBackendTree(t *testing.T) (tree, backend string) {
	t.Helper()
	tree = t.TempDir()
	backend = filepath.Join(tree, "apps", "backend")
	for _, dir := range []string{backend, filepath.Join(tree, "scripts")} {
		if err := os.MkdirAll(dir, 0o750); err != nil {
			t.Fatal(err)
		}
	}
	for _, name := range []string{"test-backend.sh", "test-shard.awk"} {
		body, err := os.ReadFile(filepath.Join(repoRoot(t), "scripts", name))
		if err != nil {
			t.Fatal(err)
		}
		writeExecutable(t, filepath.Join(tree, "scripts", name), string(body))
	}
	return tree, backend
}

func runScript(t *testing.T, tree string, env ...string) (string, int) {
	t.Helper()
	cmd := exec.Command(filepath.Join(tree, "scripts", "test-backend.sh"))
	cmd.Env = append(os.Environ(), env...)
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

const affectedGoStub = `#!/bin/sh
case "$1" in
list)
  shift
  fmt=""
  pats=""
  while [ $# -gt 0 ]; do
    case "$1" in
      -tags) shift ;;
      -f) fmt=1; shift ;;
      -e) ;;
      *) pats="$pats $1" ;;
    esac
    shift
  done
  for p in $pats; do
    case "$p" in
      ./...) printf '%s\n' $STUB_UNIVERSE ;;
      *)
        for m in $STUB_UNIVERSE; do
          if [ "$m" = "m/${p#./}" ]; then
            if [ -n "$fmt" ]; then echo "$PWD/${p#./}/f.go"; else echo "$m"; fi
          fi
        done
        ;;
    esac
  done
  ;;
test)
  echo "$@" >>"$STUB_LOG"
  for a; do
    case "$a" in -coverprofile=*) printf 'mode: atomic\nm/x.go:1.1,2.2 1 1\n' >"${a#-coverprofile=}" ;; esac
  done
  echo '{"Time":"2026-09-27T10:00:00Z","Action":"pass","Package":"m/a","Elapsed":1}'
  ;;
esac
`

func affectedEnv(t *testing.T, out, log, packages string) []string {
	t.Helper()
	stub := t.TempDir()
	writeExecutable(t, filepath.Join(stub, "go"), affectedGoStub)
	env := []string{
		"PATH=" + stub + string(os.PathListSeparator) + os.Getenv("PATH"),
		"TEST_BACKEND_OUT=" + out,
		"STUB_LOG=" + log,
		"STUB_UNIVERSE=m/a m/b m/c m/d",
	}
	if packages != "" {
		file := filepath.Join(t.TempDir(), "packages")
		writeFile(t, file, packages)
		env = append(env, "TEST_BACKEND_PACKAGES="+file)
	}
	return env
}

func TestTestBackendAffected_aPackagesFileRestrictsTheShardAndListsItsFiles(t *testing.T) {
	t.Parallel()
	tree, _ := newBackendTree(t)
	out, log := t.TempDir(), filepath.Join(t.TempDir(), "log")
	body, code := runScript(t, tree, append(affectedEnv(t, out, log, "./a\n./c\n"), "TEST_BACKEND_PASS=short:1/1")...)
	if code != 0 {
		t.Fatalf("exited %d:\n%s", code, body)
	}
	if calls := readOrEmpty(t, log); !strings.HasSuffix(strings.TrimSpace(calls), " m/a m/c") {
		t.Fatalf("go test calls = %q, want only the listed packages m/a m/c", calls)
	}
	if got := readOrEmpty(t, filepath.Join(out, "covered.txt")); got != "a/f.go\nc/f.go\n" {
		t.Fatalf("covered.txt = %q, want the listed packages' files relative to apps/backend", got)
	}
}

func TestTestBackendAffected_anEmptyIntersectionPrintsNothingToTestAndWritesNoJSON(t *testing.T) {
	t.Parallel()
	tree, _ := newBackendTree(t)
	out, log := t.TempDir(), filepath.Join(t.TempDir(), "log")
	for _, pass := range []string{"short:1/2", "rest:1/1"} {
		body, code := runScript(t, tree, append(affectedEnv(t, out, log, "./elsewhere\n"), "TEST_BACKEND_PASS="+pass)...)
		if code != 0 || !strings.Contains(body, "nothing to test in this leg") {
			t.Fatalf("%s: exit %d, output %q, want exit 0 and nothing to test in this leg", pass, code, body)
		}
	}
	if calls := readOrEmpty(t, log); calls != "" {
		t.Fatalf("go test calls = %q, want none", calls)
	}
	matches, err := filepath.Glob(filepath.Join(out, "*.json"))
	if err != nil || len(matches) != 0 {
		t.Fatalf("json files = %v (%v), want none", matches, err)
	}
	if _, err := os.Stat(filepath.Join(out, "covered.txt")); err != nil {
		t.Fatalf("an empty leg must still write covered.txt: %v", err)
	}
}

func TestTestBackendAffected_reportGatesOnlyTheCoveredFilesAndTakesAnEmptySetAsAPass(t *testing.T) {
	t.Parallel()
	tree, _ := newBackendTree(t)
	ctl := filepath.Join(t.TempDir(), "monacoctl")
	writeExecutable(t, ctl, "#!/bin/sh\necho \"$@\" >>\"$STUB_LOG\"\n")
	out, log := t.TempDir(), filepath.Join(t.TempDir(), "log")
	writeFile(t, filepath.Join(out, "short-1-of-8.json"), "{}\n")
	writeFile(t, filepath.Join(out, "short-1-of-8.cover"), "mode: atomic\nx 1\n")
	writeFile(t, filepath.Join(out, "covered.txt"), "a/f.go\nc/f.go\n")
	env := append(affectedEnv(t, out, log, ""), "TEST_BACKEND_PASS=report", "MONACOCTL="+ctl, "BASE=abc123")
	if body, code := runScript(t, tree, env...); code != 0 {
		t.Fatalf("report exited %d:\n%s", code, body)
	}
	calls := readOrEmpty(t, log)
	if !strings.Contains(calls, " --only a/f.go --only c/f.go\n") || !strings.Contains(calls, "flows check --affected --base abc123 --from ") {
		t.Fatalf("monacoctl calls = %q, want coverage limited with --only and flows scoped to the diff", calls)
	}

	empty := t.TempDir()
	writeFile(t, filepath.Join(empty, "covered.txt"), "")
	env = append(affectedEnv(t, empty, log, ""), "TEST_BACKEND_PASS=report", "MONACOCTL="+ctl)
	if body, code := runScript(t, tree, env...); code != 0 || !strings.Contains(body, "no affected packages") {
		t.Fatalf("empty set: exit %d, output %q, want exit 0 and no affected packages", code, body)
	}

	env = append(affectedEnv(t, t.TempDir(), log, ""), "TEST_BACKEND_PASS=report", "MONACOCTL="+ctl)
	if body, code := runScript(t, tree, env...); code == 0 || !strings.Contains(body, "no test output under") {
		t.Fatalf("no output and no packages file: exit %d, output %q, want the missing output error", code, body)
	}
}
