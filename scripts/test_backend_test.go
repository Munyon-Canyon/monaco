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
	tree := t.TempDir()
	backend := filepath.Join(tree, "apps", "backend")
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
	out := t.TempDir()
	artifact := filepath.Join(out, "backend-test-short-1")
	if err := os.MkdirAll(artifact, 0o750); err != nil {
		t.Fatal(err)
	}
	write := func(path, body string) {
		t.Helper()
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write(filepath.Join(artifact, "short-1-of-2.json"), strings.Join([]string{
		`{"Action":"pass","Package":"m/a","Test":"TestX","Elapsed":9}`,
		`{"Action":"pass","Package":"m/a","Elapsed":1.25}`,
		`{"Action":"pass","Package":"m/b","Elapsed":2}`,
		`{"Action":"pass","Package":"m/b","Elapsed":3}`,
	}, "\n")+"\n")
	write(filepath.Join(artifact, "full-1-of-1.json"), `{"Action":"fail","Package":"m/c","Elapsed":7.04}`+"\n")
	cmd := exec.Command(filepath.Join(tree, "scripts", "test-backend.sh"))
	cmd.Env = append(os.Environ(), "TEST_BACKEND_PASS=weights", "TEST_BACKEND_OUT="+out)
	if body, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("weights failed: %v\n%s", err, body)
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
