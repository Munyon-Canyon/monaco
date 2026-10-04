package scripts_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

const commentsHookMessage = "No comments in Go. Use a better name, a type, a test, or an issue."

func commentsHookRepo(t *testing.T) string {
	t.Helper()
	root := repoRoot(t)
	dir := t.TempDir()
	for _, rel := range []string{
		"scripts/agent-guard-go-comments.sh",
		"apps/backend/go.mod",
		"apps/backend/go.sum",
	} {
		copyFile(t, filepath.Join(root, rel), filepath.Join(dir, rel))
	}
	copyBackendPackages(t, root, dir, "./cmd/monacoctl")
	git(t, dir, "init", "-q")
	git(t, dir, "add", ".")
	git(t, dir, "-c", "user.name=t", "-c", "user.email=t@t", "-c", "commit.gpgsign=false", "commit", "-q", "-m", "base")
	files := map[string]string{
		"apps/backend/internal/modules/foo/bad.go":         "package foo\n\nfunc Two() int {\n\treturn 2 // two\n}\n",
		"apps/backend/internal/modules/foo/ok.go":          "package foo\n\nfunc Three() int { return 3 }\n",
		"apps/backend/internal/modules/foo/testdata/fx.go": "package fx\n\n// fixture\n",
		"apps/backend/notes.md":                            "// not go\n",
		"scripts/tool.go":                                  "package tool\n\n// outside the backend\n",
	}
	for rel, src := range files {
		path := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	resolved, err := filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatal(err)
	}
	return resolved
}

func runCommentsHook(t *testing.T, dir, input string) (int, string, string) {
	t.Helper()
	return runShellHook(t, dir, "agent-guard-go-comments.sh", input)
}

func claudeInput(path string) string {
	return `{"hook_event_name":"PostToolUse","tool_name":"Edit","tool_input":{"file_path":"` + path + `"}}`
}

func cursorInput(path string) string {
	return `{"hook_event_name":"postToolUse","tool_name":"Write","tool_input":{"file_path":"` + path + `"},"workspace_roots":["/x"]}`
}

const badFinding = "apps/backend/internal/modules/foo/bad.go:4: comment not allowed: return 2 // two\n" + commentsHookMessage

func TestAgentGuardGoComments_claudeCodeGetsExit2WithTheOffendingLineOnStderr(t *testing.T) {
	t.Parallel()
	dir := commentsHookRepo(t)

	code, stdout, stderr := runCommentsHook(t, dir, claudeInput(filepath.Join(dir, "apps/backend/internal/modules/foo/bad.go")))

	if code != 2 || stdout != "" || stderr != badFinding+"\n" {
		t.Fatalf("exit=%d stdout=%q stderr=%q, want 2, no stdout, %q", code, stdout, stderr, badFinding+"\n")
	}
}

func TestAgentGuardGoComments_cursorGetsTheOffendingLineAsAdditionalContext(t *testing.T) {
	t.Parallel()
	dir := commentsHookRepo(t)

	code, stdout, stderr := runCommentsHook(t, dir, cursorInput(filepath.Join(dir, "apps/backend/internal/modules/foo/bad.go")))

	var got map[string]string
	if err := json.Unmarshal([]byte(stdout), &got); err != nil {
		t.Fatalf("stdout %q is not JSON: %v", stdout, err)
	}
	if code != 0 || stderr != "" || len(got) != 1 || got["additional_context"] != badFinding {
		t.Fatalf("exit=%d stderr=%q output=%v, want 0, no stderr, only additional_context=%q", code, stderr, got, badFinding)
	}
}

func TestAgentGuardGoComments_checksAnEditInALinkedWorktree(t *testing.T) {
	t.Parallel()
	dir := commentsHookRepo(t)
	const rel = "apps/backend/internal/modules/foo/lane.go"
	path := fileInLinkedWorktree(t, dir, rel, "package foo\n\nfunc Four() int {\n\treturn 4 // four\n}\n")

	code, stdout, stderr := runCommentsHook(t, dir, claudeInput(path))

	want := rel + ":4: comment not allowed: return 4 // four\n" + commentsHookMessage + "\n"
	if code != 2 || stdout != "" || stderr != want {
		t.Fatalf("exit=%d stdout=%q stderr=%q, want 2, no stdout, %q", code, stdout, stderr, want)
	}
}

func TestAgentGuardGoComments_allowsEverythingElse(t *testing.T) {
	t.Parallel()
	dir := commentsHookRepo(t)
	other := fileInAnotherClone(t, dir, "apps/backend/internal/modules/foo/bad.go", "package foo\n\nfunc Two() int {\n\treturn 2 // two\n}\n")
	for name, path := range map[string]string{
		"clean backend go file": filepath.Join(dir, "apps/backend/internal/modules/foo/ok.go"),
		"backend testdata":      filepath.Join(dir, "apps/backend/internal/modules/foo/testdata/fx.go"),
		"non-go backend file":   filepath.Join(dir, "apps/backend/notes.md"),
		"go outside backend":    filepath.Join(dir, "scripts/tool.go"),
		"deleted file":          filepath.Join(dir, "apps/backend/gone.go"),
		"outside the repo":      "/etc/hosts",
		"another clone":         other,
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			code, stdout, stderr := runCommentsHook(t, dir, claudeInput(path))
			if code != 0 || stdout != "" || stderr != "" {
				t.Fatalf("claude: exit=%d stdout=%q stderr=%q, want 0 and no output", code, stdout, stderr)
			}
			code, stdout, stderr = runCommentsHook(t, dir, cursorInput(path))
			if code != 0 || stdout != "{}\n" || stderr != "" {
				t.Fatalf("cursor: exit=%d stdout=%q stderr=%q, want 0 and {}", code, stdout, stderr)
			}
		})
	}
}
