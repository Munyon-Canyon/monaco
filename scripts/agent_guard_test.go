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

func TestAgentGuard_blocksPushesToMainTheFeatureBranchAndGraphiteTrunks(t *testing.T) {
	work, _ := pushRepo(t)
	trunks := `{"trunk":"main","trunks":[{"name":"main"},{"name":"milestone-9"}]}`
	if err := os.WriteFile(filepath.Join(work, ".git", ".graphite_repo_config"), []byte(trunks), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, cmd := range []string{
		"git push origin main",
		"git push origin HEAD:main",
		"git push origin ticket:refs/heads/backend-rewrite-4",
		"git push origin milestone-9",
		"git push -u origin ticket:milestone-9 2>&1 | tail -3",
		"git -C . push --all origin",
		"bash -c 'git push origin main'",
	} {
		r := guard(t, work, cmd)
		if r.code != 2 || !(strings.Contains(r.stderr, "squash-merged PRs") || strings.Contains(r.stderr, "--all")) {
			t.Errorf("%q: want blocked, got %d %q", cmd, r.code, r.stderr)
		}
	}
	git(t, work, "switch", "-q", "-c", "backend-rewrite-7")
	assertBlocked(t, guard(t, work, "git push"), "git push on backend-rewrite-7", "'backend-rewrite-7' is not allowed")
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
	dir := t.TempDir()
	calls = filepath.Join(dir, "calls")
	view := `{"number":42,"headRefOid":"0123456789abcdef0123456789abcdef01234567","url":"https://github.com/o/r/pull/42"}`
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

func TestAgentGuard_ghPrMergeNeedsVerifySuccessFromTheVerifierApp(t *testing.T) {
	cwd := t.TempDir()
	const bot, botID = "monaco-verifier[bot]", 334715092
	blocked := map[string]string{
		"no statuses":             `[]`,
		"verify posted by a user": `[` + status("success", "lognorman20", 1) + `]`,
		"latest verify failed":    `[` + status("failure", bot, botID) + `,` + status("success", bot, botID) + `]`,
		"impersonated bot login":  `[` + status("success", bot, 7) + `]`,
	}
	for name, statuses := range blocked {
		env, _ := ghStub(t, statuses)
		assertBlocked(t, guard(t, cwd, "gh pr merge 42 --squash", env...), name, "has no verify success from monaco-verifier[bot]")
	}
	env, calls := ghStub(t, `[`+status("success", bot, botID)+`,`+status("failure", bot, botID)+`]`)
	assertAllowed(t, guard(t, cwd, "gh pr merge 42 --squash --auto", env...), "verify success from the app")
	got, _ := os.ReadFile(calls)
	if !strings.Contains(string(got), "pr view 42 --json") {
		t.Errorf("want the PR selector passed to gh pr view, got %q", got)
	}
	env, _ = ghStub(t, `[`+status("success", bot, botID)+`]`)
	assertAllowed(t, guard(t, cwd, "gh pr view 42", env...), "gh pr view")
}

func TestAgentGuard_prBodiesComeFromFiles(t *testing.T) {
	cwd := t.TempDir()
	for _, cmd := range []string{
		`gh pr edit 5 --body "x"`,
		`gh pr edit 5 -b x`,
		`gh pr create --base b --title t --body="x"`,
	} {
		assertBlocked(t, guard(t, cwd, cmd), cmd, "scripts/pr-body.sh <pr> <file>")
	}
	for _, cmd := range []string{
		`gh pr edit 5 --body-file body.md`,
		`gh pr create --base b --title t --body-file body.md`,
		`scripts/pr-body.sh 5 body.md`,
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

func TestPrBody_setsTheBodyFromAFileOnlyWhenItPassesThePrFormat(t *testing.T) {
	dir := t.TempDir()
	calls := filepath.Join(dir, "calls")
	writeExecutable(t, filepath.Join(dir, "bin", "gh"), fmt.Sprintf(`#!/bin/sh
echo "$*" >> %q
[ "$1 $2" = "pr view" ] && echo "Add the thing"
exit 0
`, calls))
	env := append(os.Environ(), "PATH="+filepath.Join(dir, "bin")+":"+os.Getenv("PATH"))
	prBody := func(file string) (string, error) {
		script := filepath.Join(repoRoot(t), "scripts", "pr-body.sh")
		if _, err := os.ReadFile(script); err != nil {
			t.Fatal(err)
		}
		cmd := exec.Command("bash", script, "7", file)
		cmd.Env = env
		out, err := cmd.CombinedOutput()
		return string(out), err
	}

	bad := filepath.Join(dir, "bad.md")
	if err := os.WriteFile(bad, []byte("## TLDR\n\nOnly this.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if out, err := prBody(bad); err == nil || !strings.Contains(out, `missing the "## Why" section`) {
		t.Fatalf("want a format failure, got err=%v out=%q", err, out)
	}
	if got, _ := os.ReadFile(calls); strings.Contains(string(got), "pr edit") {
		t.Fatalf("gh pr edit ran for a body that fails the format: %q", got)
	}

	var body strings.Builder
	for _, s := range []string{"TLDR", "Why", "What changed", "Proof", "What came up", "Reviewer focus"} {
		fmt.Fprintf(&body, "## %s\n\nText.\n\n", s)
	}
	unlinked := filepath.Join(dir, "unlinked.md")
	if err := os.WriteFile(unlinked, []byte(body.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	if out, err := prBody(unlinked); err == nil || !strings.Contains(out, `"## Why" links no ticket`) {
		t.Fatalf("want a missing ticket link failure, got err=%v out=%q", err, out)
	}
	good := filepath.Join(dir, "good.md")
	linked := strings.Replace(body.String(), "## Why\n\nText.", "## Why\n\nPart of #7.", 1)
	if err := os.WriteFile(good, []byte(linked), 0o644); err != nil {
		t.Fatal(err)
	}
	if out, err := prBody(good); err != nil {
		t.Fatalf("pr-body.sh: %v\n%s", err, out)
	}
	if got, _ := os.ReadFile(calls); !strings.Contains(string(got), "pr edit 7 --body-file "+good) {
		t.Fatalf("want gh pr edit 7 --body-file, got %q", got)
	}
}
