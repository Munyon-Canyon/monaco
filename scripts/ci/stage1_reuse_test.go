package ci_test

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
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
	r.commit("base.txt", "base\n")
	return r
}

func (r *reuseRepo) write(name, body string, mode os.FileMode) {
	r.t.Helper()
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
	runs := map[string][]map[string]string{"workflow_runs": {{"head_sha": sha}}}
	data, _ := json.Marshal(runs)
	r.write(filepath.Join(r.fake, "runs.json"), string(data), 0o644)
	check := map[string]any{"conclusion": "success", "output": map[string]any{"summary": summary}}
	if summary == "" {
		check["output"] = map[string]any{"summary": nil}
	}
	data, _ = json.Marshal(map[string]any{"check_runs": []any{check}})
	r.write(filepath.Join(r.fake, sha+".json"), string(data), 0o644)
}

func (r *reuseRepo) run(base, head string) map[string]string {
	r.t.Helper()
	root := repoRoot(r.t)
	out := filepath.Join(r.t.TempDir(), "out")
	cmd := exec.Command("bash", filepath.Join(root, "scripts", "ci", "stage1-reuse.sh"))
	cmd.Dir = r.dir
	cmd.Env = append(os.Environ(),
		"PATH="+r.bin+string(os.PathListSeparator)+os.Getenv("PATH"),
		"FAKE_GH="+r.fake, "BASE_SHA="+base, "HEAD_SHA="+head, "HEAD_REF=feature",
		"GITHUB_REPOSITORY=o/r", "GITHUB_OUTPUT="+out)
	if log, err := cmd.CombinedOutput(); err != nil {
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
	return got
}

func TestStage1Reuse_reusesOnlyAGreenResultForTheSameDiff(t *testing.T) {
	r := newReuseRepo(t)
	base := r.git("rev-parse", "HEAD")
	r.git("switch", "-q", "-c", "feature")
	head := r.commit("change.txt", "one\n")

	first := r.run(base, head)
	if first["reuse"] != "false" || first["patch-id"] == "" {
		t.Fatalf("first push with no earlier run: %v", first)
	}
	r.greenRun(head, "patch-id: "+first["patch-id"])

	r.git("switch", "-q", "trunk")
	newBase := r.commit("other.txt", "moved\n")
	r.git("switch", "-q", "feature")
	r.git("rebase", "-q", "trunk")
	restacked := r.git("rev-parse", "HEAD")
	if got := r.run(newBase, restacked); got["reuse"] != "true" || got["patch-id"] != first["patch-id"] {
		t.Fatalf("restack with the same diff: %v, want reuse of %s", got, first["patch-id"])
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
	if got := r.run(base, base); got["reuse"] != "false" || got["patch-id"] != "" {
		t.Fatalf("empty diff: %v", got)
	}
}
