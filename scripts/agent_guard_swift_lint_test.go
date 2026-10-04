package scripts_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func swiftGuardRepo(t *testing.T, withLint bool) string {
	t.Helper()
	root := repoRoot(t)
	dir := t.TempDir()
	copyFile(t, filepath.Join(root, "scripts/agent-guard-swift-lint.sh"), filepath.Join(dir, "scripts/agent-guard-swift-lint.sh"))
	copyFile(t, filepath.Join(root, "scripts/swiftlint-ratchet.sh"), filepath.Join(dir, "scripts/swiftlint-ratchet.sh"))
	writeStub(t, filepath.Join(dir, "scripts/require-docker.sh"), "#!/bin/bash\nexit 1\n")
	git(t, dir, "init", "-q")
	git(t, dir, "add", ".")
	git(t, dir, "-c", "user.name=t", "-c", "user.email=t@t", "commit", "-q", "-m", "base")
	bin := filepath.Join(dir, "bin")
	if err := os.MkdirAll(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	writeStub(t, filepath.Join(bin, "swift"), swiftFormatStub)
	if withLint {
		writeStub(t, filepath.Join(bin, "swiftlint"), swiftlintStub)
	}
	files := map[string]string{
		"apps/mobile/Monaco/Bad.swift":                        "let groups_path = BAD_FORMAT\n",
		"apps/mobile/Monaco/Crash.swift":                      "let x = LINT_CRASH\n",
		"apps/mobile/Monaco/Force.swift":                      "let bad = FORCE_UNWRAP\n",
		"apps/mobile/Monaco/Ok.swift":                         "let value = 1\n",
		"apps/mobile/Monaco/Notes.md":                         "not swift\n",
		"apps/mobile/Monaco/Syntax.swift":                     "struct A {\n    let x = SYNTAX_ERROR\n",
		"packages/mobile-core/Sources/MonacoCore/Force.swift": "let bad = FORCE_UNWRAP\n",
		"scripts/Tool.swift":                                  "let bad = BAD_FORMAT\n",
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

func runSwiftGuard(t *testing.T, dir, input string) (int, string, string) {
	t.Helper()
	return runShellHook(t, dir, "agent-guard-swift-lint.sh", input, "PATH="+filepath.Join(dir, "bin")+":/usr/bin:/bin")
}

func TestAgentGuardSwiftLint_claudeCodeGetsExit2WithTheOffendingLine(t *testing.T) {
	dir := swiftGuardRepo(t, true)
	want := "apps/mobile/Monaco/Bad.swift:1: AlwaysUseLowerCamelCase: rename the constant: let groups_path = BAD_FORMAT"

	code, stdout, stderr := runSwiftGuard(t, dir, claudeInput(filepath.Join(dir, "apps/mobile/Monaco/Bad.swift")))
	if code != 2 || stdout != "" || stderr != want+"\n" {
		t.Fatalf("exit=%d stdout=%q stderr=%q, want 2 and %q", code, stdout, stderr, want)
	}
}

func TestAgentGuardSwiftLint_cursorGetsTheOffendingLineAsAdditionalContext(t *testing.T) {
	dir := swiftGuardRepo(t, true)
	want := "apps/mobile/Monaco/Force.swift:1: force_unwrapping: Force Unwrapping Violation: let bad = FORCE_UNWRAP"

	code, stdout, stderr := runSwiftGuard(t, dir, cursorInput(filepath.Join(dir, "apps/mobile/Monaco/Force.swift")))
	var got map[string]string
	if err := json.Unmarshal([]byte(stdout), &got); err != nil {
		t.Fatalf("stdout %q is not JSON: %v", stdout, err)
	}
	if code != 0 || stderr != "" || got["additional_context"] != want {
		t.Fatalf("exit=%d stderr=%q output=%v, want additional_context=%q", code, stderr, got, want)
	}
}

func TestAgentGuardSwiftLint_lintsPackagesMobileCore(t *testing.T) {
	dir := swiftGuardRepo(t, true)
	want := "packages/mobile-core/Sources/MonacoCore/Force.swift:1: force_unwrapping: Force Unwrapping Violation: let bad = FORCE_UNWRAP"

	code, stdout, stderr := runSwiftGuard(t, dir, claudeInput(filepath.Join(dir, "packages/mobile-core/Sources/MonacoCore/Force.swift")))
	if code != 2 || stdout != "" || stderr != want+"\n" {
		t.Fatalf("exit=%d stdout=%q stderr=%q, want 2 and %q", code, stdout, stderr, want)
	}
}

func TestAgentGuardSwiftLint_reportsASwiftlintCrash(t *testing.T) {
	dir := swiftGuardRepo(t, true)
	want := "Fatal error: sourcekitd crashed\nswiftlint-ratchet: swiftlint exited 1\n"

	code, stdout, stderr := runSwiftGuard(t, dir, claudeInput(filepath.Join(dir, "apps/mobile/Monaco/Crash.swift")))
	if code != 2 || stdout != "" || stderr != want {
		t.Fatalf("exit=%d stdout=%q stderr=%q, want 2 and %q", code, stdout, stderr, want)
	}
}

func TestAgentGuardSwiftLint_checksAnEditInALinkedWorktree(t *testing.T) {
	dir := swiftGuardRepo(t, true)
	const rel = "apps/mobile/Monaco/Lane.swift"
	path := fileInLinkedWorktree(t, dir, rel, "let groups_path = BAD_FORMAT\n")
	want := rel + ":1: AlwaysUseLowerCamelCase: rename the constant: let groups_path = BAD_FORMAT"

	code, stdout, stderr := runSwiftGuard(t, dir, claudeInput(path))
	if code != 2 || stdout != "" || stderr != want+"\n" {
		t.Fatalf("exit=%d stdout=%q stderr=%q, want 2 and %q", code, stdout, stderr, want)
	}
}

func TestAgentGuardSwiftLint_allowsCleanNonSwiftAndOutside(t *testing.T) {
	dir := swiftGuardRepo(t, true)
	for name, path := range map[string]string{
		"clean swift":      filepath.Join(dir, "apps/mobile/Monaco/Ok.swift"),
		"non-swift":        filepath.Join(dir, "apps/mobile/Monaco/Notes.md"),
		"swift outside":    filepath.Join(dir, "scripts/Tool.swift"),
		"deleted file":     filepath.Join(dir, "apps/mobile/Monaco/Gone.swift"),
		"outside the repo": "/etc/hosts",
		"another clone":    fileInAnotherClone(t, dir, "apps/mobile/Monaco/Bad.swift", "let groups_path = BAD_FORMAT\n"),
	} {
		t.Run(name, func(t *testing.T) {
			code, stdout, stderr := runSwiftGuard(t, dir, claudeInput(path))
			if code != 0 || stdout != "" || stderr != "" {
				t.Fatalf("claude: exit=%d stdout=%q stderr=%q", code, stdout, stderr)
			}
			code, stdout, stderr = runSwiftGuard(t, dir, cursorInput(path))
			if code != 0 || stdout != "{}\n" || stderr != "" {
				t.Fatalf("cursor: exit=%d stdout=%q stderr=%q", code, stdout, stderr)
			}
		})
	}
}

func TestAgentGuardSwiftLint_reportsAnUnparsableFormatError(t *testing.T) {
	dir := swiftGuardRepo(t, true)
	want := "apps/mobile/Monaco/Syntax.swift:2:13: error: expected expression in 'let' statement"

	code, stdout, stderr := runSwiftGuard(t, dir, claudeInput(filepath.Join(dir, "apps/mobile/Monaco/Syntax.swift")))
	if code != 2 || stdout != "" || stderr != want+"\n" {
		t.Fatalf("exit=%d stdout=%q stderr=%q, want 2 and %q", code, stdout, stderr, want)
	}
}

func TestAgentGuardSwiftLint_reportsWhenSwiftlintCannotRun(t *testing.T) {
	dir := swiftGuardRepo(t, false)
	code, stdout, stderr := runSwiftGuard(t, dir, claudeInput(filepath.Join(dir, "apps/mobile/Monaco/Ok.swift")))
	if code != 2 || stdout != "" || stderr != "swiftlint skipped: start Docker\n" {
		t.Fatalf("exit=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
}
