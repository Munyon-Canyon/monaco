package scripts_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type roleRepo struct {
	primary, lane, checks string
}

func newRoleRepo(t *testing.T) roleRepo {
	t.Helper()
	t.Setenv("PYENV_VERSION", "system")
	dir := t.TempDir()
	primary, lane, remote := filepath.Join(dir, "primary"), filepath.Join(dir, "lane"), filepath.Join(dir, "remote.git")
	git(t, dir, "init", "-q", "--bare", "-b", "main", remote)
	git(t, dir, "init", "-q", "-b", "main", primary)
	for _, kv := range [][2]string{{"user.email", "a@example.com"}, {"user.name", "a"}, {"commit.gpgsign", "false"}} {
		git(t, primary, "config", kv[0], kv[1])
	}
	writeRoleFile(t, filepath.Join(primary, ".monaco", "agents.toml"), "milestone = \"ms\"\n")
	writeRoleFile(t, filepath.Join(primary, "apps", "backend", "go.mod"), "module x\n")
	writeRoleFile(t, filepath.Join(primary, "scripts", "go.mod"), "module y\n")
	git(t, primary, "add", "-A")
	git(t, primary, "commit", "-q", "-m", "chore: root")
	git(t, primary, "remote", "add", "origin", remote)
	git(t, primary, "worktree", "add", "-q", "-b", "ticket", lane)
	common := filepath.Join(primary, ".git")
	writeRoleFile(t, filepath.Join(common, ".monaco", "agents", "1.json"), `{"ticket":1,"worktree":"/elsewhere"}`)
	writeRoleFile(t, filepath.Join(common, ".monaco", "agents", "2.json"), `not json`)
	writeRoleFile(t, filepath.Join(common, ".monaco", "agents", "9.json"), `{"ticket":9,"worktree":"`+lane+`"}`)
	return roleRepo{primary, lane, filepath.Join(common, "pstack", "ms", "checks")}
}

func writeRoleFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

func (r roleRepo) markChecked(t *testing.T) {
	t.Helper()
	tree := strings.TrimSpace(gitOut(t, r.lane, "rev-parse", "HEAD^{tree}"))
	writeRoleFile(t, filepath.Join(r.checks, tree), "head x\n")
}

func TestAgentGuard_anOwnerPushesOnlyATreeThatAgentsCheckPassed(t *testing.T) {
	r := newRoleRepo(t)
	pushes := []string{"gt submit --stack --no-interactive --draft", "gt ss -d", "git push origin ticket"}
	for _, cmd := range pushes {
		assertBlocked(t, guard(t, r.lane, cmd), cmd, "`monacoctl agents check` has not passed on this tree")
		assertAllowed(t, guard(t, r.primary, cmd), "operator: "+cmd)
	}
	r.markChecked(t)
	for _, cmd := range pushes {
		assertAllowed(t, guard(t, r.lane, cmd), "checked: "+cmd)
	}
	assertAllowed(t, guard(t, r.primary, "git -C "+r.lane+" push origin ticket"), "git -C into the checked lane")
	commitFile(t, r.lane, "more.txt", "feat: more")
	assertBlocked(t, guard(t, r.lane, "gt submit --stack --draft"), "gt submit after a new commit", "has not passed")
	assertBlocked(t, guard(t, r.primary, "cd "+r.lane+" && git push origin ticket"), "cd into the lane", "has not passed")
}

func TestAgentGuard_anOwnerRunsNoHeavyTests(t *testing.T) {
	r := newRoleRepo(t)
	backend := filepath.Join(r.lane, "apps", "backend")
	heavy := map[string]string{
		"just test backend":                        r.lane,
		"just verify backend":                      r.lane,
		"just test":                                r.lane,
		"scripts/test-backend.sh":                  r.lane,
		"bash scripts/test-backend.sh -run X":      r.lane,
		"go test -race ./internal/x":               backend,
		"go test -short -race=true ./internal/x":   backend,
		"go test -count=1 ./internal/x":            backend,
		"go test -short=false ./internal/x":        backend,
		"go test -short ./...":                     backend,
		"cd apps/backend && go test -short ./...":  r.lane,
		"cd scripts && go test ./...":              r.lane,
		"if true; then just test backend; fi":      r.lane,
		"timeout 300 go test -race -short ./x/...": backend,
	}
	for cmd, cwd := range heavy {
		assertBlocked(t, guard(t, cwd, cmd), cmd, "do not run the full suite")
		assertAllowed(t, guard(t, filepath.Join(r.primary, strings.TrimPrefix(cwd, r.lane)), cmd), "operator: "+cmd)
	}
	for cmd, cwd := range map[string]string{
		"just test mobile": r.lane,
		"go test -short -count=1 ./internal/x ./cmd/api":     backend,
		"go test -short -race=false -count=1 ./internal/...": backend,
		"cd scripts && go test -short -count=1 ./...":        r.lane,
		"go vet ./...": backend,
	} {
		assertAllowed(t, guard(t, cwd, cmd), cmd)
	}
}

