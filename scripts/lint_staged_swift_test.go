package scripts_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func swiftLintRepo(t *testing.T) string {
	t.Helper()
	root := repoRoot(t)
	dir := t.TempDir()
	copyFile(t, filepath.Join(root, "scripts/githooks/lint-staged-swift.sh"), filepath.Join(dir, "scripts/githooks/lint-staged-swift.sh"))
	copyFile(t, filepath.Join(root, "scripts/swiftlint-ratchet.sh"), filepath.Join(dir, "scripts/swiftlint-ratchet.sh"))
	bin := filepath.Join(dir, "bin")
	if err := os.MkdirAll(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	writeStub(t, filepath.Join(bin, "swift"), swiftFormatStub)
	writeStub(t, filepath.Join(bin, "swiftlint"), swiftlintStub)
	git(t, dir, "init", "-q")
	git(t, dir, "add", ".")
	git(t, dir, "-c", "user.name=t", "-c", "user.email=t@t", "commit", "-q", "-m", "base")
	return dir
}

func writeStub(t *testing.T, path, src string) {
	t.Helper()
	writeExecutable(t, path, src)
}

func stageSwift(t *testing.T, dir, rel, src string) (string, error) {
	t.Helper()
	path := filepath.Join(dir, rel)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	git(t, dir, "add", rel)
	cmd := exec.Command("bash", "scripts/githooks/lint-staged-swift.sh")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "PATH="+filepath.Join(dir, "bin")+":/usr/bin:/bin")
	out, err := cmd.CombinedOutput()
	return string(out), err
}

const swiftPreCommit = "pre-commit: comments or lint findings in staged Swift files. No comments in Swift. Use a better name, a type, a test, or an issue."

func TestLintStagedSwift_passesACleanFileAndIgnoresTheRest(t *testing.T) {
	dir := swiftLintRepo(t)
	out, err := stageSwift(t, dir, "apps/backend/Note.swift", "let bad = BAD_FORMAT\n")
	if err != nil {
		t.Fatalf("swift outside the mobile trees must be ignored, err=%v out=%s", err, out)
	}
	out, err = stageSwift(t, dir, "apps/mobile/Monaco/Ok.swift", "let value = 1\n")
	if err != nil {
		t.Fatalf("expected a clean file to pass, err=%v out=%s", err, out)
	}
	if strings.Contains(out, swiftPreCommit) {
		t.Fatalf("clean file printed the failure line: %s", out)
	}
}

func TestLintStagedSwift_blocksAFormatFinding(t *testing.T) {
	dir := swiftLintRepo(t)
	out, err := stageSwift(t, dir, "packages/mobile-core/Sources/MonacoCore/Bad.swift", "let bad = BAD_FORMAT\n")
	if err == nil || !strings.Contains(out, swiftPreCommit) {
		t.Fatalf("expected the format hook to block, err=%v out=%s", err, out)
	}
}

func TestLintStagedSwift_blocksASwiftlintFinding(t *testing.T) {
	dir := swiftLintRepo(t)
	out, err := stageSwift(t, dir, "apps/mobile/Monaco/Force.swift", "let bad = FORCE_UNWRAP\n")
	if err == nil || !strings.Contains(out, swiftPreCommit) || !strings.Contains(out, "force_unwrapping") {
		t.Fatalf("expected the ratchet to block, err=%v out=%s", err, out)
	}
}

func TestLintStagedSwift_blocksAModifiedFile(t *testing.T) {
	dir := swiftLintRepo(t)
	const rel = "apps/mobile/Monaco/Ok.swift"
	if out, err := stageSwift(t, dir, rel, "let value = 1\n"); err != nil {
		t.Fatalf("a clean new file must pass, err=%v out=%s", err, out)
	}
	git(t, dir, "-c", "user.name=t", "-c", "user.email=t@t", "commit", "-q", "-m", "add Ok")

	out, err := stageSwift(t, dir, rel, "let bad = FORCE_UNWRAP\n")
	if err == nil || !strings.Contains(out, swiftPreCommit) || !strings.Contains(out, "force_unwrapping") {
		t.Fatalf("expected the ratchet to block a modified file, err=%v out=%s", err, out)
	}
}

func TestLintStagedSwift_emptyIndexExitsZero(t *testing.T) {
	dir := swiftLintRepo(t)
	cmd := exec.Command("bash", "scripts/githooks/lint-staged-swift.sh")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "PATH=/usr/bin:/bin")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("nothing staged must pass, err=%v out=%s", err, out)
	}
}

const swiftFormatStub = `#!/bin/bash
file=""
for arg in "$@"; do
  file="$arg"
done
if [[ -f "$file" ]] && grep -q BAD_FORMAT "$file"; then
  echo "$file:1:1: error: [AlwaysUseLowerCamelCase] rename the constant"
  exit 1
fi
if [[ -f "$file" ]] && grep -q SYNTAX_ERROR "$file"; then
  echo "$file:2:13: error: expected expression in 'let' statement"
  exit 1
fi
exit 0
`

const swiftlintStub = `#!/bin/bash
if [[ "${1:-}" == "version" ]]; then
  echo 0.65.0
  exit 0
fi
echo "file,line,character,severity,type,reason,rule_id"
for arg in "$@"; do
  if [[ -f "$arg" ]] && grep -q LINT_CRASH "$arg"; then
    echo "Fatal error: sourcekitd crashed" >&2
    exit 1
  fi
  if [[ -f "$arg" ]] && grep -q FORCE_UNWRAP "$arg"; then
    echo "$arg,1,1,error,force_unwrapping,Force Unwrapping Violation,force_unwrapping"
  fi
done
exit 0
`
