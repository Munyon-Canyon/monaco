package scripts_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

type hookRun struct {
	code   int
	stdout string
	stderr string
}

func runHook(t *testing.T, script string, event map[string]any, env ...string) hookRun {
	t.Helper()
	input, err := json.Marshal(event)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(repoRoot(t), "scripts", script)
	if _, err := os.ReadFile(path); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("python3", path)
	cmd.Stdin = bytes.NewReader(input)
	cmd.Env = append(os.Environ(), env...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err = cmd.Run()
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		return hookRun{exit.ExitCode(), stdout.String(), stderr.String()}
	}
	if err != nil {
		t.Fatal(err)
	}
	return hookRun{0, stdout.String(), stderr.String()}
}

func guard(t *testing.T, cwd, command string, env ...string) hookRun {
	t.Helper()
	return runHook(t, "agent-guard.py", map[string]any{
		"hook_event_name": "PreToolUse",
		"tool_name":       "Bash",
		"cwd":             cwd,
		"tool_input":      map[string]any{"command": command},
	}, env...)
}

func assertBlocked(t *testing.T, r hookRun, command, want string) {
	t.Helper()
	if r.code != 2 || !strings.Contains(r.stderr, want) {
		t.Errorf("%q: want exit 2 with %q, got %d stderr=%q", command, want, r.code, r.stderr)
	}
}

func assertAllowed(t *testing.T, r hookRun, command string) {
	t.Helper()
	if r.code != 0 {
		t.Errorf("%q: want allowed, got %d stderr=%q", command, r.code, r.stderr)
	}
}

func pushRepo(t *testing.T) (work, remote string) {
	t.Helper()
	dir := t.TempDir()
	remote = filepath.Join(dir, "remote.git")
	work = filepath.Join(dir, "work")
	git(t, dir, "init", "-q", "--bare", "-b", "main", remote)
	git(t, dir, "init", "-q", "-b", "ticket", work)
	git(t, work, "config", "user.email", "a@example.com")
	git(t, work, "config", "user.name", "a")
	git(t, work, "config", "commit.gpgsign", "false")
	git(t, work, "commit", "-q", "--allow-empty", "-m", "chore: root")
	git(t, work, "remote", "add", "origin", remote)
	git(t, work, "push", "-q", "origin", "ticket", "ticket:main")
	return work, remote
}

func TestAgentGuard_blocksMutationRunsAndAllowsInstallingTheTool(t *testing.T) {
	cwd := t.TempDir()
	for _, cmd := range []string{
		"gremlins unleash ./internal/modules/bus",
		"cd apps/backend && go run ./cmd/monacoctl mutation --pkg bus",
		"bin/monacoctl mutation",
		"just test mutation",
		"/Users/x/.git/pstack/m7-rest/heavy.sh gremlins unleash",
		"FOO=1 nice -n 19 gremlins unleash",
	} {
		assertBlocked(t, guard(t, cwd, cmd), cmd, "mutation testing runs in CI")
	}
	for _, cmd := range []string{
		"scripts/install-gremlins.sh",
		"go install github.com/go-gremlins/gremlins/cmd/gremlins@v0.5.0",
		"grep -rn mutation docs",
		"cd apps/backend && go run ./cmd/monacoctl flows check",
	} {
		assertAllowed(t, guard(t, cwd, cmd), cmd)
	}
}

