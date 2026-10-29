package scripts_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

var regenGitEnv = []string{
	"GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null",
	"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@example.com",
	"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@example.com",
}

func regenRun(t *testing.T, dir, name string, args ...string) {
	t.Helper()
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), regenGitEnv...)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("%s %v: %v\n%s", name, args, err, out)
	}
}

// regenRepo is a temp repo whose docs/reference/events.md is the sorted concatenation of src/*.txt,
// as a generator would write it. Branch feature adds two sources in two commits and branch main adds
// one, each regenerating, so rebasing feature onto main conflicts in events.md at each commit.
func regenRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	gen := "mkdir -p docs/reference && sort src/*.txt > docs/reference/events.md"
	commit := func(msg, src, body string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Join(dir, "src"), 0o755); err != nil {
			t.Fatal(err)
		}
		writeTestFile(t, filepath.Join(dir, "src", src), body)
		regenRun(t, dir, "sh", "-c", gen)
		regenRun(t, dir, "git", "add", "-A")
		regenRun(t, dir, "git", "commit", "-q", "-m", msg)
	}
	regenRun(t, dir, "git", "init", "-q", "-b", "main")
	if err := os.MkdirAll(filepath.Join(dir, "apps", "backend"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, filepath.Join(dir, "apps", "backend", "go.mod"), "module example.com/m\n\ngo 1.26\n\ntoolchain go1.26.9\n")
	commit("base", "base.txt", "a\nm\nz\n")
	regenRun(t, dir, "git", "checkout", "-q", "-b", "feature")
	commit("feature one", "feature1.txt", "b\n")
	commit("feature two", "feature2.txt", "n\n")
	regenRun(t, dir, "git", "checkout", "-q", "main")
	commit("main", "main.txt", "c\n")
	regenRun(t, dir, "git", "checkout", "-q", "feature")
	return dir
}

func runRegen(t *testing.T, dir string, extraEnv ...string) (string, error) {
	t.Helper()
	// The fake gt lives outside the repo so the script's git add -A cannot stage it.
	// restack rebases onto main and continue continues the rebase.
	gt := filepath.Join(t.TempDir(), "gt")
	writeExecutable(t, gt, "#!/bin/sh\ncase \"$1\" in restack) git rebase main ;; continue) git rebase --continue ;;"+
		" modify) echo modify >> \"$(dirname \"$0\")/calls\"; git commit -q --amend --no-edit ;; esac\n")
	cmd := exec.Command(filepath.Join(repoRoot(t), "scripts", "restack-regen.sh"))
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), regenGitEnv...)
	cmd.Env = append(cmd.Env, "GT="+gt, "GENERATED_GLOBS=docs/reference/events.md", "MIGRATE_ORDER=true")
	cmd.Env = append(cmd.Env, extraEnv...)
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func TestRestackRegen_settlesAGeneratedOnlyRestack(t *testing.T) {
	t.Parallel()
	dir := regenRepo(t)
	out, err := runRegen(t, dir, "GENERATE=sort src/*.txt > docs/reference/events.md")
	if err != nil {
		t.Fatalf("restack-regen: %v\n%s", err, out)
	}
	if _, err := os.Stat(filepath.Join(dir, ".git", "rebase-merge")); err == nil {
		t.Fatalf("the rebase is still in progress\n%s", out)
	}
	got, err := os.ReadFile(filepath.Join(dir, "docs", "reference", "events.md"))
	if err != nil {
		t.Fatal(err)
	}
	if want := "a\nb\nc\nm\nn\nz\n"; string(got) != want {
		t.Fatalf("events.md = %q, want %q\n%s", got, want, out)
	}
	cmd := exec.Command("git", "log", "--format=%s", "main..feature")
	cmd.Dir = dir
	log, err := cmd.Output()
	if err != nil {
		t.Fatal(err)
	}
	if string(log) != "feature two\nfeature one\n" {
		t.Fatalf("feature's commits = %q", log)
	}
}

func TestRestackRegen_defaultGenerateRunsOnTheToolchainGoModPins(t *testing.T) {
	t.Parallel()
	dir := regenRepo(t)
	bin := t.TempDir()
	seen := filepath.Join(t.TempDir(), "toolchain")
	writeExecutable(t, filepath.Join(bin, "go"), "#!/bin/sh\nroot=$(git rev-parse --show-toplevel)\nprintf '%s' \"$GOTOOLCHAIN\" > \"$SEEN\"\nsort \"$root\"/src/*.txt > \"$root\"/docs/reference/events.md\n")
	out, err := runRegen(t, dir, "PATH="+bin+":"+os.Getenv("PATH"), "SEEN="+seen)
	if err != nil {
		t.Fatalf("restack-regen: %v\n%s", err, out)
	}
	got, err := os.ReadFile(seen)
	if err != nil || string(got) != "go1.26.9" {
		t.Fatalf("go generate ran with GOTOOLCHAIN=%q (%v)\n%s", got, err, out)
	}
}

