package ci_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const fakePruneGH = `#!/usr/bin/env bash
set -euo pipefail
[[ "$1" == "cache" ]] || { echo "unexpected $1" >&2; exit 1; }
shift
sub="$1"
shift
printf '%s\n' "$sub $*" >> "$FAKE_GH/calls"
case "$sub" in
  list)
    ref=""
    key=""
    while [[ $# -gt 0 ]]; do
      case "$1" in
        --ref) ref="$2"; shift 2 ;;
        --key) key="$2"; shift 2 ;;
        --json|--limit|--sort|--order) shift 2 ;;
        *) echo "unexpected list arg $1" >&2; exit 1 ;;
      esac
    done
    if [[ -n "${FAKE_GH_FAIL_LIST:-}" ]]; then
      echo "HTTP 502: Bad Gateway" >&2
      exit 1
    fi
    file="$FAKE_GH/list-${key}.json"
    if [[ ! -f "$file" ]]; then
      echo '[]'
      exit 0
    fi
    cat "$file"
    ;;
  delete)
    printf '%s\n' "$1" >> "$FAKE_GH/deleted"
    if [[ -n "${FAKE_GH_FAIL_DELETE:-}" ]]; then
      echo "HTTP 502: Bad Gateway" >&2
      exit 1
    fi
    ;;
  *)
    echo "unexpected sub $sub" >&2
    exit 1
    ;;
esac
`

func TestPruneCaches_keepsTheNewestPerPrefix(t *testing.T) {
	repo := newPruneRepo(t)
	const ref = "refs/heads/backend-rewrite-checkpoint-4"
	repo.list("xcode-cas-macOS-", `[
	  {"id": 12, "key": "not-a-cache", "createdAt": "2026-09-30T12:00:00Z", "sizeInBytes": 999, "ref": "refs/heads/backend-rewrite-checkpoint-4"},
	  {"id": 13, "key": "xcode-cas-macOS-elsewhere", "createdAt": "2026-09-30T13:00:00Z", "sizeInBytes": 999, "ref": "refs/heads/other"},
	  {"id": 10, "key": "xcode-cas-macOS-new", "createdAt": "2026-09-30T10:00:00Z", "sizeInBytes": 100, "ref": "refs/heads/backend-rewrite-checkpoint-4"},
	  {"id": 11, "key": "xcode-cas-macOS-old", "createdAt": "2026-09-30T09:00:00Z", "sizeInBytes": 1048576, "ref": "refs/heads/backend-rewrite-checkpoint-4"}
	]`)
	repo.list("swiftpm-linux-6.3-", `[
	  {"id": 21, "key": "swiftpm-linux-6.3-old", "createdAt": "2026-09-30T08:00:00Z", "sizeInBytes": 1048576, "ref": "refs/heads/backend-rewrite-checkpoint-4"},
	  {"id": 20, "key": "swiftpm-linux-6.3-new", "createdAt": "2026-09-30T09:00:00Z", "sizeInBytes": 100, "ref": "refs/heads/backend-rewrite-checkpoint-4"}
	]`)
	repo.list("swiftpm-macos-", `[
	  {"id": 30, "key": "swiftpm-macos-only", "createdAt": "2026-09-30T09:00:00Z", "sizeInBytes": 100, "ref": "refs/heads/backend-rewrite-checkpoint-4"}
	]`)

	out := repo.run(t, nil, ref, "xcode-cas-macOS-", "swiftpm-linux-6.3-", "swiftpm-macos-")
	if !strings.Contains(out, "pruned 2 entries, 2 MB\n") {
		t.Fatalf("summary:\n%s", out)
	}
	deleted := repo.deleted(t)
	for _, id := range []string{"11", "21"} {
		if !strings.Contains(deleted, id+"\n") {
			t.Fatalf("deleted ids missing %s:\n%s", id, deleted)
		}
	}
	for _, id := range []string{"10", "12", "13", "20", "30"} {
		if strings.Contains(deleted, id+"\n") {
			t.Fatalf("deleted %s, which must stay:\n%s\nout:\n%s", id, deleted, out)
		}
	}
	calls := repo.calls(t)
	if !strings.Contains(calls, "--ref "+ref) || !strings.Contains(calls, "--key xcode-cas-macOS-") {
		t.Fatalf("list calls:\n%s", calls)
	}
}

