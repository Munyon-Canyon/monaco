package ci_test

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

const fakeWarmGH = `#!/usr/bin/env bash
set -euo pipefail
if [[ "$1" == "run" ]]; then
  shift
  [[ "${1:-}" == "download" ]] || { echo "unexpected run $*" >&2; exit 1; }
  printf '%s\n' "$*" >> "$FAKE_GH/downloads"
  exit 0
fi
[[ "$1" == "api" ]] || { echo "unexpected $1" >&2; exit 1; }
shift
filter=""
url=""
while [[ $# -gt 0 ]]; do
  case "$1" in
    --paginate) shift ;;
    --jq) filter="$2"; shift 2 ;;
    *) url="$1"; shift ;;
  esac
done
if [[ "$url" == *"/actions/artifacts"* ]]; then
  file="$FAKE_GH/artifacts.json"
elif [[ "$url" == *"/actions/runs/"* ]]; then
  id="${url##*/actions/runs/}"
  id="${id%%\?*}"
  file="$FAKE_GH/run-$id.json"
else
  echo "unexpected url $url" >&2
  exit 1
fi
jq -r "$filter" "$file"
`

func TestFetchWarmCache_picksNewestPushOnTheFeatureBranch(t *testing.T) {
	repo := newWarmCache(t, "spm-ios-test")
	const feature = "backend-rewrite-checkpoint-4"
	repo.artifact("spm-ios-test", "2026-09-30T08:00:00Z", 1, true)
	repo.run(1, ".github/workflows/ci-warm.yml", "push", "success", feature)
	repo.artifact("spm-ios-test", "2026-09-30T07:00:00Z", 2, false)
	repo.run(2, ".github/workflows/ci-warm.yml", "pull_request", "success", feature)
	repo.artifact("spm-ios-test", "2026-09-30T06:00:00Z", 3, false)
	repo.run(3, ".github/workflows/ci-warm.yml", "merge_group", "success", feature)
	repo.artifact("spm-ios-test", "2026-09-30T05:00:00Z", 4, false)
	repo.run(4, ".github/workflows/ci-warm.yml", "push", "success", "other-branch")
	repo.artifact("spm-ios-test", "2026-09-30T04:00:00Z", 5, false)
	repo.run(5, ".github/workflows/ci-warm.yml", "push", "failure", feature)
	repo.artifact("spm-ios-test", "2026-09-30T03:30:00Z", 6, false)
	repo.run(6, ".github/workflows/ci-ios.yml", "push", "success", feature)
	repo.artifact("spm-ios-other", "2026-09-30T03:20:00Z", 9, false)
	repo.run(9, ".github/workflows/ci-warm.yml", "push", "success", feature)
	repo.artifact("spm-ios-test", "2026-09-30T03:00:00Z", 7, false)
	repo.run(7, ".github/workflows/ci-warm.yml", "push", "success", feature)
	repo.artifact("spm-ios-test", "2026-09-30T02:00:00Z", 8, false)
	repo.run(8, ".github/workflows/ci-warm.yml", "push", "success", "main")
	repo.flush(t)

	got, dest := repo.fetch(t, feature)
	if got != "artifact run 7\n" {
		t.Fatalf("got %q, want the newest feature-branch push", got)
	}
	downloads := repo.downloads(t)
	if !strings.Contains(downloads, "download 7 ") || !strings.Contains(downloads, dest) || !strings.Contains(downloads, "spm-ios-test") {
		t.Fatalf("download args: %s", downloads)
	}
}

func TestFetchWarmCache_acceptsANewerPushOnMain(t *testing.T) {
	repo := newWarmCache(t, "xcode-cas-test")
	repo.artifact("xcode-cas-test", "2026-09-30T04:00:00Z", 2, false)
	repo.run(2, ".github/workflows/ci-warm.yml", "pull_request", "success", "backend-rewrite-checkpoint-4")
	repo.artifact("xcode-cas-test", "2026-09-30T03:00:00Z", 3, false)
	repo.run(3, ".github/workflows/ci-warm.yml", "push", "success", "main")
	repo.artifact("xcode-cas-test", "2026-09-30T01:00:00Z", 4, false)
	repo.run(4, ".github/workflows/ci-warm.yml", "push", "success", "backend-rewrite-checkpoint-4")
	repo.flush(t)

	got, _ := repo.fetch(t, "backend-rewrite-checkpoint-4")
	if got != "artifact run 3\n" {
		t.Fatalf("got %q, want the main push", got)
	}
}