func TestAgentGuard_anOwnerDoesNotWaitOnCI(t *testing.T) {
	r := newRoleRepo(t)
	for _, cmd := range []string{
		"gh run watch 123",
		"gh pr checks 5 --watch",
		"gh pr checks 5 --watch --fail-fast",
		"while true; do gh pr checks 5; sleep 5; done",
		"until gh pr checks 5 >/dev/null; do sleep 5; done",
		"for i in 1 2 3; do gh pr checks 5; done",
		"watch -n 30 gh pr checks 5",
		"sleep 60",
		"sleep 11",
		"sleep 1m",
		"sleep 5 6",
		"sleep infinity",
		"gh pr view 5 && sleep 300",
	} {
		assertBlocked(t, guard(t, r.lane, cmd), cmd, "do not wait on CI")
	}
	for _, cmd := range []string{
		"gh pr checks 5",
		"gh pr checks 5 --json name,state",
		"sleep 10",
		"sleep 2.5 && echo",
		"sleep $DELAY",
		"gh run rerun 123 --failed",
		"gh run view 123 --log-failed",
	} {
		assertAllowed(t, guard(t, r.lane, cmd), cmd)
	}
	for _, cmd := range []string{"sleep 60", "gh run watch 123", "while true; do gh pr checks 5; done"} {
		assertAllowed(t, guard(t, r.primary, cmd), "operator: "+cmd)
	}
}

func TestAgentGuard_aQueuedStackRefusesGraphiteRewrites(t *testing.T) {
	r := newRoleRepo(t)
	record := filepath.Join(r.primary, ".git", ".monaco", "agents", "9.json")
	writeRoleFile(t, record, `{"ticket":9,"worktree":"`+r.lane+`","queued":{"top":12,"prs":[11,12]}}`)
	r.markChecked(t)
	rewrites := []string{
		"gt submit --stack --no-interactive --draft", "gt ss -d", "gt s -d", "gt modify -a", "gt m", "gt restack", "gt r",
	}
	for _, cmd := range rewrites {
		assertBlocked(t, guard(t, r.lane, cmd), cmd, "this stack is in the merge queue as #12")
		assertAllowed(t, guard(t, r.primary, cmd), "operator: "+cmd)
	}
	for _, cmd := range []string{"gt log", "gt checkout ticket", "git status"} {
		assertAllowed(t, guard(t, r.lane, cmd), cmd)
	}
	writeRoleFile(t, record, `{"ticket":9,"worktree":"`+r.lane+`"}`)
	for _, cmd := range rewrites {
		assertAllowed(t, guard(t, r.lane, cmd), "unmarked: "+cmd)
	}
}

func TestAgentGuard_anOwnerOpensDraftsAndMarksThemReadyOnlyThroughPrBody(t *testing.T) {
	r := newRoleRepo(t)
	r.markChecked(t)
	for cmd, want := range map[string]string{
		"gt submit --stack --no-interactive --publish":   "--publish marks every submitted PR ready",
		"gt ss --draft --publish":                        "--publish marks every submitted PR ready",
		"gt submit -dp":                                  "--publish marks every submitted PR ready",
		"gt submit --stack --no-interactive":             "without --draft opens a new PR ready",
		"gt ss":                                          "without --draft opens a new PR ready",
		"gt s --no-edit":                                 "without --draft opens a new PR ready",
		"gt submit --stack --draft=false":                "without --draft opens a new PR ready",
		"gh pr ready 5":                                  "gh pr ready skips the local PR format check",
		"gh pr ready":                                    "gh pr ready skips the local PR format check",
		"gh pr edit 5 --body-file b.md && gh pr ready 5": "gh pr ready skips the local PR format check",
	} {
		assertBlocked(t, guard(t, r.lane, cmd), cmd, "scripts/pr-body.sh <pr> <title> <body-file>")
		assertBlocked(t, guard(t, r.lane, cmd), cmd, want)
		assertAllowed(t, guard(t, r.primary, cmd), "operator: "+cmd)
	}
	assertBlocked(t, guard(t, r.primary, "cd "+r.lane+" && gt ss"), "cd into the lane", "without --draft")
	for _, cmd := range []string{
		"gt submit --stack --no-interactive --draft",
		"gt ss -d",
		"gt submit -nd --stack",
		"gt s --draft=true",
		`scripts/pr-body.sh 5 "Add the thing" body.md`,
		"gh pr ready 5 --undo",
		"gh pr view 5 --json isDraft",
	} {
		assertAllowed(t, guard(t, r.lane, cmd), cmd)
	}
}
