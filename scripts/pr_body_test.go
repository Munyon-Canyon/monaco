package scripts_test

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

type prBodySandbox struct {
	repo, calls, view string
	env               []string
}

func newPrBodySandbox(t *testing.T) prBodySandbox {
	t.Helper()
	t.Setenv("PYENV_VERSION", "system")
	dir := t.TempDir()
	s := prBodySandbox{
		repo:  filepath.Join(dir, "repo"),
		calls: filepath.Join(dir, "calls"),
		view:  filepath.Join(dir, "view"),
	}
	writeExecutable(t, filepath.Join(dir, "bin", "gh"), fmt.Sprintf(`#!/bin/sh
echo "$*" >> %q
case "$1 $2" in
  "pr view") cat %q ;;
  "pr list") echo '[]' ;;
  "pr edit"|"pr ready") ;;
  *) echo "unexpected: $*" >&2; exit 1 ;;
esac
`, s.calls, s.view))
	s.env = append(os.Environ(), "PATH="+filepath.Join(dir, "bin")+":"+os.Getenv("PATH"))
	git(t, dir, "init", "-q", "-b", "main", s.repo)
	commitFile(t, s.repo, "base.txt", "chore: base")
	return s
}

func (s prBodySandbox) head(t *testing.T) string {
	t.Helper()
	return strings.TrimSpace(gitOut(t, s.repo, "rev-parse", "HEAD"))
}

func (s prBodySandbox) run(t *testing.T, draft bool, base, title, body string) (string, error) {
	t.Helper()
	view := fmt.Sprintf("%t\tbackend-rewrite-3\tticket\t%s\t%s\n", draft, base, s.head(t))
	if err := os.WriteFile(s.view, []byte(view), 0o600); err != nil {
		t.Fatal(err)
	}
	_ = os.Remove(s.calls)
	file := filepath.Join(s.repo, "..", "body.md")
	if err := os.WriteFile(file, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("bash", filepath.Join(repoRoot(t), "scripts", "pr-body.sh"), "7", title, file)
	cmd.Dir = s.repo
	cmd.Env = s.env
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func (s prBodySandbox) ghCalls(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile(s.calls)
	if err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	return string(b)
}

func goodPrBody() string {
	var b strings.Builder
	for _, s := range []string{"TLDR", "Why", "What changed", "Proof", "What came up", "Reviewer focus"} {
		text := "Text."
		if s == "Why" {
			text = "Part of #7."
		}
		fmt.Fprintf(&b, "## %s\n\n%s\n\n", s, text)
	}
	return b.String()
}

func TestPrBody_marksADraftReadyOnlyAfterTheFullPrFormatCheckPasses(t *testing.T) {
	s := newPrBodySandbox(t)
	base := s.head(t)
	commitFile(t, s.repo, "a.txt", "feat(bus): add the thing")

	template := readRepo(t, repoRoot(t), ".github/pull_request_template.md")
	failures := []struct{ name, title, body, want string }{
		{"commit-subject title", "feat(bus): add the thing", goodPrBody(), "commit-type prefix"},
		{"empty template", "Add the thing", template, `"## TLDR" is empty`},
		{"no ticket link", "Add the thing", strings.Replace(goodPrBody(), "Part of #7.", "Text.", 1), "links no ticket"},
	}
	for _, f := range failures {
		out, err := s.run(t, true, base, f.title, f.body)
		if err == nil || !strings.Contains(out, f.want) {
			t.Errorf("%s: want a failure naming %q, got err=%v out=%q", f.name, f.want, err, out)
		}
		if calls := s.ghCalls(t); strings.Contains(calls, "pr edit") || strings.Contains(calls, "pr ready") {
			t.Errorf("%s: gh changed the PR after a failed check: %q", f.name, calls)
		}
	}

	commitFile(t, s.repo, "b.txt", "Add a thing")
	if out, err := s.run(t, true, base, "Add the thing", goodPrBody()); err == nil ||
		!strings.Contains(out, `"Add a thing" is not a Conventional Commit`) {
		t.Fatalf("want the commit check from base..head, got err=%v out=%q", err, out)
	}
	if calls := s.ghCalls(t); strings.Contains(calls, "pr edit") || strings.Contains(calls, "pr ready") {
		t.Fatalf("gh changed the PR after a failed commit check: %q", calls)
	}
}

func TestPrBody_setsTitleAndBodyThenMarksOnlyADraftReady(t *testing.T) {
	s := newPrBodySandbox(t)
	base := s.head(t)
	commitFile(t, s.repo, "a.txt", "feat(bus): add the thing")
	file := filepath.Join(s.repo, "..", "body.md")

	if out, err := s.run(t, true, base, "Add the thing", goodPrBody()); err != nil {
		t.Fatalf("pr-body.sh: %v\n%s", err, out)
	}
	calls := s.ghCalls(t)
	edit := strings.Index(calls, "pr edit 7 --title Add the thing --body-file "+file+"\n")
	ready := strings.Index(calls, "pr ready 7\n")
	if edit < 0 || ready < edit {
		t.Fatalf("want gh pr edit with title and body, then gh pr ready; got %q", calls)
	}

	if out, err := s.run(t, false, base, "Add the thing", goodPrBody()); err != nil {
		t.Fatalf("pr-body.sh on a ready PR: %v\n%s", err, out)
	}
	if calls := s.ghCalls(t); !strings.Contains(calls, "pr edit 7 --title") || strings.Contains(calls, "pr ready") {
		t.Fatalf("a ready PR gets its title and body and no second gh pr ready; got %q", calls)
	}
}
