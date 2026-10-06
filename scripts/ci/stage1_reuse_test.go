package ci_test

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

const fakeGH = `#!/usr/bin/env bash
set -euo pipefail
endpoint="$2"
filter=""
while [[ $# -gt 0 ]]; do
  if [[ "$1" == --jq ]]; then filter="$2"; fi
  shift
done
if [[ -f "$FAKE_GH/fail" && "$endpoint" == *"$(cat "$FAKE_GH/fail")"* ]]; then
  echo "gh: API rate limit exceeded for installation ID 1 (HTTP 403)" >&2
  exit 1
fi
case "$endpoint" in
  */actions/runs\?*) file="$FAKE_GH/runs.json" ;;
  */commits/*/check-runs\?*) sha="${endpoint#*/commits/}"; file="$FAKE_GH/${sha%%/*}.json" ;;
  *) echo "fake gh: unexpected $endpoint" >&2; exit 1 ;;
esac
[[ -f "$file" ]] || file="$FAKE_GH/none.json"
jq -r "$filter" "$file"
`

type reuseRepo struct {
	t              *testing.T
	dir, fake, bin string
}

func newReuseRepo(t *testing.T) *reuseRepo {
	t.Helper()
	if _, err := exec.LookPath("jq"); err != nil {
		t.Skip("jq not installed")
	}
	r := &reuseRepo{t: t, dir: t.TempDir(), fake: t.TempDir(), bin: t.TempDir()}
	r.write(filepath.Join(r.bin, "gh"), fakeGH, 0o755)
	r.write(filepath.Join(r.fake, "none.json"), `{"check_runs": [], "workflow_runs": []}`, 0o644)
	r.git("init", "-q", "-b", "trunk")
	wf := filepath.Join(r.dir, ".github", "workflows")
	if err := os.MkdirAll(wf, 0o755); err != nil {
		t.Fatal(err)
	}
	r.write(filepath.Join(wf, "ci.yml"), "name: ci\n", 0o644)
	r.commit("base.txt", "base\n")
	return r
}

// writeExecutable holds syscall.ForkLock while the file is open for writing, so no fork can copy the
// descriptor. A child that holds the copy makes exec of the file fail with ETXTBSY until the child
// itself execs (golang/go#22315).
func writeExecutable(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatal(err)
	}
	syscall.ForkLock.Lock()
	err := os.WriteFile(path, []byte(body), 0o700)
	syscall.ForkLock.Unlock()
	if err != nil {
		t.Fatal(err)
	}
}

func (r *reuseRepo) write(name, body string, mode os.FileMode) {
	r.t.Helper()
	if mode&0o111 != 0 {
		writeExecutable(r.t, name, body)
		return
	}
	if err := os.WriteFile(name, []byte(body), mode); err != nil {
		r.t.Fatal(err)
	}
}

func (r *reuseRepo) git(args ...string) string {
	r.t.Helper()
	cmd := exec.Command("git", append([]string{"-c", "user.name=t", "-c", "user.email=t@example.com", "-c", "commit.gpgsign=false"}, args...)...)
	cmd.Dir = r.dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		r.t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

func (r *reuseRepo) commit(name, body string) string {
	r.t.Helper()
	r.write(filepath.Join(r.dir, name), body, 0o644)
	r.git("add", ".")
	r.git("commit", "-q", "-m", name)
	return r.git("rev-parse", "HEAD")
}

func (r *reuseRepo) greenRun(sha, summary string) {
	r.t.Helper()
	r.runs(sha)
	check := map[string]any{"conclusion": "success", "output": map[string]any{"summary": summary}}
	if summary == "" {
		check["output"] = map[string]any{"summary": nil}
	}
	data, _ := json.Marshal(map[string]any{"check_runs": []any{check}})
	r.write(filepath.Join(r.fake, sha+".json"), string(data), 0o644)
}

// runs lists the branch's workflow runs, newest first.
func (r *reuseRepo) runs(shas ...string) {
	r.t.Helper()
	list := []map[string]string{}
	for _, sha := range shas {
		list = append(list, map[string]string{"head_sha": sha})
	}
	data, _ := json.Marshal(map[string]any{"workflow_runs": list})
	r.write(filepath.Join(r.fake, "runs.json"), string(data), 0o644)
}

func (r *reuseRepo) run(base, head string) map[string]string {
	r.t.Helper()
	got, _ := r.runLog(base, head)
	return got
}

func (r *reuseRepo) runLog(base, head string, env ...string) (map[string]string, string) {
	r.t.Helper()
	root := repoRoot(r.t)
	out := filepath.Join(r.t.TempDir(), "out")
	cmd := exec.Command("bash", filepath.Join(root, "scripts", "ci", "stage1-reuse.sh"))
	cmd.Dir = r.dir
	cmd.Env = append(os.Environ(),
		"PATH="+r.bin+string(os.PathListSeparator)+os.Getenv("PATH"),
		"FAKE_GH="+r.fake, "BASE_SHA="+base, "HEAD_SHA="+head, "HEAD_REF=feature",
		"GITHUB_REPOSITORY=o/r", "GITHUB_OUTPUT="+out)
	cmd.Env = append(cmd.Env, env...)
	log, err := cmd.CombinedOutput()
	if err != nil {
		r.t.Fatalf("stage1-reuse.sh: %v\n%s", err, log)
	}
	data, err := os.ReadFile(out)
	if err != nil {
		r.t.Fatal(err)
	}
	got := map[string]string{}
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		k, v, _ := strings.Cut(line, "=")
		got[k] = v
	}
	return got, string(log)
}