func TestAgentGuard_blocksPushesToMainStagingAndGraphiteTrunks(t *testing.T) {
	work, _ := pushRepo(t)
	trunks := `{"trunk":"main","trunks":[{"name":"main"},{"name":"milestone-9"}]}`
	if err := os.WriteFile(filepath.Join(work, ".git", ".graphite_repo_config"), []byte(trunks), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, cmd := range []string{
		"git push origin main",
		"git push origin HEAD:main",
		"git push origin ticket:refs/heads/staging",
		"git push origin ticket:staging",
		"git push origin milestone-9",
		"git push -u origin ticket:milestone-9 2>&1 | tail -3",
		"git -C . push --all origin",
		"bash -c 'git push origin main'",
	} {
		r := guard(t, work, cmd)
		if r.code != 2 || !(strings.Contains(r.stderr, "only through the Graphite merge queue") || strings.Contains(r.stderr, "--all")) {
			t.Errorf("%q: want blocked, got %d %q", cmd, r.code, r.stderr)
		}
	}
	git(t, work, "switch", "-q", "-c", "staging")
	assertBlocked(t, guard(t, work, "git push"), "git push on staging", "'staging' is not allowed")
	git(t, work, "switch", "-q", "-c", "backend-rewrite-7")
	assertAllowed(t, guard(t, work, "git push origin backend-rewrite-7"), "an old checkpoint name is a plain branch now")
	git(t, work, "switch", "-q", "ticket")
	assertAllowed(t, guard(t, work, "git push origin ticket"), "git push origin ticket")
	assertAllowed(t, guard(t, t.TempDir(), "cd "+work+" && git push"), "cd work && git push")
}

func TestAgentGuard_forcePushNeedsAnExplicitLease(t *testing.T) {
	work, _ := pushRepo(t)
	for cmd, want := range map[string]string{
		"git push --force origin ticket":                                   "--force/-f is not allowed",
		"git push -fu origin ticket":                                       "--force/-f is not allowed",
		"git push --force-with-lease origin ticket":                        "without <branch>:<sha>",
		"git push --force-with-lease=ticket origin ticket":                 "without <branch>:<sha>",
		"git push origin +ticket":                                          "'+ticket' force-pushes without a lease",
		"git push --force --force-with-lease=ticket:abcdef1 origin ticket": "--force/-f is not allowed",
	} {
		assertBlocked(t, guard(t, work, cmd), cmd, want)
	}
	sha := strings.TrimSpace(gitOut(t, work, "rev-parse", "HEAD"))
	cmd := "git push --force-with-lease=ticket:" + sha + " origin ticket"
	assertAllowed(t, guard(t, work, cmd), cmd)
}

func TestAgentGuard_blocksAPushThatWouldDropRemoteCommits(t *testing.T) {
	work, remote := pushRepo(t)
	other := filepath.Join(t.TempDir(), "other")
	git(t, filepath.Dir(other), "clone", "-q", "-b", "ticket", remote, other)
	commitFile(t, other, "theirs.txt", "fix: pushed by someone else")
	git(t, other, "push", "-q", "origin", "ticket")
	commitFile(t, work, "mine.txt", "feat: mine")

	assertBlocked(t, guard(t, work, "git push origin ticket"), "push while behind", "fix: pushed by someone else")
	lease := "git push --force-with-lease=ticket:" + strings.TrimSpace(gitOut(t, other, "rev-parse", "HEAD")) + " origin ticket"
	assertBlocked(t, guard(t, work, lease), "lease while behind", "has commits that 'ticket' lacks")

	git(t, work, "pull", "-q", "--rebase", "origin", "ticket")
	assertAllowed(t, guard(t, work, "git push origin ticket"), "push after pull --rebase")
	git(t, work, "push", "-q", "origin", "ticket")

	old := strings.TrimSpace(gitOut(t, work, "rev-parse", "HEAD"))
	git(t, work, "commit", "-q", "--amend", "--no-edit", "--date=2020-01-01T00:00:00")
	if strings.TrimSpace(gitOut(t, work, "rev-parse", "HEAD")) == old {
		t.Fatal("the amend did not rewrite the commit")
	}
	rewritten := "git push --force-with-lease=ticket:" + old + " origin ticket"
	assertAllowed(t, guard(t, work, rewritten), "force push of a patch-equivalent rewrite")
	assertAllowed(t, guard(t, work, "git push origin brand-new-branch"), "push of a branch the remote lacks")
}

func TestAgentGuard_headlessClaudeNeedsTimeout(t *testing.T) {
	cwd := t.TempDir()
	for _, cmd := range []string{`claude -p "hi"`, `claude --print hi`, `cd /tmp && claude --model haiku -p hi`} {
		assertBlocked(t, guard(t, cwd, cmd), cmd, "claude -p must run under timeout")
	}
	for _, cmd := range []string{`timeout 180 claude -p hi`, `timeout -k 5 120 claude -p hi`, `claude --version`, `gtimeout 60 claude -p hi`} {
		assertAllowed(t, guard(t, cwd, cmd), cmd)
	}
}

func ghStub(t *testing.T, statuses string) (env []string, calls string) {
	t.Helper()
	return ghStubOnBase(t, "backend-rewrite-9", statuses)
}

func ghStubOnBase(t *testing.T, base, statuses string) (env []string, calls string) {
	t.Helper()
	dir := t.TempDir()
	calls = filepath.Join(dir, "calls")
	view := fmt.Sprintf(`{"baseRefName":%q,"number":42,"headRefOid":"0123456789abcdef0123456789abcdef01234567","url":"https://github.com/o/r/pull/42"}`, base)
	writeExecutable(t, filepath.Join(dir, "bin", "gh"), fmt.Sprintf(`#!/bin/sh
echo "$*" >> %q
case "$1 $2" in
  "pr view") echo '%s' ;;
  "api repos/o/r/commits/0123456789abcdef0123456789abcdef01234567/statuses?per_page=100") echo '%s' ;;
  *) echo "unexpected: $*" >&2; exit 1 ;;
esac
`, calls, view, statuses))
	return []string{"PATH=" + filepath.Join(dir, "bin") + ":" + os.Getenv("PATH")}, calls
}

func status(state, login string, id int) string {
	return fmt.Sprintf(`{"context":"verify","state":%q,"creator":{"login":%q,"id":%d}}`, state, login, id)
}

func TestAgentGuard_ghPrMergeNeedsTheLatestVerifyToBeSuccess(t *testing.T) {
	cwd := t.TempDir()
	blocked := map[string]string{
		"no statuses":          `[]`,
		"latest verify failed": `[` + status("failure", "verifier", 2) + `,` + status("success", "verifier", 2) + `]`,
	}
	for name, statuses := range blocked {
		env, _ := ghStub(t, statuses)
		assertBlocked(t, guard(t, cwd, "gh pr merge 42 --squash", env...), name, "has no verify success")
	}
	env, calls := ghStub(t, `[`+status("success", "lognorman20", 1)+`,`+status("failure", "verifier", 2)+`]`)
	assertAllowed(t, guard(t, cwd, "gh pr merge 42 --squash --auto", env...), "latest verify success, posted by anyone")
	got, _ := os.ReadFile(calls)
	if !strings.Contains(string(got), "pr view 42 --json") {
		t.Errorf("want the PR selector passed to gh pr view, got %q", got)
	}
	env, _ = ghStub(t, `[`+status("success", "lognorman20", 1)+`]`)
	assertAllowed(t, guard(t, cwd, "gh pr view 42", env...), "gh pr view")
}

func TestAgentGuard_onlyTheOperatorMergesIntoMain(t *testing.T) {
	cwd := t.TempDir()
	env, _ := ghStubOnBase(t, "main", `[`+status("success", "lognorman20", 1)+`]`)
	for _, cmd := range []string{"gh pr merge 42 --squash", "gh pr merge 42 --auto --squash"} {
		assertBlocked(t, guard(t, cwd, cmd, env...), cmd, "Only the operator merges into main")
	}
}

func TestAgentGuard_prBodiesComeFromFiles(t *testing.T) {
	cwd := t.TempDir()
	for _, cmd := range []string{
		`gh pr edit 5 --body "x"`,
		`gh pr edit 5 -b x`,
		`gh pr create --base b --title t --body="x"`,
	} {
		assertBlocked(t, guard(t, cwd, cmd), cmd, "scripts/pr-body.sh <pr> <title> <body-file>")
	}
	for _, cmd := range []string{
		`gh pr edit 5 --body-file body.md`,
		`gh pr create --base b --title t --body-file body.md`,
		`scripts/pr-body.sh 5 "Add the thing" body.md`,
	} {
		assertAllowed(t, guard(t, cwd, cmd), cmd)
	}
}

func TestAgentGuard_commitSubjectsAreConventionalWhereverTheHookCanSeeThem(t *testing.T) {
	cwd := t.TempDir()
	if err := os.WriteFile(filepath.Join(cwd, "bad.txt"), []byte("Add a thing\n\nbody\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cwd, "good.txt"), []byte("feat(bus): add a thing\n\nbody\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	heredoc := func(subject string) string {
		return "git commit -m \"$(cat <<'EOF'\n" + subject + "\n\nIt's the body.\nEOF\n)\""
	}
	for _, cmd := range []string{
		`git commit -m "Add a thing"`,
		`git commit -am "Add a thing"`,
		`git commit -a -m "feature: add a thing"`,
		`git commit --message="fix:no space"`,
		`git -C . commit --amend -m "Update"`,
		`git commit -F bad.txt`,
		"git commit -F - <<'EOF'\nAdd a thing\nEOF",
		heredoc("Add a thing"),
		`gt create -am "Add a thing"`,
		`gt modify -m "wip"`,
		`gt c -m "Add a thing"`,
	} {
		assertBlocked(t, guard(t, cwd, cmd), cmd, "/commit skill")
	}
	for _, cmd := range []string{
		`git commit -m "feat(bus): add a thing"`,
		`git commit -am "fix!: drop the legacy route"`,
		`git commit -m "refactor(platform/db): split pool" -m "Body."`,
		`git commit -F good.txt`,
		"git commit -F - <<'EOF'\nci: cache go builds\nEOF",
		heredoc("docs: explain the hook"),
		`git commit --amend --no-edit`,
		`git commit`,
		`git commit -m "$MSG"`,
		`gt create -am "test(testkit): cover the guard"`,
		`gt modify -a`,
		`gt create branch-name`,
		`git log -m --oneline`,
	} {
		assertAllowed(t, guard(t, cwd, cmd), cmd)
	}
}

func TestAgentGuard_ignoresCommandTextInsideHeredocBodiesAndComments(t *testing.T) {
	work, _ := pushRepo(t)
	for _, cmd := range []string{
		"cat > body.md <<'EOF'\ngit push origin main\ngremlins unleash, don't\nEOF\necho written",
		"git status # never git push origin main",
		"echo 'git push origin main'",
		`printf '%s\n' "claude -p hi"`,
	} {
		assertAllowed(t, guard(t, work, cmd), cmd)
	}
	assertBlocked(t, guard(t, work, "cat > b <<EOF\nx\nEOF\ngit push origin main"), "push after a heredoc", "'main' is not allowed")
}

func commitFile(t *testing.T, dir, name, subject string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(subject), 0o644); err != nil {
		t.Fatal(err)
	}
	git(t, dir, "add", name)
	git(t, dir, "-c", "user.email=b@example.com", "-c", "user.name=b", "commit", "-q", "-m", subject)
}

func gitOut(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git %v: %v", args, err)
	}
	return string(out)
}

func TestAgentGuard_blocksRawRebaseAndMergeInAWorktreeAndAtTheRoot(t *testing.T) {
	t.Setenv("PYENV_VERSION", "system")
	worktree := filepath.Join(t.TempDir(), ".worktrees", "lane")
	for _, cmd := range []string{"git rebase origin/main", "git merge origin/main", "cd " + worktree + " && git rebase"} {
		assertBlocked(t, guard(t, worktree, cmd), cmd, "gt restack")
	}
	root := t.TempDir()
	git(t, root, "init", "-q")
	assertBlocked(t, guard(t, root, "git rebase --onto main"), "git rebase --onto main", "gt restack")
	assertAllowed(t, guard(t, worktree, "gt sync --no-interactive --no-restack"), "gt sync --no-restack")
	assertAllowed(t, guard(t, worktree, "gt restack"), "gt restack")
	assertAllowed(t, guard(t, root, "gt restack"), "gt restack")
	elsewhere := t.TempDir()
	assertAllowed(t, guard(t, elsewhere, "git rebase origin/main"), "git rebase origin/main")
}

func TestAgentGuard_gtSyncLeavesOtherStacksUnrestacked(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	py := "PYENV_VERSION=system"
	for _, cmd := range []string{"gt sync", "gt sync --no-interactive", "cd /tmp && gt sync -f", "gt --cwd /w sync"} {
		assertBlocked(t, guard(t, dir, cmd, py), cmd, "gt sync --no-interactive --no-restack")
	}
	for _, cmd := range []string{"gt sync --no-interactive --no-restack", "gt restack --upstack", "gt submit --stack"} {
		assertAllowed(t, guard(t, dir, cmd, py), cmd)
	}
}

func TestAgentGuard_stagingTakesPRsOnlyThroughTheQueue(t *testing.T) {
	cwd := t.TempDir()
	env, _ := ghStubOnBase(t, "staging", `[`+status("success", "verifier", 2)+`]`)
	for _, cmd := range []string{"gh pr merge 42 --squash", "gh pr merge 42 --auto", "gh pr merge --auto 42 --squash"} {
		assertBlocked(t, guard(t, cwd, cmd, env...), cmd, "takes PRs only through the Graphite merge queue")
	}
}

func TestAgentGuard_onlyLandStackAddsTheQueueLabel(t *testing.T) {
	cwd := t.TempDir()
	for _, cmd := range []string{
		"gh pr edit 5 --add-label merge-queue",
		"gh pr edit --add-label=merge-queue 5",
		"gh pr edit 5 --add-label docs,merge-queue",
		"cd /tmp && gh pr edit 5 --title t --add-label merge-queue",
		"gh issue edit 5 --add-label merge-queue",
		"gh api repos/o/r/issues/5/labels -f labels[]=merge-queue",
		"gh api -X POST repos/o/r/issues/5/labels --input labels.json -f 'labels[]=merge-queue'",
	} {
		assertBlocked(t, guard(t, cwd, cmd), cmd, "Only `monacoctl agents land-stack <top-pr>` adds it")
	}
	for _, cmd := range []string{"gh pr edit 5 --add-label fast-track", "gh pr edit 5 --add-label=docs,fast-track",
		"gh issue edit 5 --add-label fast-track", "gh api repos/o/r/issues/5/labels -f labels[]=fast-track"} {
		assertBlocked(t, guard(t, cwd, cmd), cmd, "Only a human adds it")
	}
	for _, cmd := range []string{
		"gh pr edit 5 --add-label docs",
		"gh pr edit 5 --remove-label merge-queue",
		"gh pr edit 5 --remove-label fast-track",
		"gh pr edit 5 --add-label merge-queue-later",
		"gh api repos/o/r/issues/5/labels -f labels[]=docs",
		"gh api repos/o/r/labels",
	} {
		assertAllowed(t, guard(t, cwd, cmd), cmd)
	}
}

func TestAgentGuard_graphiteOwnsEveryBase(t *testing.T) {
	cwd := t.TempDir()
	for _, cmd := range []string{
		"gh pr edit 5 --base backend-rewrite-9",
		"gh pr edit 5 -B backend-rewrite-9",
		"gh pr edit --base=backend-rewrite-9 5",
		"cd /tmp && gh pr edit 5 --title t --base b",
	} {
		assertBlocked(t, guard(t, cwd, cmd), cmd, "gh pr edit --base is not allowed")
	}
	for _, cmd := range []string{
		"gh pr edit 5 --title t",
		"gh pr edit 5 --body-file body.md",
		"gh pr view 5 --json baseRefName",
		"cd apps/backend && go run ./cmd/monacoctl agents land-stack 5",
	} {
		assertAllowed(t, guard(t, cwd, cmd), cmd)
	}
}

func queuedGH(t *testing.T, heads string) []string {
	t.Helper()
	dir := t.TempDir()
	writeExecutable(t, filepath.Join(dir, "bin", "gh"), fmt.Sprintf(`#!/bin/sh
case "$1 $2" in
  "pr list") echo '%s' ;;
  *) exit 1 ;;
esac
`, heads))
	return []string{"PATH=" + filepath.Join(dir, "bin") + ":" + os.Getenv("PATH")}
}

func TestAgentGuard_noPushToAStackInTheGraphiteQueue(t *testing.T) {
	work, _ := pushRepo(t)
	git(t, work, "update-ref", "refs/remotes/origin/staging", "HEAD")
	git(t, work, "switch", "-q", "-c", "other")
	git(t, work, "switch", "-q", "ticket")
	git(t, work, "switch", "-q", "-c", "lower")
	git(t, work, "commit", "-q", "--allow-empty", "-m", "feat: lower")
	git(t, work, "switch", "-q", "-c", "upper")
	git(t, work, "commit", "-q", "--allow-empty", "-m", "feat: upper")
	env := queuedGH(t, `[{"number":7,"headRefName":"lower"}]`)
	for _, cmd := range []string{"git push origin upper", "git push origin lower", "gt submit --stack", "gt modify -a"} {
		assertBlocked(t, guard(t, work, cmd, env...), cmd, "#7 (lower) is in the Graphite merge queue")
	}
	git(t, work, "switch", "-q", "other")
	for _, cmd := range []string{"git push origin other", "gt submit"} {
		assertAllowed(t, guard(t, work, cmd, env...), "a stack outside the queue: "+cmd)
	}
	git(t, work, "switch", "-q", "upper")
	for name, heads := range map[string]string{"nothing queued": `[]`, "gh fails": `not json`} {
		assertAllowed(t, guard(t, work, "gt submit --stack", queuedGH(t, heads)...), name)
	}
}

func TestAgentGuard_editsOfGeneratedSwiftAreDenied(t *testing.T) {
	t.Parallel()
	edit := func(tool, file string) hookRun {
		return runHook(t, "agent-guard.py", map[string]any{
			"hook_event_name": "PreToolUse",
			"tool_name":       tool,
			"cwd":             t.TempDir(),
			"tool_input":      map[string]any{"file_path": file},
		})
	}
	for _, tool := range []string{"Edit", "Write", "MultiEdit"} {
		file := "/repo/packages/flows/Sources/MonacoFlows/Flow00.gen.swift"
		assertBlocked(t, edit(tool, file), tool+" "+file, "go run ./cmd/gen flows")
	}
	cases := "/repo/packages/mobile-core/Tests/MonacoAPITests/ErrorCodeCases.gen.swift"
	assertBlocked(t, edit("Edit", cases), "Edit "+cases, "go generate ./api")
	plain := "/repo/packages/mobile-core/Sources/MonacoCore/SignInModel.swift"
	assertAllowed(t, edit("Edit", plain), "Edit "+plain)
}

func runShellHook(t *testing.T, dir, script, input string, env ...string) (int, string, string) {
	t.Helper()
	cmd := exec.Command("bash", filepath.Join(dir, "scripts", script))
	cmd.Dir = t.TempDir()
	cmd.Env = append(os.Environ(), env...)
	cmd.Stdin = strings.NewReader(input)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		return exit.ExitCode(), stdout.String(), stderr.String()
	}
	if err != nil {
		t.Fatal(err)
	}
	return 0, stdout.String(), stderr.String()
}

