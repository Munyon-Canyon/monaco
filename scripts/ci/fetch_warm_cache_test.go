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
  dir=""
  while [[ $# -gt 0 ]]; do
    case "$1" in
      --dir) dir="$2"; shift 2 ;;
      *) shift ;;
    esac
  done
  if [[ -n "${FAKE_GH_FAIL_DOWNLOAD:-}" ]]; then
    echo "download failed" >&2
    exit 1
  fi
  if [[ -n "${FAKE_TAR_FILE:-}" ]]; then
    cp "$FAKE_TAR_FILE" "$dir/$(basename "$FAKE_TAR_FILE")"
    exit 0
  fi
  root="${FAKE_TAR_ROOT:-dest}"
  stage="$(mktemp -d)"
  mkdir -p "$stage/$root"
  echo ok > "$stage/$root/marker"
  tar -C "$stage" -cf "$dir/cache.tar" "$root"
  rm -rf "$stage"
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
  if [[ -n "${FAKE_GH_FAIL_RUNS:-}" ]]; then
    echo "HTTP 502: Bad Gateway" >&2
    exit 1
  fi
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
	if !strings.Contains(downloads, "download 7 ") || !strings.Contains(downloads, "spm-ios-test") {
		t.Fatalf("download args: %s", downloads)
	}
	marker, err := os.ReadFile(filepath.Join(dest, "marker"))
	if err != nil || string(marker) != "ok\n" {
		t.Fatalf("extracted marker: %q %v", marker, err)
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

func TestFetchWarmCache_printsNothingWhenTheRunLookupFails(t *testing.T) {
	repo := newWarmCache(t, "spm-ios-test")
	repo.artifact("spm-ios-test", "2026-09-30T03:00:00Z", 7, false)
	repo.run(7, ".github/workflows/ci-warm.yml", "push", "success", "main")
	repo.flush(t)
	repo.extra = append(repo.extra, "FAKE_GH_FAIL_RUNS=1")

	got, dest := repo.fetch(t, "backend-rewrite-checkpoint-4")
	if got != "nothing\n" {
		t.Fatalf("got %q, want nothing", got)
	}
	if _, err := os.Stat(dest); !os.IsNotExist(err) {
		t.Fatalf("dest created: %v", err)
	}
}

func TestFetchWarmCache_leavesDestUntouchedWhenDownloadFails(t *testing.T) {
	repo := newWarmCache(t, "spm-ios-test")
	repo.artifact("spm-ios-test", "2026-09-30T03:00:00Z", 7, false)
	repo.run(7, ".github/workflows/ci-warm.yml", "push", "success", "main")
	repo.flush(t)
	repo.extra = append(repo.extra, "FAKE_GH_FAIL_DOWNLOAD=1")
	repo.prepareDest = func(dest string) {
		if err := os.MkdirAll(dest, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dest, "keep"), []byte("stay"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	got, dest := repo.fetch(t, "backend-rewrite-checkpoint-4")
	if got != "nothing\n" {
		t.Fatalf("got %q, want nothing", got)
	}
	body, err := os.ReadFile(filepath.Join(dest, "keep"))
	if err != nil || string(body) != "stay" {
		t.Fatalf("dest changed: %q %v", body, err)
	}
}

func TestFetchWarmCache_rejectsARunFromAnotherRepository(t *testing.T) {
	repo := newWarmCache(t, "spm-ios-test")
	repo.artifact("spm-ios-test", "2026-09-30T03:00:00Z", 7, false)
	repo.runFrom(7, ".github/workflows/ci-warm.yml", "push", "success", "main", "other/repo")
	repo.flush(t)

	got, _ := repo.fetch(t, "backend-rewrite-checkpoint-4")
	if got != "nothing\n" {
		t.Fatalf("got %q, want nothing", got)
	}
	if _, err := os.Stat(filepath.Join(repo.fake, "downloads")); !os.IsNotExist(err) {
		t.Fatalf("download ran: %v", err)
	}
}

func TestFetchWarmCache_preservesModeAndSymlink(t *testing.T) {
	repo := newWarmCache(t, "spm-ios-test")
	repo.artifact("spm-ios-test", "2026-09-30T03:00:00Z", 7, false)
	repo.run(7, ".github/workflows/ci-warm.yml", "push", "success", "main")
	repo.flush(t)

	stage := t.TempDir()
	root := filepath.Join(stage, "dest")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "tool"), []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("tool", filepath.Join(root, "link")); err != nil {
		t.Fatal(err)
	}
	tarPath := filepath.Join(t.TempDir(), "cache.tar")
	cmd := exec.Command("tar", "-C", stage, "-cf", tarPath, "dest")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("tar: %v\n%s", err, out)
	}
	repo.extra = append(repo.extra, "FAKE_TAR_FILE="+tarPath)

	got, dest := repo.fetch(t, "backend-rewrite-checkpoint-4")
	if got != "artifact run 7\n" {
		t.Fatalf("got %q, want the artifact", got)
	}
	info, err := os.Lstat(filepath.Join(dest, "tool"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o755 {
		t.Fatalf("tool mode %o, want 755", info.Mode().Perm())
	}
	target, err := os.Readlink(filepath.Join(dest, "link"))
	if err != nil {
		t.Fatal(err)
	}
	if target != "tool" {
		t.Fatalf("link target %q, want tool", target)
	}
}

func TestFetchWarmCache_extractsZstd(t *testing.T) {
	if _, err := exec.LookPath("zstd"); err != nil {
		t.Fatal(err)
	}
	repo := newWarmCache(t, "spm-ios-test")
	repo.artifact("spm-ios-test", "2026-09-30T03:00:00Z", 7, false)
	repo.run(7, ".github/workflows/ci-warm.yml", "push", "success", "main")
	repo.flush(t)

	stage := t.TempDir()
	root := filepath.Join(stage, "dest")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "marker"), []byte("zst\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	tarPath := filepath.Join(t.TempDir(), "cache.tar.zst")
	cmd := exec.Command("tar", "-C", stage, "--use-compress-program=zstd -T0 -3", "-cf", tarPath, "dest")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("tar: %v\n%s", err, out)
	}
	repo.extra = append(repo.extra, "FAKE_TAR_FILE="+tarPath)

	got, dest := repo.fetch(t, "backend-rewrite-checkpoint-4")
	if got != "artifact run 7\n" {
		t.Fatalf("got %q, want the artifact", got)
	}
	marker, err := os.ReadFile(filepath.Join(dest, "marker"))
	if err != nil || string(marker) != "zst\n" {
		t.Fatalf("extracted marker: %q %v", marker, err)
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
	t           *testing.T
	name        string
	fake, bin   string
	artifacts   []warmArtifact
	extra       []string
	prepareDest func(dest string)
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
	r.runFrom(id, path, event, conclusion, branch, "o/r")
}

func (r *warmCache) runFrom(id int, path, event, conclusion, branch, fullName string) {
	r.t.Helper()
	body, err := json.Marshal(map[string]any{
		"path": path, "event": event, "conclusion": conclusion, "head_branch": branch,
		"head_repository": map[string]string{"full_name": fullName},
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
	if r.prepareDest != nil {
		r.prepareDest(dest)
	}
	cmd := exec.Command("bash", filepath.Join(repoRoot(t), "scripts", "ci", "fetch-warm-cache.sh"), r.name, dest)
	cmd.Env = append(os.Environ(),
		"PATH="+r.bin+string(os.PathListSeparator)+os.Getenv("PATH"),
		"FAKE_GH="+r.fake,
		"GITHUB_REPOSITORY=o/r",
		"FEATURE_BRANCH="+feature,
		"FAKE_TAR_ROOT=dest",
	)
	cmd.Env = append(cmd.Env, r.extra...)
	var stderr strings.Builder
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("fetch-warm-cache.sh: %v\n%s%s", err, out, stderr.String())
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