func TestRestackRegen_stopsOnAHandWrittenConflict(t *testing.T) {
	t.Parallel()
	dir := regenRepo(t)
	// Both sides add notes.txt, so the last feature commit conflicts in a hand-written file.
	regenRun(t, dir, "git", "checkout", "-q", "main")
	writeTestFile(t, filepath.Join(dir, "notes.txt"), "main\n")
	regenRun(t, dir, "git", "add", "notes.txt")
	regenRun(t, dir, "git", "commit", "-q", "-m", "main notes")
	regenRun(t, dir, "git", "checkout", "-q", "feature")
	writeTestFile(t, filepath.Join(dir, "notes.txt"), "feature\n")
	regenRun(t, dir, "git", "add", "notes.txt")
	regenRun(t, dir, "git", "commit", "-q", "-m", "feature notes")

	out, err := runRegen(t, dir)
	if err == nil {
		t.Fatalf("restack-regen succeeded on a hand-written conflict\n%s", out)
	}
	if !strings.Contains(out, "not generated") || !strings.Contains(out, "notes.txt") {
		t.Fatalf("output does not name the hand-written path:\n%s", out)
	}
	if _, err := os.Stat(filepath.Join(dir, ".git", "rebase-merge")); err != nil {
		t.Fatalf("the rebase should stay stopped for a hand merge: %v", err)
	}
}

// migrationRepo is regenRepo plus a migration on feature that main overtakes with a later one.
func migrationRepo(t *testing.T) string {
	t.Helper()
	dir := regenRepo(t)
	mig := filepath.Join(dir, "apps", "backend", "migrations")
	regenRun(t, dir, "git", "checkout", "-q", "main")
	if err := os.MkdirAll(mig, 0o755); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, filepath.Join(mig, "20261029090000_trunk.sql"), "select 1;\n")
	regenRun(t, dir, "git", "add", "-A")
	regenRun(t, dir, "git", "commit", "-q", "-m", "trunk migration")
	regenRun(t, dir, "git", "checkout", "-q", "feature")
	if err := os.MkdirAll(mig, 0o755); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, filepath.Join(mig, "20261029080000_mine.sql"), "select 2;\n")
	regenRun(t, dir, "git", "add", "-A")
	regenRun(t, dir, "git", "commit", "-q", "-m", "my migration")
	return dir
}

// The fake order check fails while 20261029080000_mine.sql exists; the fake rebase renames it past the trunk's.
const (
	fakeOrder  = "MIGRATE_ORDER=test ! -e apps/backend/migrations/20261029080000_mine.sql || { echo 'adds 20261029080000_mine.sql at or below 20261029090000_trunk.sql'; exit 1; }"
	fakeRebase = "MIGRATE_REBASE=git mv apps/backend/migrations/20261029080000_mine.sql apps/backend/migrations/20261029090001_mine.sql"
)

func TestRestackRegen_rebasesAMigrationTheTrunkOvertook(t *testing.T) {
	t.Parallel()
	dir := migrationRepo(t)
	out, err := runRegen(t, dir, "GENERATE=sort src/*.txt > docs/reference/events.md", fakeOrder, fakeRebase)
	if err != nil {
		t.Fatalf("restack-regen: %v\n%s", err, out)
	}
	for _, want := range []string{
		"the restack left a migration out of order", "adds 20261029080000_mine.sql at or below 20261029090000_trunk.sql",
		"renamed:", "20261029090001_mine.sql",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("output lacks %q:\n%s", want, out)
		}
	}
	tree, err := exec.Command("git", "-C", dir, "ls-tree", "--name-only", "HEAD", "apps/backend/migrations/").Output()
	if err != nil || !strings.Contains(string(tree), "20261029090001_mine.sql") || strings.Contains(string(tree), "080000") {
		t.Fatalf("HEAD migrations %q %v", tree, err)
	}
}

func TestRestackRegen_leavesAnInOrderMigrationAlone(t *testing.T) {
	t.Parallel()
	dir := regenRepo(t)
	out, err := runRegen(t, dir, "GENERATE=sort src/*.txt > docs/reference/events.md", "MIGRATE_REBASE=false")
	if err != nil || strings.Contains(out, "out of order") || strings.Contains(out, "renamed") {
		t.Fatalf("restack-regen: %v\n%s", err, out)
	}
}

func TestRestackRegen_stopsWhenTheRenameDoesNotFixTheOrder(t *testing.T) {
	t.Parallel()
	dir := migrationRepo(t)
	out, err := runRegen(t, dir, "GENERATE=sort src/*.txt > docs/reference/events.md",
		"MIGRATE_ORDER=echo 'a lower branch adds it'; exit 1", "MIGRATE_REBASE=true")
	if err == nil || !strings.Contains(out, "still out of order after the rename") {
		t.Fatalf("restack-regen: %v\n%s", err, out)
	}
}
