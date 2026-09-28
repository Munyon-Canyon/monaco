package scripts_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestFlakeTests_selectsChangedTestFuncs(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "apps", "backend", "internal", "db", "lock_test.go")
	if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
		t.Fatal(err)
	}
	body := "package db\n\nfunc TestLock(t *testing.T) {}\nfunc TestUnlock(t *testing.T) {}\nfunc helper() {}\n"
	if err := os.WriteFile(file, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	other := filepath.Join(dir, "apps", "backend", "internal", "db", "note.go")
	if err := os.WriteFile(other, []byte("package db\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	script := filepath.Join(repoRoot(t), "scripts", "ci", "flake-tests.sh")
	cmd := exec.Command("bash", script, "--print", "--root", dir,
		"apps/backend/internal/db/lock_test.go", "apps/backend/internal/db/note.go")
	cmd.Env = append(os.Environ(), "PYENV_VERSION=system")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("flake-tests.sh: %v\n%s", err, out)
	}
	got := strings.TrimSpace(string(out))
	want := "go test -count=20 -cpu=1,2 -run '^(TestLock|TestUnlock)$' ./internal/db"
	if got != want {
		t.Fatalf("got %q\nwant %q", got, want)
	}
}