func TestFetchWarmCache_printsNothingWhenNoneMatch(t *testing.T) {
	repo := newWarmCache(t, "spm-ios-test")
	repo.artifact("spm-ios-test", "2026-09-30T06:00:00Z", 2, false)
	repo.run(2, ".github/workflows/ci-warm.yml", "pull_request", "success", "backend-rewrite-checkpoint-4")
	repo.artifact("spm-ios-test", "2026-09-30T05:00:00Z", 3, false)
	repo.run(3, ".github/workflows/ci-warm.yml", "merge_group", "success", "main")
	repo.artifact("spm-ios-test", "2026-09-30T04:00:00Z", 4, false)
	repo.run(4, ".github/workflows/ci-warm.yml", "push", "success", "other-branch")
	repo.flush(t)

	got, _ := repo.fetch(t, "backend-rewrite-checkpoint-4")
	if got != "nothing\n" {
		t.Fatalf("got %q, want nothing", got)
	}
	if _, err := os.Stat(filepath.Join(repo.fake, "downloads")); !os.IsNotExist(err) {
		t.Fatalf("download ran: %v", err)
	}
}

type warmArtifact struct {
	Name        string `json:"name"`
	Expired     bool   `json:"expired"`
	CreatedAt   string `json:"created_at"`
	WorkflowRun struct {
		ID int `json:"id"`
	} `json:"workflow_run"`
}

type warmCache struct {
	t         *testing.T
	name      string
	fake, bin string
	artifacts []warmArtifact
}

func newWarmCache(t *testing.T, name string) *warmCache {
	t.Helper()
	if _, err := exec.LookPath("jq"); err != nil {
		t.Fatal(err)
	}
	r := &warmCache{t: t, name: name, fake: t.TempDir(), bin: t.TempDir()}
	if err := os.WriteFile(filepath.Join(r.bin, "gh"), []byte(fakeWarmGH), 0o755); err != nil {
		t.Fatal(err)
	}
	return r
}

func (r *warmCache) artifact(name, created string, run int, expired bool) {
	r.t.Helper()
	var item warmArtifact
	item.Name = name
	item.Expired = expired
	item.CreatedAt = created
	item.WorkflowRun.ID = run
	r.artifacts = append(r.artifacts, item)
}

func (r *warmCache) run(id int, path, event, conclusion, branch string) {
	r.t.Helper()
	body, err := json.Marshal(map[string]string{
		"path": path, "event": event, "conclusion": conclusion, "head_branch": branch,
	})
	if err != nil {
		r.t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(r.fake, "run-"+strconv.Itoa(id)+".json"), body, 0o644); err != nil {
		r.t.Fatal(err)
	}
}

func (r *warmCache) flush(t *testing.T) {
	t.Helper()
	body, err := json.Marshal(map[string]any{"artifacts": r.artifacts})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(r.fake, "artifacts.json"), body, 0o644); err != nil {
		t.Fatal(err)
	}
}

func (r *warmCache) fetch(t *testing.T, feature string) (string, string) {
	t.Helper()
	dest := filepath.Join(t.TempDir(), "dest")
	cmd := exec.Command("bash", filepath.Join(repoRoot(t), "scripts", "ci", "fetch-warm-cache.sh"), r.name, dest)
	cmd.Env = append(os.Environ(),
		"PATH="+r.bin+string(os.PathListSeparator)+os.Getenv("PATH"),
		"FAKE_GH="+r.fake,
		"GITHUB_REPOSITORY=o/r",
		"FEATURE_BRANCH="+feature,
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("fetch-warm-cache.sh: %v\n%s", err, out)
	}
	return string(out), dest
}

func (r *warmCache) downloads(t *testing.T) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(r.fake, "downloads"))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}