func fileInLinkedWorktree(t *testing.T, repo, rel, src string) string {
	t.Helper()
	lane := filepath.Join(repo, ".worktrees", "lane")
	git(t, repo, "worktree", "add", "-q", "--detach", lane)
	path := filepath.Join(lane, rel)
	writeFile(t, path, src)
	return path
}

func fileInAnotherClone(t *testing.T, repo, rel, src string) string {
	t.Helper()
	clone := filepath.Join(t.TempDir(), "other")
	git(t, filepath.Dir(clone), "clone", "-q", repo, clone)
	path := filepath.Join(clone, rel)
	writeFile(t, path, src)
	return path
}

const (
	perfBudgets  = "apps/mobile/MonacoUITests/perf-budgets.tsv"
	raisedBudget = "Home\tlaunch_first_frame_ms\t1000\n"
)

func gatesHookRepo(t *testing.T) string {
	t.Helper()
	root := repoRoot(t)
	dir := t.TempDir()
	for _, rel := range []string{"scripts/agent-guard-gates.sh", "scripts/check-gate-changes.py"} {
		copyFile(t, filepath.Join(root, rel), filepath.Join(dir, rel))
	}
	writeFile(t, filepath.Join(dir, perfBudgets), "Home\tlaunch_first_frame_ms\t950\n")
	git(t, dir, "init", "-q")
	git(t, dir, "add", ".")
	git(t, dir, "-c", "user.name=t", "-c", "user.email=t@t", "-c", "commit.gpgsign=false", "commit", "-q", "-m", "base")
	return dir
}

func TestAgentGuardGates_checksAnEditInALinkedWorktree(t *testing.T) {
	t.Parallel()
	dir := gatesHookRepo(t)
	path := fileInLinkedWorktree(t, dir, perfBudgets, raisedBudget)

	code, stdout, stderr := runShellHook(t, dir, "agent-guard-gates.sh", claudeInput(path))

	want := perfBudgets + ":1: gate-file: raised `Home launch_first_frame_ms` 950 -> 1000."
	if code != 2 || stdout != "" || !strings.HasPrefix(stderr, want) {
		t.Fatalf("exit=%d stdout=%q stderr=%q, want 2, no stdout, stderr starting %q", code, stdout, stderr, want)
	}
}

func TestAgentGuardGates_ignoresAnEditInAnotherClone(t *testing.T) {
	t.Parallel()
	dir := gatesHookRepo(t)
	path := fileInAnotherClone(t, dir, perfBudgets, raisedBudget)

	code, stdout, stderr := runShellHook(t, dir, "agent-guard-gates.sh", claudeInput(path))

	if code != 0 || stdout != "" || stderr != "" {
		t.Fatalf("exit=%d stdout=%q stderr=%q, want 0 and no output", code, stdout, stderr)
	}
}
