package scripts_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

type prBodySandbox struct {
	repo, calls, view string
	mergeableReads    []string
	env               []string
}

func newPrBodySandbox(t *testing.T) prBodySandbox {
	t.Helper()
	t.Setenv("PYENV_VERSION", "system")
	if _, err := exec.LookPath("jq"); err != nil {
		t.Fatalf("the fake gh runs the script's --jq filter through jq, as gh does: %v", err)
	}
	dir := t.TempDir()
	s := prBodySandbox{
		repo:  filepath.Join(dir, "repo"),
		calls: filepath.Join(dir, "calls"),
		view:  filepath.Join(dir, "view"),
	}
	writeExecutable(t, filepath.Join(dir, "bin", "sleep"), fmt.Sprintf("#!/bin/sh\necho \"sleep $*\" >> %q\n", s.calls))
	writeExecutable(t, filepath.Join(dir, "bin", "gh"), fmt.Sprintf(`#!/bin/sh
calls=%q
view=%q
echo "$*" >> "$calls"
method=GET
path=
filter=
prev=
for arg in "$@"; do
  case "$prev" in
    -X) method=$arg ;;
    --jq) filter=$arg ;;
  esac
  case "$arg" in
    repos/*) path=$arg ;;
  esac
  prev=$arg
done
base_ref=$(sed -n 1p "$view" | jq -r .base.ref)
head_ref=$(sed -n 1p "$view" | jq -r .head.ref)
case "$1" in
  api)
    case "$method $path" in
      "GET repos/o/r/pulls/7")
        n=$(grep -c '^api repos/o/r/pulls/7 ' "$calls")
        line=$(sed -n "${n}p" "$view")
        printf '%%s\n' "${line:-$(tail -n 1 "$view")}" | jq -r "$filter" ;;
      "GET repos/o/r/pulls?state=open&per_page=100&base=$head_ref") echo '[{"number":8,"body":"Part of #99"}]' ;;
      "GET repos/o/r/pulls?state=open&per_page=100&head=o:$base_ref") echo '[{"number":9,"body":"Part of #99"}]' ;;
      "PATCH repos/o/r/pulls/7") ;;
      "POST repos/o/r/pulls/7/ccr/ready_for_review") ;;
      *) echo "unexpected api: $*" >&2; exit 1 ;;
    esac
    ;;
  pr)
    case "$2" in
      ready)
        if [ -n "${GH_READY_MODE:-}" ]; then
          echo "$GH_READY_MODE" >&2
          exit 1
        fi
        ;;
      list) echo 'HTTP 403: GraphQL is not permitted' >&2; exit 1 ;;
      *) echo "unexpected: $*" >&2; exit 1 ;;
    esac
    ;;
  repo)
    case "$2" in
      view) echo 'o/r' ;;
      *) echo "unexpected: $*" >&2; exit 1 ;;
    esac
    ;;
  *) echo "unexpected: $*" >&2; exit 1 ;;
esac
`, s.calls, s.view))
	env := make([]string, 0, len(os.Environ())+2)
	for _, e := range os.Environ() {
		if strings.HasPrefix(e, "GH_REPO=") || strings.HasPrefix(e, "PATH=") {
			continue
		}
		env = append(env, e)
	}
	s.env = append(env, "PATH="+filepath.Join(dir, "bin")+":"+os.Getenv("PATH"), "GH_REPO=o/r")
	git(t, dir, "init", "-q", "-b", "main", s.repo)
	commitFile(t, s.repo, "base.txt", "chore: base")
	return s
}

func (s prBodySandbox) head(t *testing.T) string {
	t.Helper()
	return strings.TrimSpace(gitOut(t, s.repo, "rev-parse", "HEAD"))
}

func (s prBodySandbox) withoutRepo() prBodySandbox {
	env := make([]string, 0, len(s.env))
	for _, e := range s.env {
		if !strings.HasPrefix(e, "GH_REPO=") {
			env = append(env, e)
		}
	}
	s.env = env
	return s
}

func (s prBodySandbox) run(t *testing.T, draft bool, base, title, body string) (string, error) {
	t.Helper()
	return s.runWith(t, draft, base, title, body, nil)
}

func prJSON(draft bool, base, head, mergeable string) string {
	state := map[string]string{"true": "clean", "false": "dirty", "null": "unknown"}[mergeable]
	return fmt.Sprintf(`{"draft":%t,"base":{"ref":"backend-rewrite-9","sha":%q},"head":{"ref":"ticket","sha":%q},"mergeable":%s,"mergeable_state":%q}`+"\n",
		draft, base, head, mergeable, state)
}