func TestStage1Reuse_reusesOnlyAGreenResultForTheSameDiff(t *testing.T) {
	r := newReuseRepo(t)
	base := r.git("rev-parse", "HEAD")
	r.git("switch", "-q", "-c", "feature")
	head := r.commit("change.txt", "one\n")

	first := r.run(base, head)
	if first["reuse"] != "false" || first["patch-id"] == "" || len(first["ci-id"]) != 12 {
		t.Fatalf("first push with no earlier run: %v", first)
	}
	r.greenRun(head, "patch-id: "+first["patch-id"]+" ci-id: "+first["ci-id"])

	r.git("switch", "-q", "trunk")
	newBase := r.commit("other.txt", "moved\n")
	r.git("switch", "-q", "feature")
	r.git("rebase", "-q", "trunk")
	restacked := r.git("rev-parse", "HEAD")
	if got := r.run(newBase, restacked); got["reuse"] != "true" || got["patch-id"] != first["patch-id"] || got["ci-id"] != first["ci-id"] {
		t.Fatalf("restack with the same diff: %v, want reuse of %s ci-id %s", got, first["patch-id"], first["ci-id"])
	}

	changed := r.commit("change.txt", "two\n")
	if got := r.run(newBase, changed); got["reuse"] != "false" {
		t.Fatalf("planted one-line change: %v, want a full stage 1", got)
	}

	r.greenRun(head, "")
	if got := r.run(newBase, restacked); got["reuse"] != "false" {
		t.Fatalf("green ci-ok without a recorded patch ID: %v, want a full stage 1", got)
	}
}

func TestStage1Reuse_neverReusesAnEmptyDiff(t *testing.T) {
	r := newReuseRepo(t)
	base := r.git("rev-parse", "HEAD")
	if got := r.run(base, base); got["reuse"] != "false" || got["patch-id"] != "" || len(got["ci-id"]) != 12 {
		t.Fatalf("empty diff: %v", got)
	}
}

func TestStage1Reuse_samePatchAndCiIDReuses(t *testing.T) {
	r := newReuseRepo(t)
	base := r.git("rev-parse", "HEAD")
	r.git("switch", "-q", "-c", "feature")
	head := r.commit("change.txt", "one\n")
	first := r.run(base, head)
	r.greenRun(head, "patch-id: "+first["patch-id"]+" ci-id: "+first["ci-id"])

	r.git("switch", "-q", "trunk")
	newBase := r.commit("other.txt", "moved\n")
	r.git("switch", "-q", "feature")
	r.git("rebase", "-q", "trunk")
	restacked := r.git("rev-parse", "HEAD")
	got := r.run(newBase, restacked)
	if got["reuse"] != "true" || got["patch-id"] != first["patch-id"] || got["ci-id"] != first["ci-id"] {
		t.Fatalf("same patch and ci-id: %v, want reuse", got)
	}
}

func TestStage1Reuse_samePatchDifferentCiIDDoesNotReuse(t *testing.T) {
	r := newReuseRepo(t)
	base := r.git("rev-parse", "HEAD")
	r.git("switch", "-q", "-c", "feature")
	head := r.commit("change.txt", "one\n")
	first := r.run(base, head)
	r.greenRun(head, "patch-id: "+first["patch-id"]+" ci-id: "+first["ci-id"])

	r.git("switch", "-q", "trunk")
	newBase := r.commit(".github/workflows/ci.yml", "name: ci\non: push\n")
	r.git("switch", "-q", "feature")
	r.git("rebase", "-q", "trunk")
	restacked := r.git("rev-parse", "HEAD")
	got := r.run(newBase, restacked)
	if got["patch-id"] != first["patch-id"] || got["ci-id"] == first["ci-id"] || got["ci-id"] == "" {
		t.Fatalf("trunk workflow change: %v, want the same patch and a new ci-id", got)
	}
	if got["reuse"] != "false" {
		t.Fatalf("same patch, different ci-id: %v, want a full stage 1", got)
	}
}