func TestPruneCaches_emptyList(t *testing.T) {
	repo := newPruneRepo(t)
	repo.list("xcode-cas-macOS-", "[]")
	out := repo.run(t, nil, "refs/heads/backend-rewrite-checkpoint-4", "xcode-cas-macOS-")
	if out != "pruned 0 entries, 0 MB\n" {
		t.Fatalf("got %q", out)
	}
	if deleted := repo.deleted(t); deleted != "" {
		t.Fatalf("deleted %q", deleted)
	}
}

func TestPruneCaches_listErrorExitsZero(t *testing.T) {
	repo := newPruneRepo(t)
	out := repo.run(t, []string{"FAKE_GH_FAIL_LIST=1"}, "refs/heads/main", "xcode-cas-macOS-")
	if !strings.Contains(out, "pruned 0 entries, 0 MB\n") || !strings.Contains(out, "list failed") {
		t.Fatalf("got %q", out)
	}
	if deleted := repo.deleted(t); deleted != "" {
		t.Fatalf("deleted %q", deleted)
	}
}

func TestPruneCaches_deleteErrorExitsZero(t *testing.T) {
	repo := newPruneRepo(t)
	repo.list("swiftpm-macos-", `[
	  {"id": 2, "key": "swiftpm-macos-new", "createdAt": "2026-09-30T10:00:00Z", "sizeInBytes": 10, "ref": "refs/heads/main"},
	  {"id": 1, "key": "swiftpm-macos-old", "createdAt": "2026-09-30T09:00:00Z", "sizeInBytes": 1048576, "ref": "refs/heads/main"}
	]`)
	out := repo.run(t, []string{"FAKE_GH_FAIL_DELETE=1"}, "refs/heads/main", "swiftpm-macos-")
	if !strings.Contains(out, "pruned 0 entries, 0 MB\n") || !strings.Contains(out, "delete 1 failed") {
		t.Fatalf("got %q", out)
	}
}

func TestPruneCaches_emptyPrefixRefused(t *testing.T) {
	repo := newPruneRepo(t)
	repo.list("", `[
	  {"id": 1, "key": "go-mod-linux-abc", "createdAt": "2026-09-30T09:00:00Z", "sizeInBytes": 1048576, "ref": "refs/heads/main"},
	  {"id": 2, "key": "go-build-linux-abc", "createdAt": "2026-09-30T10:00:00Z", "sizeInBytes": 100, "ref": "refs/heads/main"}
	]`)
	cmd := exec.Command("bash", repo.script, "refs/heads/main", "xcode-cas-macOS-", "")
	cmd.Env = repo.env(nil)
	out, err := cmd.CombinedOutput()
	exit, ok := err.(*exec.ExitError)
	if !ok || exit.ExitCode() != 2 {
		t.Fatalf("exit: %v\n%s", err, out)
	}
	if deleted := repo.deleted(t); deleted != "" {
		t.Fatalf("deleted %q", deleted)
	}
}

func TestPruneCaches_usage(t *testing.T) {
	repo := newPruneRepo(t)
	cmd := exec.Command("bash", repo.script)
	cmd.Env = repo.env(nil)
	err := cmd.Run()
	exit, ok := err.(*exec.ExitError)
	if !ok || exit.ExitCode() != 2 {
		t.Fatalf("usage exit: %v", err)
	}
}

type pruneRepo struct {
	dir    string
	script string
	bin    string
}

func newPruneRepo(t *testing.T) *pruneRepo {
	t.Helper()
	dir := t.TempDir()
	bin := t.TempDir()
	if err := os.WriteFile(filepath.Join(bin, "gh"), []byte(fakePruneGH), 0o755); err != nil {
		t.Fatal(err)
	}
	return &pruneRepo{
		dir:    dir,
		script: filepath.Join("..", "..", "scripts", "ci", "prune-caches.sh"),
		bin:    bin,
	}
}

func (r *pruneRepo) list(key, json string) {
	if err := os.WriteFile(filepath.Join(r.dir, "list-"+key+".json"), []byte(json), 0o644); err != nil {
		panic(err)
	}
}

func (r *pruneRepo) env(extra []string) []string {
	return append(os.Environ(), append([]string{
		"FAKE_GH=" + r.dir,
		"PATH=" + r.bin + string(os.PathListSeparator) + os.Getenv("PATH"),
	}, extra...)...)
}

func (r *pruneRepo) run(t *testing.T, extra []string, args ...string) string {
	t.Helper()
	cmd := exec.Command("bash", append([]string{r.script}, args...)...)
	cmd.Env = r.env(extra)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	return string(out)
}

func (r *pruneRepo) deleted(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(r.dir, "deleted"))
	if os.IsNotExist(err) {
		return ""
	}
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func (r *pruneRepo) calls(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(r.dir, "calls"))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
