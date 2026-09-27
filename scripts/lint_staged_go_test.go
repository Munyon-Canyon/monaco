package scripts_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func backendRepoWithHook(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("golangci-lint"); err != nil {
		if os.Getenv("CI") != "" {
			t.Fatal("golangci-lint must be on PATH in CI")
		}
		t.Skip("golangci-lint not on PATH; run just install")
	}
	root := repoRoot(t)
	dir := t.TempDir()
	copyFile(t, filepath.Join(root, "scripts/githooks/lint-staged-go.sh"), filepath.Join(dir, "scripts/githooks/lint-staged-go.sh"))
	for _, rel := range []string{
		"apps/backend/go.mod",
		"apps/backend/.golangci.yml",
		"apps/backend/go.sum",
	} {
		copyFile(t, filepath.Join(root, rel), filepath.Join(dir, rel))
	}
	copyBackendPackages(t, root, dir, "./internal/platform/lint/nogo/cmd/nogo", "./cmd/monacoctl")
	git(t, dir, "init", "-q")
	git(t, dir, "add", ".")
	git(t, dir, "-c", "user.name=t", "-c", "user.email=t@t", "commit", "-q", "-m", "base")
	return dir
}

func copyBackendPackages(t *testing.T, root, dir string, pkgs ...string) {
	t.Helper()
	backend := filepath.Join(root, "apps/backend")
	args := append([]string{"list", "-deps", "-f", `{{if .Module}}{{range .GoFiles}}{{$.Dir}}/{{.}}
{{end}}{{range .EmbedFiles}}{{$.Dir}}/{{.}}
{{end}}{{end}}`}, pkgs...)
	cmd := exec.Command("go", args...)
	cmd.Dir = backend
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("go list: %v", err)
	}
	for _, file := range strings.Fields(string(out)) {
		rel, err := filepath.Rel(backend, file)
		if err != nil || strings.HasPrefix(rel, "..") {
			continue
		}
		copyFile(t, file, filepath.Join(dir, "apps/backend", rel))
	}
}

func copyFile(t *testing.T, from, to string) {
	t.Helper()
	b, err := os.ReadFile(from)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(to), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(to, b, 0o755); err != nil {
		t.Fatal(err)
	}
}

func git(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func stageAndLint(t *testing.T, dir, rel, src string) (string, error) {
	t.Helper()
	path := filepath.Join(dir, "apps/backend", rel)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	git(t, dir, "add", ".")
	cmd := exec.Command("bash", "scripts/githooks/lint-staged-go.sh")
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func TestLintStagedGo_blocksTimeNowInAModule(t *testing.T) {
	dir := backendRepoWithHook(t)
	out, err := stageAndLint(t, dir, "internal/modules/foo/foo.go",
		"package foo\n\nimport \"time\"\n\nfunc Stamp() time.Time { return time.Now() }\n")
	if err == nil {
		t.Fatalf("expected the hook to block, out=%s", out)
	}
	if !strings.Contains(out, "forbidigo") || !strings.Contains(out, "time.Now") {
		t.Fatalf("expected a forbidigo time.Now finding, out=%s", out)
	}
}

func TestLintStagedGo_passesCleanModuleAndIgnoresTestdata(t *testing.T) {
	dir := backendRepoWithHook(t)
	out, err := stageAndLint(t, dir, "internal/modules/foo/testdata/bad.go",
		"package bad\n\nimport \"time\"\n\nfunc Stamp() time.Time { return time.Now() }\n")
	if err != nil {
		t.Fatalf("testdata must not be linted, err=%v out=%s", err, out)
	}
	out, err = stageAndLint(t, dir, "internal/modules/foo/foo.go", "package foo\n\nfunc Two() int { return 2 }\n")
	if err != nil {
		t.Fatalf("expected a clean module to pass, err=%v out=%s", err, out)
	}
}

func TestLintStagedGo_blocksABareGoStatementInAModule(t *testing.T) {
	dir := backendRepoWithHook(t)
	out, err := stageAndLint(t, dir, "internal/modules/foo/spawn.go",
		"package foo\n\nfunc Spawn(fn func()) {\n\tgo fn()\n}\n")
	if err == nil {
		t.Fatalf("expected the hook to block, out=%s", out)
	}
	if !strings.Contains(out, "bare go statement") {
		t.Fatalf("expected a nogo finding, out=%s", out)
	}
}

func TestLintStagedGo_blocksACommentInAStagedFile(t *testing.T) {
	dir := backendRepoWithHook(t)
	out, err := stageAndLint(t, dir, "internal/modules/foo/two.go",
		"package foo\n\nfunc Two() int {\n\treturn 2 // hi\n}\n")
	if err == nil {
		t.Fatalf("expected the hook to block, out=%s", out)
	}
	if !strings.Contains(out, "internal/modules/foo/two.go:4: comment not allowed") {
		t.Fatalf("expected a file:line comment finding, out=%s", out)
	}
}
