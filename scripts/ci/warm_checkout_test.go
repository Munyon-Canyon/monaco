package ci_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestWarmCheckout_acceptsAnAncestorAndRefusesAStranger(t *testing.T) {
	root := t.TempDir()
	origin := filepath.Join(root, "origin.git")
	work := filepath.Join(root, "work")
	git := func(dir string, args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=mrrobot-ec",
			"GIT_AUTHOR_EMAIL=andrescamp_ac@hotmail.com",
			"GIT_COMMITTER_NAME=mrrobot-ec",
			"GIT_COMMITTER_EMAIL=andrescamp_ac@hotmail.com",
		)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
		}
	}
	git("", "init", "--bare", "-b", "main", origin)
	git("", "clone", origin, work)
	if err := os.WriteFile(filepath.Join(work, "f"), []byte("a\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	git(work, "add", "f")
	git(work, "commit", "-m", "init")
	git(work, "push", "origin", "main")
	git(work, "checkout", "-b", "feature")
	if err := os.WriteFile(filepath.Join(work, "f"), []byte("b\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	git(work, "commit", "-am", "feature")
	git(work, "push", "origin", "feature")
	feature := strings.TrimSpace(mustGit(t, work, "rev-parse", "HEAD"))

	stranger := filepath.Join(root, "stranger")
	git("", "clone", origin, stranger)
	if err := os.WriteFile(filepath.Join(stranger, "f"), []byte("c\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	git(stranger, "commit", "-am", "stranger")
	strangerSHA := strings.TrimSpace(mustGit(t, stranger, "rev-parse", "HEAD"))
	git(stranger, "push", "origin", "HEAD:refs/heads/stranger")
	git(work, "fetch", "origin", "stranger")

	script := filepath.Join(repoRoot(t), "scripts", "ci", "warm-checkout.sh")
	run := func(sha string) (string, error) {
		cmd := exec.Command("bash", script)
		cmd.Dir = work
		cmd.Env = append(os.Environ(), "CHECKOUT_SHA="+sha, "FEATURE_BRANCH=feature")
		out, err := cmd.CombinedOutput()
		return string(out), err
	}
	if out, err := run(feature); err != nil {
		t.Fatalf("ancestor: %v\n%s", err, out)
	}
	if got := strings.TrimSpace(mustGit(t, work, "rev-parse", "HEAD")); got != feature {
		t.Fatalf("checked out %s, want %s", got, feature)
	}
	if out, err := run(strangerSHA); err == nil {
		t.Fatalf("stranger was accepted\n%s", out)
	} else if !strings.Contains(out, "refusing") {
		t.Fatalf("refusal:\n%s", out)
	}
}

func mustGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return string(out)
}
