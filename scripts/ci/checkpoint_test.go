package ci_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const branch = "backend-rewrite-9"

func mergeBackJob(t *testing.T) []string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(repoRoot(t), ".github", "workflows", "checkpoint.yml"))
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(string(raw), "\n")
	for i, line := range lines {
		if line != "  merge-back:" {
			continue
		}
		end := i + 1
		for end < len(lines) && (lines[end] == "" || strings.HasPrefix(lines[end], "   ")) {
			end++
		}
		return lines[i:end]
	}
	t.Fatal("checkpoint.yml has no merge-back job")
	return nil
}

func mergeBackScript(t *testing.T) string {
	t.Helper()
	job := mergeBackJob(t)
	for i, line := range job {
		if strings.TrimSpace(line) != "run: |" {
			continue
		}
		indent := strings.Repeat(" ", len(line)-len(strings.TrimLeft(line, " "))+2)
		var body []string
		for _, l := range job[i+1:] {
			if l != "" && !strings.HasPrefix(l, indent) {
				break
			}
			body = append(body, strings.TrimPrefix(l, indent))
		}
		script := strings.Join(body, "\n")
		if strings.Contains(script, "git push") {
			return script
		}
	}
	t.Fatal("merge-back has no run step that pushes")
	return ""
}

func TestMergeBack_checksOutTheHeadCommitAndPushesTheBranchByName(t *testing.T) {
	job := strings.Join(mergeBackJob(t), "\n")
	for _, want := range []string{
		"ref: ${{ github.event.pull_request.head.sha }}",
		"BRANCH: ${{ github.event.pull_request.head.ref }}",
		"HEAD_SHA: ${{ github.event.pull_request.head.sha }}",
		`git push origin "HEAD:refs/heads/$BRANCH"`,
	} {
		if !strings.Contains(job, want) {
			t.Errorf("merge-back lacks %q", want)
		}
	}
	if strings.Contains(job, "ref: ${{ github.event.pull_request.head.ref }}") {
		t.Error("merge-back checks out the head ref, which is gone when GitHub deletes the branch at merge")
	}
}

type checkpointRepo struct {
	dir, remote, head, squash string
}

func git(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t", "GIT_CONFIG_GLOBAL=/dev/null")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

func newCheckpointRepo(t *testing.T) checkpointRepo {
	t.Helper()
	root := t.TempDir()
	r := checkpointRepo{dir: filepath.Join(root, "work"), remote: filepath.Join(root, "origin.git")}
	git(t, root, "init", "-q", "--bare", "-b", "main", r.remote)
	git(t, root, "clone", "-q", r.remote, r.dir)
	git(t, r.dir, "commit", "-q", "--allow-empty", "-m", "base")
	git(t, r.dir, "push", "-q", "origin", "HEAD:refs/heads/main")
	git(t, r.dir, "switch", "-q", "-c", branch)
	git(t, r.dir, "commit", "-q", "--allow-empty", "-m", "feature")
	r.head = git(t, r.dir, "rev-parse", "HEAD")
	git(t, r.dir, "push", "-q", "origin", "HEAD:refs/heads/"+branch)
	git(t, r.dir, "switch", "-q", "main")
	git(t, r.dir, "commit", "-q", "--allow-empty", "-m", "checkpoint (squash)")
	r.squash = git(t, r.dir, "rev-parse", "HEAD")
	git(t, r.dir, "push", "-q", "origin", "main")
	git(t, r.dir, "switch", "-q", "--detach", r.head)
	git(t, r.dir, "branch", "-q", "-D", branch)
	return r
}

func (r checkpointRepo) runMergeBack(t *testing.T) (summary string, out string, err error) {
	t.Helper()
	summaryFile := filepath.Join(t.TempDir(), "summary")
	cmd := exec.Command("bash", "--noprofile", "--norc", "-eo", "pipefail", "-c", mergeBackScript(t))
	cmd.Dir = r.dir
	cmd.Env = append(os.Environ(), "BRANCH="+branch, "HEAD_SHA="+r.head, "SQUASH="+r.squash, "GITHUB_STEP_SUMMARY="+summaryFile, "GIT_CONFIG_GLOBAL=/dev/null")
	b, err := cmd.CombinedOutput()
	s, _ := os.ReadFile(summaryFile)
	return string(s), string(b), err
}

func (r checkpointRepo) assertMergedBack(t *testing.T) {
	t.Helper()
	tip := git(t, r.remote, "rev-parse", "refs/heads/"+branch)
	if parents := git(t, r.remote, "rev-list", "--parents", "-n", "1", tip); parents != tip+" "+r.head+" "+r.squash {
		t.Fatalf("%s tip parents %q, want the PR head then the squash", branch, parents)
	}
}

func TestMergeBack_recreatesABranchGitHubDeletedAtMerge(t *testing.T) {
	r := newCheckpointRepo(t)
	git(t, r.remote, "update-ref", "-d", "refs/heads/"+branch)

	summary, out, err := r.runMergeBack(t)
	if err != nil {
		t.Fatalf("merge-back failed: %v\n%s", err, out)
	}
	r.assertMergedBack(t)
	if !strings.HasPrefix(summary, "Recreated `"+branch+"`") {
		t.Fatalf("summary %q, want it to say the branch was recreated", summary)
	}
}

func TestMergeBack_fastForwardsABranchStillAtTheHead(t *testing.T) {
	r := newCheckpointRepo(t)

	summary, out, err := r.runMergeBack(t)
	if err != nil {
		t.Fatalf("merge-back failed: %v\n%s", err, out)
	}
	r.assertMergedBack(t)
	if !strings.HasPrefix(summary, "Updated `"+branch+"`") {
		t.Fatalf("summary %q, want it to say the branch was updated", summary)
	}
}

func TestMergeBack_refusesABranchPushedToAfterTheMerge(t *testing.T) {
	r := newCheckpointRepo(t)
	git(t, r.dir, "commit", "-q", "--allow-empty", "-m", "late push")
	late := git(t, r.dir, "rev-parse", "HEAD")
	git(t, r.dir, "push", "-q", "origin", "HEAD:refs/heads/"+branch)
	git(t, r.dir, "switch", "-q", "--detach", r.head)

	summary, out, err := r.runMergeBack(t)
	if err == nil {
		t.Fatalf("merge-back succeeded over a late push:\n%s", out)
	}
	if !strings.Contains(out, "::error::"+branch+" moved to "+late) {
		t.Fatalf("output lacks the moved-branch error:\n%s", out)
	}
	if tip := git(t, r.remote, "rev-parse", "refs/heads/"+branch); tip != late {
		t.Fatalf("%s tip %s, want the late push %s left alone", branch, tip, late)
	}
	if summary != "" {
		t.Fatalf("summary %q, want none on failure", summary)
	}
}
