package ci_test

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestFetchBase_liveBranchPrintsBaseSHA(t *testing.T) {
	r := newFetchRepo(t)
	r.git("switch", "-q", "-c", "live")
	sha := r.commit("live.txt", "live\n")
	r.git("push", "-q", "origin", "live")

	out, errOut, err := r.run("BASE_SHA="+sha, "BASE_REF=live", "FEATURE_BRANCH=main")
	if err != nil {
		t.Fatalf("fetch-base: %v\n%s", err, errOut)
	}
	if out != sha {
		t.Fatalf("printed %s, want %s\n%s", out, sha, errOut)
	}
}

func TestFetchBase_deletedQueueBranchWithSHAStillPrintsIt(t *testing.T) {
	r := newFetchRepo(t)
	sha := r.git("rev-parse", "HEAD")
	r.git("push", "-q", "origin", "HEAD:refs/heads/gtmq_batch")
	r.git("push", "-q", "origin", ":refs/heads/gtmq_batch")

	out, errOut, err := r.run("BASE_SHA="+sha, "BASE_REF=gtmq_batch", "FEATURE_BRANCH=main")
	if err != nil {
		t.Fatalf("fetch-base: %v\n%s", err, errOut)
	}
	if out != sha {
		t.Fatalf("printed %s, want %s\n%s", out, sha, errOut)
	}
}

func TestFetchBase_unknownSHADeletedBranchFallsBackToTrunk(t *testing.T) {
	r := newFetchRepo(t)
	trunk := r.git("rev-parse", "HEAD")
	r.commit("local.txt", "local\n")
	const unknown = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	const gone = "gone-branch"

	out, errOut, err := r.run("BASE_SHA="+unknown, "BASE_REF="+gone, "FEATURE_BRANCH=main")
	if err != nil {
		t.Fatalf("fetch-base: %v\n%s", err, errOut)
	}
	if out != trunk {
		t.Fatalf("printed %s, want merge base %s\n%s", out, trunk, errOut)
	}
	want := "base " + gone + " not fetchable; falling back to main"
	if !strings.Contains(errOut, want) {
		t.Fatalf("stderr missing %q:\n%s", want, errOut)
	}
}

func TestFetchBase_nothingFetchableNamesAllThree(t *testing.T) {
	r := newFetchRepo(t)
	const sha = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	const ref = "missing-ref"
	const trunk = "missing-trunk"

	_, errOut, err := r.run("BASE_SHA="+sha, "BASE_REF="+ref, "FEATURE_BRANCH="+trunk)
	if err == nil {
		t.Fatal("expected exit 1")
	}
	for _, name := range []string{sha, ref, trunk} {
		if !strings.Contains(errOut, name) {
			t.Errorf("stderr missing %s:\n%s", name, errOut)
		}
	}
}

func TestFetchBase_queueRefWithoutSHAUsesTrunk(t *testing.T) {
	r := newFetchRepo(t)
	trunk := r.git("rev-parse", "HEAD")
	r.git("switch", "-q", "-c", "gtmq_upper")
	tip := r.commit("upper.txt", "upper\n")
	r.git("push", "-q", "origin", "gtmq_upper")
	r.git("switch", "-q", "main")

	out, errOut, err := r.run("BASE_REF=gtmq_upper", "FEATURE_BRANCH=main")
	if err != nil {
		t.Fatalf("fetch-base: %v\n%s", err, errOut)
	}
	if out != trunk {
		t.Fatalf("printed %s, want trunk %s (queue tip %s)\n%s", out, trunk, tip, errOut)
	}
	want := "base gtmq_upper not fetchable; falling back to main"
	if !strings.Contains(errOut, want) {
		t.Fatalf("stderr missing %q:\n%s", want, errOut)
	}
}

type fetchRepo struct {
	t      *testing.T
	dir    string
	origin string
}

func newFetchRepo(t *testing.T) *fetchRepo {
	t.Helper()
	dir := t.TempDir()
	origin := filepath.Join(dir, "origin.git")
	work := filepath.Join(dir, "work")
	gitAt(t, dir, "init", "--bare", "-q", "-b", "main", origin)
	gitAt(t, dir, "-C", origin, "config", "uploadpack.allowReachableSHA1InWant", "true")
	gitAt(t, dir, "-C", origin, "config", "uploadpack.allowAnySHA1InWant", "true")
	gitAt(t, dir, "init", "-q", "-b", "main", work)
	r := &fetchRepo{t: t, dir: work, origin: origin}
	r.git("config", "user.email", "t@example.com")
	r.git("config", "user.name", "t")
	r.git("config", "commit.gpgsign", "false")
	r.commit("README", "trunk\n")
	r.git("remote", "add", "origin", origin)
	r.git("push", "-q", "origin", "main")
	return r
}

func (r *fetchRepo) git(args ...string) string {
	r.t.Helper()
	return gitAt(r.t, r.dir, args...)
}

func (r *fetchRepo) commit(name, body string) string {
	r.t.Helper()
	path := filepath.Join(r.dir, name)
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		r.t.Fatal(err)
	}
	r.git("add", name)
	r.git("commit", "-q", "-m", name)
	return r.git("rev-parse", "HEAD")
}

func (r *fetchRepo) run(env ...string) (string, string, error) {
	r.t.Helper()
	script := filepath.Join(repoRoot(r.t), "scripts", "ci", "fetch-base.sh")
	cmd := exec.Command("bash", script)
	cmd.Dir = r.dir
	cmd.Env = fetchBaseEnv(env...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	return strings.TrimSpace(stdout.String()), stderr.String(), err
}

func fetchBaseEnv(extra ...string) []string {
	out := make([]string, 0, len(os.Environ())+len(extra))
	for _, kv := range os.Environ() {
		if strings.HasPrefix(kv, "GIT_DIR=") || strings.HasPrefix(kv, "GIT_WORK_TREE=") || strings.HasPrefix(kv, "GIT_INDEX_FILE=") {
			continue
		}
		out = append(out, kv)
	}
	return append(out, extra...)
}

func gitAt(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-c", "user.name=t", "-c", "user.email=t@example.com", "-c", "commit.gpgsign=false"}, args...)...)
	cmd.Dir = dir
	cmd.Env = fetchBaseEnv()
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}