func TestStage1Reuse_oldPatchIDSummaryDoesNotReuse(t *testing.T) {
	r := newReuseRepo(t)
	base := r.git("rev-parse", "HEAD")
	r.git("switch", "-q", "-c", "feature")
	head := r.commit("change.txt", "one\n")
	first := r.run(base, head)
	r.greenRun(head, "patch-id: "+first["patch-id"])

	r.git("switch", "-q", "trunk")
	newBase := r.commit("other.txt", "moved\n")
	r.git("switch", "-q", "feature")
	r.git("rebase", "-q", "trunk")
	restacked := r.git("rev-parse", "HEAD")
	got := r.run(newBase, restacked)
	if got["patch-id"] != first["patch-id"] || got["ci-id"] != first["ci-id"] {
		t.Fatalf("restack key: %v, want patch %s ci-id %s", got, first["patch-id"], first["ci-id"])
	}
	if got["reuse"] != "false" {
		t.Fatalf("old summary form: %v, want a full stage 1", got)
	}
}

// greenRestack records a green ci-ok for the first push and returns a restack with the same diff, which reuses it.
func greenRestack(t *testing.T) (r *reuseRepo, base, head, restacked string) {
	t.Helper()
	r = newReuseRepo(t)
	first := r.git("rev-parse", "HEAD")
	r.git("switch", "-q", "-c", "feature")
	head = r.commit("change.txt", "one\n")
	got := r.run(first, head)
	r.greenRun(head, "patch-id: "+got["patch-id"]+" ci-id: "+got["ci-id"])
	r.git("switch", "-q", "trunk")
	base = r.commit("other.txt", "moved\n")
	r.git("switch", "-q", "feature")
	r.git("rebase", "-q", "trunk")
	return r, base, head, r.git("rev-parse", "HEAD")
}

func TestStage1Reuse_apiFailureRunsTheFullStageWithAWarning(t *testing.T) {
	for _, endpoint := range []string{"/actions/runs?", "/check-runs?"} {
		r, base, _, restacked := greenRestack(t)
		if got := r.run(base, restacked); got["reuse"] != "true" {
			t.Fatalf("control before the planted failure: %v, want reuse", got)
		}
		r.write(filepath.Join(r.fake, "fail"), endpoint, 0o644)
		got, log := r.runLog(base, restacked)
		if got["reuse"] != "false" || got["patch-id"] == "" || len(got["ci-id"]) != 12 {
			t.Fatalf("gh failing on %s: %v, want reuse=false with both IDs", endpoint, got)
		}
		if !strings.Contains(log, "::warning::stage 1 reuse lookup failed") {
			t.Fatalf("gh failing on %s: no warning in\n%s", endpoint, log)
		}
	}
}

func TestStage1Reuse_looksAtTheFiveNewestDistinctSHAsOnly(t *testing.T) {
	r, base, head, restacked := greenRestack(t)
	newer := []string{"a1", "a2", "a3", "a4"}
	r.runs(append(append([]string{}, newer...), newer[0], head)...)
	if got := r.run(base, restacked); got["reuse"] != "true" {
		t.Fatalf("green run at the fifth distinct SHA: %v, want reuse", got)
	}
	r.runs(append(append([]string{}, newer...), "a5", head)...)
	if got := r.run(base, restacked); got["reuse"] != "false" {
		t.Fatalf("green run at the sixth distinct SHA: %v, want a full stage 1", got)
	}
}

// runStack runs the step as the plan job does for a stacked top PR: its base is the PR below, and TRUNK_REF names
// the trunk, so the key covers the whole stack.
func (r *reuseRepo) runStack(lower, head string) map[string]string {
	r.t.Helper()
	got, _ := r.runLog(lower, head, "TRUNK_REF=trunk")
	return got
}