func (s prBodySandbox) runWith(t *testing.T, draft bool, base, title, body string, extra []string) (string, error) {
	t.Helper()
	answers := s.mergeableReads
	if len(answers) == 0 {
		answers = []string{"true"}
	}
	var view strings.Builder
	for _, mergeable := range answers {
		view.WriteString(prJSON(draft, base, s.head(t), mergeable))
	}
	if err := os.WriteFile(s.view, []byte(view.String()), 0o600); err != nil {
		t.Fatal(err)
	}
	_ = os.Remove(s.calls)
	file := filepath.Join(s.repo, "..", "body.md")
	if err := os.WriteFile(file, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "bash", filepath.Join(repoRoot(t), "scripts", "pr-body.sh"), "7", title, file)
	cmd.Dir = s.repo
	cmd.Env = append(append([]string{}, s.env...), extra...)
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
		if calls := s.ghCalls(t); changedPR(calls) {
			t.Errorf("%s: gh changed the PR after a failed check: %q", f.name, calls)
		}
	}

	commitFile(t, s.repo, "b.txt", "Add a thing")
	if out, err := s.run(t, true, base, "Add the thing", goodPrBody()); err == nil ||
		!strings.Contains(out, `"Add a thing" is not a Conventional Commit`) {
		t.Fatalf("want the commit check from base..head, got err=%v out=%q", err, out)
	}
	if calls := s.ghCalls(t); changedPR(calls) {
		t.Fatalf("gh changed the PR after a failed commit check: %q", calls)
	}
}

func changedPR(calls string) bool {
	return strings.Contains(calls, "-X PATCH") || strings.Contains(calls, "pr ready") || strings.Contains(calls, "ccr/ready_for_review")
}

func usedGraphQLList(calls string) bool {
	return strings.Contains(calls, "pr list")
}

func exitCode(err error) int {
	var exit *exec.ExitError
	switch {
	case err == nil:
		return 0
	case errors.As(err, &exit):
		return exit.ExitCode()
	default:
		return -1
	}
}

func pullReads(calls string) int {
	return strings.Count("\n"+calls, "\napi repos/o/r/pulls/7 ")
}

func sleptSeconds(t *testing.T, calls string) int {
	t.Helper()
	total := 0
	for _, line := range strings.Split(calls, "\n") {
		secs, ok := strings.CutPrefix(line, "sleep ")
		if !ok {
			continue
		}
		n, err := strconv.Atoi(secs)
		if err != nil {
			t.Fatalf("unexpected sleep call %q", line)
		}
		total += n
	}
	return total
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
	if usedGraphQLList(calls) {
		t.Fatalf("the format check must read stack neighbours through REST, got %q", calls)
	}
	edit := strings.Index(calls, "api -X PATCH repos/o/r/pulls/7 -f title=Add the thing -F body=@"+file+"\n")
	ready := strings.Index(calls, "pr ready 7\n")
	if ready < 0 || edit < ready || strings.Contains(calls, "repo view") {
		t.Fatalf("want gh pr ready, then the REST patch, so the edit's PR format run is not skipped; got %q", calls)
	}

	if out, err := s.run(t, false, base, "Add the thing", goodPrBody()); err != nil {
		t.Fatalf("pr-body.sh on a ready PR: %v\n%s", err, out)
	}
	if calls := s.ghCalls(t); !strings.Contains(calls, "api -X PATCH repos/o/r/pulls/7 -f title=") || strings.Contains(calls, "pr ready") {
		t.Fatalf("a ready PR gets its title and body and no second gh pr ready; got %q", calls)
	}
}

func TestPrBody_marksReadyThroughCCRWhenPrReadyIsRefused(t *testing.T) {
	s := newPrBodySandbox(t)
	base := s.head(t)
	commitFile(t, s.repo, "a.txt", "feat(bus): add the thing")
	for _, mode := range []string{"403 Forbidden", "graphql: not permitted"} {
		out, err := s.runWith(t, true, base, "Add the thing", goodPrBody(), []string{"GH_READY_MODE=" + mode})
		if err != nil {
			t.Fatalf("pr-body.sh with %q: %v\n%s", mode, err, out)
		}
		calls := s.ghCalls(t)
		ready := strings.Index(calls, "pr ready 7\n")
		ccr := strings.Index(calls, "api -X POST repos/o/r/pulls/7/ccr/ready_for_review\n")
		patch := strings.Index(calls, "api -X PATCH repos/o/r/pulls/7 ")
		if ready < 0 || ccr < ready || patch < ccr || strings.Contains(calls, "pr view") || strings.Contains(calls, "pr edit") {
			t.Fatalf("mode %q: want pr ready refused, then ccr/ready_for_review, then the patch; got %q", mode, calls)
		}
	}
}

