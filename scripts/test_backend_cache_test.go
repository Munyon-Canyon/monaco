package scripts_test

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func gitRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	git(t, dir, "init", "-q")
	git(t, dir, "config", "user.email", "a@example.com")
	git(t, dir, "config", "user.name", "a")
	if err := os.WriteFile(filepath.Join(dir, "keep"), []byte("a\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	git(t, dir, "add", "keep")
	git(t, dir, "commit", "-q", "-m", "chore: root")
	return dir
}

func cacheExec(t *testing.T, dir, cache, command string) (string, error) {
	t.Helper()
	cmd := exec.Command(filepath.Join(repoRoot(t), "scripts", "test-backend.sh"))
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"MONACO_TEST_BACKEND_CACHE="+cache,
		"TEST_BACKEND_CACHE_EXEC="+command,
	)
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func TestTestBackendCache_repeatTreeSkipsTheCommandAndAFailureDoesNot(t *testing.T) {
	dir := gitRepo(t)
	cache := t.TempDir()
	marker := filepath.Join(t.TempDir(), "ran")
	cmd := "printf 'x\\n' >> " + shellQuote(marker)
	out, err := cacheExec(t, dir, cache, cmd)
	if err != nil {
		t.Fatal(err, out)
	}
	out, err = cacheExec(t, dir, cache, cmd)
	if err != nil {
		t.Fatal(err, out)
	}
	if !strings.Contains(out, "cached PASS") {
		t.Fatalf("second run did not use the cache: %s", out)
	}
	body, err := os.ReadFile(marker)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(string(body), "x") != 1 {
		t.Fatalf("command ran %d times, want 1:\n%s", strings.Count(string(body), "x"), body)
	}

	fail := "printf 'y\\n' >> " + shellQuote(marker) + "; exit 1"
	if _, err := cacheExec(t, dir, cache, fail); err == nil {
		t.Fatal("failure was cached as success")
	}
	if _, err := cacheExec(t, dir, cache, fail); err == nil {
		t.Fatal("second failure was cached as success")
	}
	body, err = os.ReadFile(marker)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(string(body), "y") != 2 {
		t.Fatalf("failed command ran %d times, want 2", strings.Count(string(body), "y"))
	}

	if err := os.WriteFile(filepath.Join(dir, "untracked"), []byte("new\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	again := "printf 'z\\n' >> " + shellQuote(marker)
	if _, err := cacheExec(t, dir, cache, again); err != nil {
		t.Fatal(err)
	}
	if _, err := cacheExec(t, dir, cache, again); err != nil {
		t.Fatal(err)
	}
	body, _ = os.ReadFile(marker)
	if strings.Count(string(body), "z") != 1 {
		t.Fatalf("untracked file did not bust the cache, z count=%d", strings.Count(string(body), "z"))
	}
}

func TestTestBackendCache_failingSuiteKeepsItsStatusWhenTeeSucceeds(t *testing.T) {
	t.Parallel()
	dir := gitRepo(t)
	cache := t.TempDir()
	const want = 17
	out, err := cacheExec(t, dir, cache, "echo suite-failed; exit 17")
	var exit *exec.ExitError
	if !errors.As(err, &exit) || exit.ExitCode() != want {
		t.Fatalf("run_cached returned %v, want exit %d:\n%s", err, want, out)
	}
	if !strings.Contains(out, "suite-failed") {
		t.Fatalf("tee did not forward the suite output:\n%s", out)
	}
	entries, err := os.ReadDir(cache)
	if err != nil {
		t.Fatal(err)
	}
	logged := false
	for _, entry := range entries {
		name := entry.Name()
		if strings.HasSuffix(name, ".pass") {
			t.Fatalf("failure was stored as a pass: %s", name)
		}
		if !strings.HasSuffix(name, ".log") {
			continue
		}
		body, err := os.ReadFile(filepath.Join(cache, name))
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(body), "suite-failed") {
			logged = true
		}
	}
	if !logged {
		t.Fatal("tee did not write the suite output to the cache log")
	}
}