// stack builds a two-PR stack on trunk, lower then feature, records a green ci-ok for the top PR with its whole-stack
// key, and returns that key.
func stack(t *testing.T) (r *reuseRepo, first map[string]string) {
	t.Helper()
	r = newReuseRepo(t)
	r.commit("shared.txt", "base\n")
	r.git("switch", "-q", "-c", "lower")
	lower := r.commit("shared.txt", "lower\n")
	r.git("switch", "-q", "-c", "feature")
	head := r.commit("top.txt", "top\n")
	first = r.runStack(lower, head)
	if first["reuse"] != "false" || first["patch-id"] == "" || len(first["ci-id"]) != 12 {
		t.Fatalf("first push of the stack: %v", first)
	}
	if own := r.run(lower, head); own["patch-id"] == first["patch-id"] {
		t.Fatalf("whole-stack patch ID %s equals the top PR's own, so it ignores the lower PR", first["patch-id"])
	}
	r.greenRun(head, "patch-id: "+first["patch-id"]+" ci-id: "+first["ci-id"])
	return r, first
}

// restack moves lower onto trunk, resolving a conflict on shared.txt with resolved, then moves feature onto lower.
func (r *reuseRepo) restack(resolved string) (lower, head string) {
	r.t.Helper()
	oldLower := r.git("rev-parse", "lower")
	r.git("switch", "-q", "lower")
	if resolved == "" {
		r.git("rebase", "-q", "trunk")
	} else {
		cmd := exec.Command("git", "-c", "user.name=t", "-c", "user.email=t@example.com", "rebase", "-q", "trunk")
		cmd.Dir = r.dir
		if out, err := cmd.CombinedOutput(); err == nil {
			r.t.Fatalf("rebase of lower onto trunk did not conflict:\n%s", out)
		}
		r.write(filepath.Join(r.dir, "shared.txt"), resolved, 0o644)
		r.git("add", "shared.txt")
		r.git("-c", "core.editor=true", "rebase", "--continue")
	}
	lower = r.git("rev-parse", "HEAD")
	r.git("rebase", "-q", "--onto", "lower", oldLower, "feature")
	return lower, r.git("rev-parse", "HEAD")
}

func TestStage1Reuse_cleanRestackOfAStackReuses(t *testing.T) {
	r, first := stack(t)
	r.git("switch", "-q", "trunk")
	r.commit("other.txt", "moved\n")
	lower, head := r.restack("")
	got := r.runStack(lower, head)
	if got["reuse"] != "true" || got["patch-id"] != first["patch-id"] || got["ci-id"] != first["ci-id"] {
		t.Fatalf("clean restack onto an unrelated trunk change: %v, want reuse of %v", got, first)
	}
}

func TestStage1Reuse_restackThatResolvesAConflictDoesNotReuse(t *testing.T) {
	r, first := stack(t)
	r.git("switch", "-q", "trunk")
	r.commit("shared.txt", "trunk\n")
	lower, head := r.restack("trunk\nlower\n")
	got := r.runStack(lower, head)
	if got["patch-id"] == first["patch-id"] || got["patch-id"] == "" {
		t.Fatalf("conflict resolved in the lower PR: %v, want a new whole-stack patch ID", got)
	}
	if got["reuse"] != "false" {
		t.Fatalf("conflict resolved in the lower PR: %v, want a full stage 1", got)
	}
}

func TestStage1Reuse_stackRestackOntoAWorkflowChangeDoesNotReuse(t *testing.T) {
	r, first := stack(t)
	r.git("switch", "-q", "trunk")
	r.commit(".github/workflows/ci.yml", "name: ci\non: push\n")
	lower, head := r.restack("")
	got := r.runStack(lower, head)
	if got["patch-id"] != first["patch-id"] || got["ci-id"] == first["ci-id"] || got["ci-id"] == "" {
		t.Fatalf("trunk workflow change under the stack: %v, want the same patch and a new ci-id", got)
	}
	if got["reuse"] != "false" {
		t.Fatalf("same stack patch, different ci-id: %v, want a full stage 1", got)
	}
}

func TestStage1Reuse_stackWithNoMergeBaseDoesNotReuse(t *testing.T) {
	r := newReuseRepo(t)
	r.git("switch", "-q", "--orphan", "feature")
	if err := os.MkdirAll(filepath.Join(r.dir, ".github", "workflows"), 0o755); err != nil {
		t.Fatal(err)
	}
	r.commit(".github/workflows/ci.yml", "name: ci\n")
	head := r.commit("top.txt", "top\n")
	r.greenRun(head, "patch-id:  ci-id: ")
	if got := r.runStack(head, head); got["reuse"] != "false" || got["patch-id"] != "" || len(got["ci-id"]) != 12 {
		t.Fatalf("stack with no merge base with trunk: %v, want no key and a full stage 1", got)
	}
}