func TestPrBody_readyFailureThatIsNotARefusalStaysAFailure(t *testing.T) {
	s := newPrBodySandbox(t)
	base := s.head(t)
	commitFile(t, s.repo, "a.txt", "feat(bus): add the thing")
	out, err := s.runWith(t, true, base, "Add the thing", goodPrBody(), []string{"GH_READY_MODE=validation failed"})
	if err == nil || !strings.Contains(out, "validation failed") {
		t.Fatalf("want the ready error, got err=%v out=%q", err, out)
	}
	if calls := s.ghCalls(t); strings.Contains(calls, "ccr/ready_for_review") || strings.Contains(calls, "-X PATCH") {
		t.Fatalf("a non-refusal ready failure must not call ccr or patch: %q", calls)
	}
}

func TestPrBody_readsTheRepoWhenGHRepoIsUnset(t *testing.T) {
	s := newPrBodySandbox(t).withoutRepo()
	base := s.head(t)
	commitFile(t, s.repo, "a.txt", "feat(bus): add the thing")
	out, err := s.run(t, false, base, "Add the thing", goodPrBody())
	if err != nil {
		t.Fatalf("pr-body.sh: %v\n%s", err, out)
	}
	calls := s.ghCalls(t)
	view := strings.Index(calls, "repo view --json nameWithOwner --jq .nameWithOwner\n")
	get := strings.Index(calls, "api repos/o/r/pulls/7 ")
	if view < 0 || get < view {
		t.Fatalf("want repo view before the pull read; got %q", calls)
	}
}

const conflictMessage = "PR 7 conflicts with its base; restack before it can get checks"

func TestPrBody_refusesAConflictingPRBeforeAnythingChanges(t *testing.T) {
	s := newPrBodySandbox(t)
	base := s.head(t)
	commitFile(t, s.repo, "a.txt", "feat(bus): add the thing")
	s.mergeableReads = []string{"false"}
	for _, draft := range []bool{true, false} {
		out, err := s.run(t, draft, base, "Add the thing", goodPrBody())
		calls := s.ghCalls(t)
		if exitCode(err) != 1 || !strings.Contains(out, conflictMessage) {
			t.Errorf("draft=%t: want exit 1 and %q, got err=%v out=%q", draft, conflictMessage, err, out)
		}
		if changedPR(calls) || sleptSeconds(t, calls) != 0 {
			t.Errorf("draft=%t: a conflict is final, so gh must change nothing and the script must not wait: %q", draft, calls)
		}
	}
}

func TestPrBody_waitsForGitHubToComputeMergeabilityBeforeDeciding(t *testing.T) {
	s := newPrBodySandbox(t)
	base := s.head(t)
	commitFile(t, s.repo, "a.txt", "feat(bus): add the thing")

	s.mergeableReads = []string{"null", "null", "false"}
	out, err := s.run(t, true, base, "Add the thing", goodPrBody())
	calls := s.ghCalls(t)
	if exitCode(err) != 1 || !strings.Contains(out, conflictMessage) || changedPR(calls) || pullReads(calls) != 3 {
		t.Fatalf("null, null, false: want 3 reads, then the refusal; got err=%v out=%q calls=%q", err, out, calls)
	}

	s.mergeableReads = []string{"null", "null", "true"}
	if out, err := s.run(t, true, base, "Add the thing", goodPrBody()); err != nil {
		t.Fatalf("null, null, true: %v\n%s", err, out)
	}
	calls = s.ghCalls(t)
	if pullReads(calls) != 3 || sleptSeconds(t, calls) == 0 ||
		!strings.Contains(calls, "pr ready 7\n") || !strings.Contains(calls, "api -X PATCH repos/o/r/pulls/7 ") {
		t.Fatalf("null, null, true: want 3 reads with a wait between them, then ready and the patch; got %q", calls)
	}
}

func TestPrBody_goesOnWhenMergeabilityStaysUnknown(t *testing.T) {
	s := newPrBodySandbox(t)
	base := s.head(t)
	commitFile(t, s.repo, "a.txt", "feat(bus): add the thing")
	s.mergeableReads = []string{"null"}
	out, err := s.run(t, true, base, "Add the thing", goodPrBody())
	if err != nil || !strings.Contains(out, "mergeability is still unknown") {
		t.Fatalf("want the script to go on with a notice, got err=%v out=%q", err, out)
	}
	calls := s.ghCalls(t)
	if slept := sleptSeconds(t, calls); slept == 0 || slept > 30 ||
		!strings.Contains(calls, "pr ready 7\n") || !strings.Contains(calls, "api -X PATCH repos/o/r/pulls/7 ") {
		t.Fatalf("want a wait of at most 30s, then ready and the patch; slept %ds, got %q", slept, calls)
	}
}
