package agents

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"
)

type shellRule struct {
	prefix   string
	contains string
	out      string
	err      error
	skip     int
	times    int
	hits     int
}

type fakeShell struct {
	mu     sync.Mutex
	calls  []ghCall
	rules  []*shellRule
	before func(line string)
	next   Runner
}

func rules(rs ...shellRule) []*shellRule {
	out := make([]*shellRule, len(rs))
	for i := range rs {
		out[i] = &rs[i]
	}
	return out
}

func (sh *fakeShell) first(rs ...shellRule) {
	sh.mu.Lock()
	defer sh.mu.Unlock()
	sh.rules = append(rules(rs...), sh.rules...)
}

func (r *shellRule) applies(line string) bool {
	if !strings.HasPrefix(line, r.prefix) || !strings.Contains(line, r.contains) {
		return false
	}
	r.hits++
	return r.hits > r.skip && (r.times == 0 || r.hits <= r.skip+r.times)
}

func (sh *fakeShell) run(ctx context.Context, dir, stdin, name string, args ...string) ([]byte, error) {
	line := strings.TrimSpace(name + " " + strings.Join(args, " "))
	sh.mu.Lock()
	sh.calls = append(sh.calls, ghCall{dir, stdin, line})
	before := sh.before
	sh.mu.Unlock()
	if before != nil {
		before(line)
	}
	sh.mu.Lock()
	for _, r := range sh.rules {
		if r.applies(line) {
			sh.mu.Unlock()
			return []byte(r.out), r.err
		}
	}
	sh.mu.Unlock()
	return sh.next(ctx, dir, stdin, name, args...)
}

func (sh *fakeShell) lines(prefixes ...string) []string {
	sh.mu.Lock()
	defer sh.mu.Unlock()
	var out []string
	for _, c := range sh.calls {
		if slices.ContainsFunc(prefixes, func(p string) bool { return strings.HasPrefix(c.line, p) }) {
			out = append(out, c.line)
		}
	}
	return out
}

func (sh *fakeShell) dirOf(prefix string) string {
	sh.mu.Lock()
	defer sh.mu.Unlock()
	for _, c := range sh.calls {
		if strings.HasPrefix(c.line, prefix) {
			return c.dir
		}
	}
	return ""
}

const (
	baseSHA  = "0123456789abcdef0123456789abcdef01234567"
	firstSHA = "1111111111111111111111111111111111111111"
	lastSHA  = "2222222222222222222222222222222222222222"
)

type directFixture struct {
	t  *testing.T
	f  *fixture
	s  *stackGH
	sh *fakeShell
}

func attest(f *fixture, head, tree string) {
	f.hub.on(list("/commits/"+head+"/statuses?"), []GHStatus{
		{
			Context: "stage0/ok", State: "success", CreatedAt: f.now,
			Description: "tree " + tree + " rows go build,xcode",
		},
		{Context: "verify", State: "success", CreatedAt: f.now},
	})
}

func newDirect(t *testing.T, bottom, top *stackPR) *directFixture {
	t.Helper()
	f := newFixture(t)
	bottom.Title, top.Title = "First change", "Second change"
	s := newStackGH(t, f, bottom, top)
	sh := &fakeShell{next: s.run, rules: rules([]shellRule{
		{prefix: "git rev-parse b1-oid^{tree}", out: "tree-b1\n"},
		{prefix: "git rev-parse b2-oid^{tree}", out: "tree-b2\n"},
		{prefix: "git merge-base origin/fb b2-oid", out: baseSHA + "\n"},
		{prefix: "git diff --name-only " + baseSHA, out: ""},
		{prefix: "git merge-tree --write-tree origin/fb b2-oid", out: "mergedtree\n"},
		{prefix: "git merge-tree --write-tree --name-only", out: "simtree\n"},
		{prefix: "git commit-tree simtree", out: "simcommit\n"},
		{prefix: "git commit-tree mergedtree", out: "mergedcommit\n"},
		{prefix: "git worktree", out: ""},
		{prefix: "cp ", out: ""},
		{prefix: "scripts/ci/ready.sh", out: ""},
	}...)}
	f.run = sh.run
	f.owner(t, Record{Ticket: 40, Worktree: f.dir, State: Done})
	attest(f, "b1-oid", "tree-b1")
	attest(f, "b2-oid", "tree-b2")
	for n, sha := range map[int]string{1: firstSHA, 2: lastSHA} {
		f.hub.on(fmt.Sprintf("PUT /repos/o/r/pulls/%d/merge", n), fmt.Sprintf(`{"sha":%q,"merged":true}`, sha))
		f.hub.on(fmt.Sprintf("DELETE /repos/o/r/git/refs/heads/b%d", n), "")
	}
	f.hub.on("PATCH /repos/o/r/pulls/2", "{}")
	f.hub.on(get("/pulls/1"), `{"state":"open"}`)
	f.hub.on(get("/pulls/2"), `{"state":"open","mergeable":true}`)
	f.hub.on("POST /repos/o/r/actions/workflows/staging.yml/dispatches", "")
	return &directFixture{t: t, f: f, s: s, sh: sh}
}

func (f *fixture) root() string {
	root, err := filepath.EvalSymlinks(f.dir)
	if err != nil {
		panic(err)
	}
	return root
}

func (d *directFixture) mergeCalls() []string {
	var out []string
	for _, c := range d.f.hub.callsContaining("") {
		if strings.Contains(c, "/pulls/") && !strings.Contains(c, "/files") || strings.Contains(c, "/git/refs/") ||
			strings.Contains(c, "dispatches") {
			out = append(out, c)
		}
	}
	return out
}

func TestLandStack_sharesTheNextStagingRunWhileADispatchedRunIsQueued(t *testing.T) {
	t.Parallel()
	d := newDirect(t, green(t, 1, "b1", "fb"), green(t, 2, "b2", "b1"))
	f := d.f
	at := f.now.Add(-queuedLookback).UTC().Format(time.RFC3339)
	f.hub.on(
		get("/actions/workflows/staging.yml/runs?event=workflow_dispatch&branch=fb&created=>="+at+"&per_page=100"),
		map[string]any{"workflow_runs": []Run{
			{ID: 5, Status: "in_progress", HeadSHA: baseSHA},
			{ID: 6, Status: "queued", HeadSHA: baseSHA},
		}},
	)
	code, stdout, stderr := f.agents(t, "land-stack", "2")
	want := "staging run for 222222222222 will share the next run (a dispatched run is still queued)\n"
	if code != 0 || !strings.HasSuffix(stdout, want) || stderr != "" {
		t.Fatalf("%d %q %q", code, stdout, stderr)
	}
	if got := f.hub.callsContaining("dispatches"); len(got) != 0 {
		t.Fatalf("dispatched %v", got)
	}
	if r := f.owned(t); len(r.Landed) != 1 || r.Landed[0].SHA != lastSHA || r.Landed[0].RunID != 0 {
		t.Fatalf("landed %+v", r.Landed)
	}
}

func TestLandStack_dispatchesWhenItCannotListTheQueuedRuns(t *testing.T) {
	t.Parallel()
	d := newDirect(t, green(t, 1, "b1", "fb"), green(t, 2, "b2", "b1"))
	f := d.f
	at := f.now.Add(-queuedLookback).UTC().Format(time.RFC3339)
	route := get("/actions/workflows/staging.yml/runs?event=workflow_dispatch&branch=fb&created=>=" +
		at + "&per_page=100")
	f.hub.status[route] = 500
	f.hub.on(route, "{}")
	_, stdout, _ := f.agents(t, "land-stack", "2")
	if !strings.Contains(stdout, "watch error: list queued staging runs: ") ||
		!strings.Contains(stdout, "dispatched staging run for 222222222222") {
		t.Fatalf("%q", stdout)
	}
}

func TestLandStack_squashesAnAttestedStackBottomUp(t *testing.T) {
	t.Parallel()
	d := newDirect(t, green(t, 1, "b1", "fb"), green(t, 2, "b2", "b1"))
	f := d.f
	code, stdout, stderr := f.agents(t, "land-stack", "2")
	want := "landing #1 #2 on stage 0\n" +
		"trunk unchanged under apps/backend and packages/flows since 0123456789ab; stage 0's ready row stands\n" +
		"#1 landed as 111111111111\n#2 landed as 222222222222\n" +
		"dispatched staging run for 222222222222; agents watch reverts #1 #2 if it fails\n"
	if code != 0 || stdout != want || stderr != "" {
		t.Fatalf("%d %q %q", code, stdout, stderr)
	}
	wantCalls := []string{
		"GET /repos/o/r/pulls/1", "GET /repos/o/r/pulls/2", "PUT /repos/o/r/pulls/1/merge",
		"PATCH /repos/o/r/pulls/2", "GET /repos/o/r/pulls/2", "DELETE /repos/o/r/git/refs/heads/b1",
		"PUT /repos/o/r/pulls/2/merge",
		"DELETE /repos/o/r/git/refs/heads/b2", "POST /repos/o/r/actions/workflows/staging.yml/dispatches",
	}
	if got := d.mergeCalls(); !slices.Equal(got, wantCalls) {
		t.Fatalf("calls:\n%s", strings.Join(got, "\n"))
	}
	wantBody := `{"commit_message":"Part of #40\n\n## TLDR\nx","commit_title":"Second change (#2)",` +
		`"merge_method":"squash","sha":"b2-oid"}`
	if got := f.hub.body("PUT /repos/o/r/pulls/2/merge"); got != wantBody {
		t.Fatalf("merge body %q", got)
	}
	if got := f.hub.body("PATCH /repos/o/r/pulls/2"); got != `{"base":"fb"}` {
		t.Fatalf("patch body %q", got)
	}
	if got := f.hub.body("POST /repos/o/r/actions/workflows/staging.yml/dispatches"); got !=
		`{"inputs":{"sha":"`+lastSHA+`"},"ref":"fb"}` {
		t.Fatalf("dispatch body %q", got)
	}
	r := f.owned(t)
	landed := Landing{Top: 2, PRs: []int{1, 2}, SHA: lastSHA}
	if len(r.Landed) != 1 || r.Landed[0].Top != landed.Top || r.Landed[0].SHA != landed.SHA ||
		!slices.Equal(r.Landed[0].PRs, landed.PRs) || r.Landed[0].Checked || len(r.Queued) != 0 ||
		r.Settled == nil || r.Settled.Outcome != outcomeLanded {
		t.Fatalf("record %+v", r)
	}
	if got := f.hub.callsContaining("/labels"); len(got) != 0 {
		t.Fatalf("labelled %v", got)
	}
}

func TestLandStack_refusesAStackWithOneUnattestedHead(t *testing.T) {
	t.Parallel()
	d := newDirect(t, green(t, 1, "b1", "fb"), green(t, 2, "b2", "b1"))
	d.f.hub.on(list("/commits/b1-oid/statuses?"), []GHStatus{{Context: "verify", State: "success"}})
	code, stdout, stderr := d.f.agents(t, "land-stack", "2")
	want := "#1 has no stage 0 attestation for head b1-oid; run monacoctl agents check and scripts/pr-body.sh on it, " +
		"then land-stack again"
	if code != 1 || stdout != "" || !strings.Contains(stderr, want) {
		t.Fatalf("%d %q %q", code, stdout, stderr)
	}
	if got := d.mergeCalls(); len(got) != 0 {
		t.Fatalf("merged %v", got)
	}
	if got := d.f.hub.callsContaining("/labels"); len(got) != 0 {
		t.Fatalf("labelled %v", got)
	}
}

func TestLandStack_refusesAnAttestationOfAnotherTree(t *testing.T) {
	t.Parallel()
	d := newDirect(t, green(t, 1, "b1", "fb"), green(t, 2, "b2", "b1"))
	attest(d.f, "b2-oid", "some-other-tree")
	code, _, stderr := d.f.agents(t, "land-stack", "2")
	if code != 1 || !strings.Contains(stderr, "#2 has no stage 0 attestation for head b2-oid") {
		t.Fatalf("%d %q", code, stderr)
	}
	if got := d.mergeCalls(); len(got) != 0 {
		t.Fatalf("merged %v", got)
	}
}

func TestLandStack_runsStage2OnTheMergedTreeWhenTrunkMoved(t *testing.T) {
	t.Parallel()
	moved := shellRule{prefix: "git diff --name-only " + baseSHA, out: "apps/backend/a.go\npackages/flows/b.go\n"}
	t.Run("a failing ready.sh refuses with its tail", func(t *testing.T) {
		t.Parallel()
		d := newDirect(t, green(t, 1, "b1", "fb"), green(t, 2, "b2", "b1"))
		d.sh.first(moved, shellRule{
			prefix: "scripts/ci/ready.sh", out: "go generate: stale\n", err: errors.New("exit status 1"),
		})
		code, stdout, stderr := d.f.agents(t, "land-stack", "2")
		want := "stage 2 on the merged tree failed: go generate: stale\nexit status 1. " +
			"Restack with gt restack, run monacoctl agents check, then land-stack again"
		if code != 1 || stdout != "landing #1 #2 on stage 0\n" || !strings.Contains(stderr, want) {
			t.Fatalf("%d %q %q", code, stdout, stderr)
		}
		if got := d.mergeCalls(); len(got) != 0 {
			t.Fatalf("merged %v", got)
		}
		if got := d.sh.lines("git worktree remove"); len(got) != 1 {
			t.Fatalf("worktree removals %v", got)
		}
	})
	t.Run("a passing ready.sh lands", func(t *testing.T) {
		t.Parallel()
		d := newDirect(t, green(t, 1, "b1", "fb"), green(t, 2, "b2", "b1"))
		d.sh.first(moved)
		code, stdout, stderr := d.f.agents(t, "land-stack", "2")
		if code != 0 || stderr != "" || !strings.Contains(stdout,
			"stage 2 on the merged tree (trunk moved in 2 files since 0123456789ab): ok in ") ||
			!strings.Contains(stdout, "#2 landed as 222222222222\n") {
			t.Fatalf("%d %q %q", code, stdout, stderr)
		}
		wt := d.f.root() + "/.worktrees/.land-2"
		want := []string{
			"git worktree add --detach " + wt + " mergedcommit",
			"scripts/ci/ready.sh",
			"git worktree remove --force --force " + wt,
			"git worktree prune",
		}
		if got := d.sh.lines("git worktree", "scripts/ci/ready.sh"); !slices.Equal(got, want) {
			t.Fatalf("calls %v", got)
		}
		if got := d.sh.dirOf("scripts/ci/ready.sh"); got != wt {
			t.Fatalf("ready.sh ran in %s", got)
		}
		if got := d.sh.lines("git commit-tree mergedtree"); len(got) != 1 ||
			got[0] != "git commit-tree mergedtree -p origin/fb -m land-stack #2: stage 2 on the merged tree" {
			t.Fatalf("commit-tree %v", got)
		}
	})
}

func TestLandStack_armsWhenCIOkStaysPendingTenMinutes(t *testing.T) {
	t.Parallel()
	d := newDirect(t, green(t, 1, "b1", "fb"), stackOf(t, 2, "b2", "b1", "pending", "SUCCESS"))
	code, stdout, stderr := d.f.agents(t, "land-stack", "2")
	want := "landing #1 #2 on stage 0\nci-ok still pending after 10m; armed #2 for the queue path\n" +
		"armed #2; agents watch lands it once stage 1 passes (waiting on #2 (stage 1 pending))\n"
	if code != 0 || stdout != want || stderr != "" {
		t.Fatalf("%d %q %q", code, stdout, stderr)
	}
	if len(d.f.waited) != 30 || d.f.waited[0] != 20*time.Second {
		t.Fatalf("waited %d times: %v", len(d.f.waited), d.f.waited)
	}
	if got := d.mergeCalls(); len(got) != 0 {
		t.Fatalf("merged %v", got)
	}
	if r := d.f.owned(t); len(r.Armed) != 1 || r.Armed[0].Top != 2 || len(r.Landed) != 0 {
		t.Fatalf("record %+v", r)
	}
}

func TestLandStack_landsOnceCIOkTurnsGreenWhileItWaits(t *testing.T) {
	t.Parallel()
	d := newDirect(t, green(t, 1, "b1", "fb"), stackOf(t, 2, "b2", "b1", "pending", "SUCCESS"))
	d.f.onWait = func(n int) {
		if n == 3 {
			*d.s.prs[2] = *green(t, 2, "b2", "b1")
			d.s.prs[2].Title = "Second change"
		}
	}
	code, stdout, stderr := d.f.agents(t, "land-stack", "2")
	if code != 0 || stderr != "" || !strings.Contains(stdout, "#2 landed as 222222222222\n") || len(d.f.waited) != 3 {
		t.Fatalf("%d %q %q waited %d", code, stdout, stderr, len(d.f.waited))
	}
}

func TestLandStack_takesTheQueuePathWhenTheTopIsNotAttested(t *testing.T) {
	t.Parallel()
	d := newDirect(t, green(t, 1, "b1", "fb"), green(t, 2, "b2", "b1"))
	d.f.hub.on(list("/commits/b2-oid/statuses?"), []GHStatus{})
	code, stdout, stderr := d.f.agents(t, "land-stack", "2")
	if code != 0 || !strings.HasPrefix(stdout, "queued #1 #2\n") || stderr != "" {
		t.Fatalf("%d %q %q", code, stdout, stderr)
	}
	want := []string{"POST /repos/o/r/issues/2/labels", "POST /repos/o/r/issues/1/labels"}
	if got := d.f.hub.callsContaining("/labels"); !slices.Equal(got, want) {
		t.Fatalf("labels %v", got)
	}
	if got := d.mergeCalls(); len(got) != 0 {
		t.Fatalf("merged %v", got)
	}
}

func TestLandStack_recordsTheLandedPrefixWhenAHigherMergeFails(t *testing.T) {
	t.Parallel()
	d := newDirect(t, green(t, 1, "b1", "fb"), green(t, 2, "b2", "b1"))
	d.f.hub.on("PUT /repos/o/r/pulls/2/merge", `{"message":"Head branch was modified"}`)
	d.f.hub.status["PUT /repos/o/r/pulls/2/merge"] = 409
	code, stdout, stderr := d.f.agents(t, "land-stack", "2")
	if code != 1 || !strings.Contains(stderr, "#2 did not merge: Head branch was modified") ||
		!strings.Contains(stderr, "#1 is on fb; #2 is not: fix and run land-stack 2 again") ||
		!strings.Contains(stdout, "#1 landed as 111111111111\n") {
		t.Fatalf("%d %q %q", code, stdout, stderr)
	}
	r := d.f.owned(t)
	if len(r.Landed) != 1 || r.Landed[0].Top != 1 || r.Landed[0].SHA != firstSHA || r.Settled != nil {
		t.Fatalf("record %+v", r)
	}
	if got := d.f.hub.callsContaining("DELETE /repos/o/r/git/refs/heads/b2"); len(got) != 0 {
		t.Fatalf("deleted the unmerged branch: %v", got)
	}
}

func TestLandStack_refusesWhenAnAttestationOrMergeStepFails(t *testing.T) {
	t.Parallel()
	boom := errors.New("boom")
	becomesConflicting := func(d *directFixture) {
		d.sh.before = func(line string) {
			if strings.Contains(line, "p1: pullRequest(number:1)") {
				d.s.mu.Lock()
				d.s.prs[2].Mergeable = conflicting
				d.s.mu.Unlock()
			}
		}
	}
	for _, tc := range []struct {
		name, want string
		setup      func(d *directFixture)
	}{
		{"the top's statuses cannot be read", "statuses", func(d *directFixture) {
			d.f.hub.on(list("/commits/b2-oid/statuses?"), "[]")
			d.f.hub.status[list("/commits/b2-oid/statuses?")] = 500
		}},
		{"a lower PR's statuses cannot be read", "statuses", func(d *directFixture) {
			d.f.hub.status[list("/commits/b1-oid/statuses?")] = 500
		}},
		{"the tree of a head cannot be read", "boom", func(d *directFixture) {
			d.sh.first(shellRule{prefix: "git rev-parse b2-oid^{tree}", err: boom})
		}},
		{"the bottom does not merge with the trunk", "boom", func(d *directFixture) {
			d.sh.first(shellRule{prefix: "git fetch --quiet origin fb b1-oid", err: boom})
		}},
		{"the merge base cannot be found", "boom", func(d *directFixture) {
			d.sh.first(shellRule{prefix: "git merge-base", err: boom})
		}},
		{"stage 2 cannot build the merged tree", "boom", func(d *directFixture) {
			d.sh.first(shellRule{prefix: "git diff --name-only", out: "apps/backend/a.go\n"},
				shellRule{prefix: "git merge-tree --write-tree origin/fb b2-oid", err: boom})
		}},
		{"the stack is read again and a PR now conflicts", "conflicts with", becomesConflicting},
		{"the stack cannot be read again", "boom", func(d *directFixture) {
			d.sh.first(shellRule{prefix: "gh api graphql", contains: "p1: pullRequest(number:1)", err: boom})
		}},
		{"the bottom does not merge", "#1 did not merge: Head branch was modified", func(d *directFixture) {
			d.f.hub.on("PUT /repos/o/r/pulls/1/merge", `{"message":"Head branch was modified"}`)
			d.f.hub.status["PUT /repos/o/r/pulls/1/merge"] = 409
		}},
		{"the merge answer is not JSON", "decode PUT", func(d *directFixture) {
			d.f.hub.on("PUT /repos/o/r/pulls/1/merge", "not json")
		}},
		{"the second PR cannot be retargeted", "PATCH /repos/o/r/pulls/2", func(d *directFixture) {
			d.f.hub.status["PATCH /repos/o/r/pulls/2"] = 500
		}},
		{
			"the second PR is not mergeable once the first landed", "#2 is not mergeable onto fb after #1 landed",
			func(d *directFixture) { d.f.hub.on(get("/pulls/2"), `{"mergeable":false}`) },
		},
		{
			"GitHub never decides the second PR's mergeability", "#2 mergeability onto fb is still unknown after 1m0s",
			func(d *directFixture) { d.f.hub.on(get("/pulls/2"), `{"mergeable":null}`) },
		},
		{"the mergeability cannot be read", "GET /repos/o/r/pulls/2", func(d *directFixture) {
			d.f.hub.status[get("/pulls/2")] = 500
		}},
		{"the record cannot be written", "write owner record", func(d *directFixture) {
			freeze(d.t, d.f.Env(d.t).recordPath(40))
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			d := newDirect(t, green(t, 1, "b1", "fb"), green(t, 2, "b2", "b1"))
			tc.setup(d)
			code, _, stderr := d.f.agents(t, "land-stack", "2")
			if code != 1 || !strings.Contains(stderr, tc.want) {
				t.Fatalf("%d %q, want %q", code, stderr, tc.want)
			}
		})
	}
}

func TestLandStack_readsTheTreeAfterFetchingAHeadItLacks(t *testing.T) {
	t.Parallel()
	d := newDirect(t, green(t, 1, "b1", "fb"), green(t, 2, "b2", "b1"))
	d.sh.first(shellRule{prefix: "git rev-parse b1-oid^{tree}", err: errors.New("bad object"), times: 1})
	code, stdout, stderr := d.f.agents(t, "land-stack", "2")
	if code != 0 || !strings.Contains(stdout, "#2 landed as") || stderr != "" {
		t.Fatalf("%d %q %q", code, stdout, stderr)
	}
	if got := d.sh.lines("git fetch --quiet origin b1-oid"); len(got) != 1 {
		t.Fatalf("fetches %v", got)
	}
}

func TestLandStack_armsAndStopsStage2WhenCIOkStaysPendingAfterTheTrunkMoved(t *testing.T) {
	t.Parallel()
	d := newDirect(t, green(t, 1, "b1", "fb"), stackOf(t, 2, "b2", "b1", "pending", "SUCCESS"))
	d.sh.first(shellRule{prefix: "git diff --name-only", out: "apps/backend/a.go\n"})
	code, stdout, stderr := d.f.agents(t, "land-stack", "2")
	if code != 0 || stderr != "" || !strings.Contains(stdout, "ci-ok still pending after 10m; armed #2") {
		t.Fatalf("%d %q %q", code, stdout, stderr)
	}
}

func TestLandStack_reportsWhatLandedWhenABranchOrTheDispatchFails(t *testing.T) {
	t.Parallel()
	d := newDirect(t, green(t, 1, "b1", "fb"), green(t, 2, "b2", "b1"))
	d.f.hub.status["DELETE /repos/o/r/git/refs/heads/b1"] = 500
	d.f.hub.status["DELETE /repos/o/r/git/refs/heads/b2"] = 422
	d.f.hub.status["POST /repos/o/r/actions/workflows/staging.yml/dispatches"] = 500
	code, stdout, stderr := d.f.agents(t, "land-stack", "2")
	if code != 0 || stderr != "" ||
		!strings.Contains(stdout, "#1: branch b1 was not deleted: DELETE /repos/o/r/git/refs/heads/b1: 500") ||
		strings.Contains(stdout, "#2: branch") ||
		!strings.Contains(
			stdout,
			"watch error: dispatch staging run: POST /repos/o/r/actions/workflows/staging.yml/dispatches",
		) {
		t.Fatalf("%d %q %q", code, stdout, stderr)
	}
}

func TestLandStack_refusesWithTheLastLinesOfAFailedStage2(t *testing.T) {
	t.Parallel()
	d := newDirect(t, green(t, 1, "b1", "fb"), green(t, 2, "b2", "b1"))
	var many strings.Builder
	for i := range 30 {
		fmt.Fprintf(&many, "line %d\n", i)
	}
	d.sh.first(shellRule{prefix: "git diff --name-only", out: "apps/backend/a.go\n"},
		shellRule{prefix: "scripts/ci/ready.sh", out: many.String(), err: errors.New("exit status 1")})
	_, _, stderr := d.f.agents(t, "land-stack", "2")
	if strings.Contains(stderr, "line 9\n") || !strings.Contains(stderr, "line 29\n") {
		t.Fatalf("%q", stderr)
	}
}

func TestSquash_refusesABottomPRWhoseBaseIsNotTheTrunk(t *testing.T) {
	t.Parallel()
	env := newFixture(t).Env(t)
	stack := []stackPR{{gqlPR: gqlPR{Number: 7}, Base: "other"}}
	_, err := env.squash(t.Context(), stack, 0, "fb")
	if got := cliText(err); got != "#7 is the bottom of its stack but its base other is not fb" {
		t.Fatalf("%q", got)
	}
}

func TestLandStack_retargetsTheUpperPRBeforeDeletingTheLowerBranch(t *testing.T) {
	t.Parallel()
	d := newDirect(t, green(t, 1, "b1", "fb"), green(t, 2, "b2", "b1"))
	if code, stdout, stderr := d.f.agents(t, "land-stack", "2"); code != 0 {
		t.Fatalf("%d %q %q", code, stdout, stderr)
	}
	var got []string
	for _, c := range d.mergeCalls() {
		if strings.HasPrefix(c, "PATCH") || strings.HasPrefix(c, "DELETE") || strings.HasPrefix(c, "PUT") ||
			c == "GET /repos/o/r/pulls/2" {
			got = append(got, c)
		}
	}
	want := []string{
		"GET /repos/o/r/pulls/2", "PUT /repos/o/r/pulls/1/merge", "PATCH /repos/o/r/pulls/2",
		"GET /repos/o/r/pulls/2", "DELETE /repos/o/r/git/refs/heads/b1", "PUT /repos/o/r/pulls/2/merge",
		"DELETE /repos/o/r/git/refs/heads/b2",
	}
	if !slices.Equal(got, want) {
		t.Fatalf("calls:\n%s", strings.Join(got, "\n"))
	}
}

func TestLandStack_reopensAPRThatGitHubClosedOnBaseDeletion(t *testing.T) {
	t.Parallel()
	d := newDirect(t, green(t, 1, "b1", "fb"), green(t, 2, "b2", "b1"))
	d.f.hub.on(get("/pulls/2"), `{"state":"closed","mergeable":true}`)
	code, stdout, stderr := d.f.agents(t, "land-stack", "2")
	if code != 0 || !strings.Contains(
		stdout,
		"#2 was closed by GitHub when its base branch went away; reopened\n",
	) || !strings.Contains(stdout, "#2 landed as") {
		t.Fatalf("%d %q %q", code, stdout, stderr)
	}
	if got := d.f.hub.callsContaining("PATCH /repos/o/r/pulls/2"); len(got) != 2 {
		t.Fatalf("patches %v", got)
	}
}

func TestLandStack_reopensAClosedTopAndWalksTheStackAgain(t *testing.T) {
	t.Parallel()
	top := green(t, 2, "b2", "fb")
	top.State = "CLOSED"
	d := newDirect(t, green(t, 1, "b1", "fb"), top)
	d.f.hub.on(get("/compare/fb...b2-oid"), `{"status":"diverged"}`)
	d.f.hub.hook = func(method, path, body string, status int) {
		d.s.onLabel(method, path, body, status)
		if method == "PATCH" && strings.HasSuffix(path, "/pulls/2") && strings.Contains(body, `"open"`) {
			d.s.mu.Lock()
			top.State = "OPEN"
			d.s.mu.Unlock()
		}
	}
	code, stdout, stderr := d.f.agents(t, "land-stack", "2")
	if code != 0 || !strings.Contains(stdout, "#2 was closed by GitHub when its base branch went away; reopened\n") {
		t.Fatalf("%d %q %q", code, stdout, stderr)
	}
	closed := green(t, 2, "b2", "fb")
	closed.State = "CLOSED"
	d2 := newDirect(t, green(t, 1, "b1", "fb"), closed)
	d2.f.hub.on(get("/compare/fb...b2-oid"), `{"status":"diverged"}`)
	d2.f.hub.status["PATCH /repos/o/r/pulls/2"] = 500
	if code, _, stderr := d2.f.agents(
		t,
		"land-stack",
		"2",
	); code != 1 ||
		!strings.Contains(stderr, "PATCH /repos/o/r/pulls/2") {
		t.Fatalf("%d %q", code, stderr)
	}
	unknown := green(t, 2, "b2", "fb")
	unknown.State = "CLOSED"
	d3 := newDirect(t, green(t, 1, "b1", "fb"), unknown)
	if code, _, stderr := d3.f.agents(
		t,
		"land-stack",
		"2",
	); code != 1 ||
		!strings.Contains(stderr, "compare b2-oid with fb") {
		t.Fatalf("%d %q", code, stderr)
	}
}

func TestLandStack_fallsBackToTheQueueWhenAnUpperPRWouldConflictAfterTheLowerSquash(t *testing.T) {
	t.Parallel()
	d := newDirect(t, green(t, 1, "b1", "fb"), green(t, 2, "b2", "b1"))
	d.sh.first(shellRule{
		prefix: "git merge-tree --write-tree --name-only --no-messages simcommit b2-oid",
		out:    "badtree\nripple.go\nother.go\n", err: exitOne(t),
	})
	code, stdout, stderr := d.f.agents(t, "land-stack", "2")
	want := "#2 would conflict with fb after #1 squashes (ripple.go, other.go); landing through the queue instead\n"
	if code != 0 || !strings.Contains(stdout, want) || !strings.Contains(stdout, "queued #1 #2") {
		t.Fatalf("%d %q %q", code, stdout, stderr)
	}
	if got := d.f.hub.callsContaining("/merge"); len(got) != 0 {
		t.Fatalf("squashed %v", got)
	}
	if got := d.sh.lines("git commit-tree simtree -p origin/fb"); len(got) != 1 {
		t.Fatalf("simulated squashes %v", got)
	}
}

func TestSquashConflict_staysQuietForABottomConflictAndFailsOnGitErrors(t *testing.T) {
	t.Parallel()
	d := newDirect(t, green(t, 1, "b1", "fb"), green(t, 2, "b2", "b1"))
	d.sh.first(shellRule{
		prefix: "git merge-tree --write-tree --name-only --no-messages origin/fb b1-oid",
		out:    "badtree\na.go\n", err: exitOne(t),
	})
	env := d.f.Env(t)
	stack := []stackPR{*green(t, 1, "b1", "fb"), *green(t, 2, "b2", "b1")}
	if got, err := env.squashConflict(t.Context(), d.f.dir, stack); got != "" || err != nil {
		t.Fatalf("%q %v", got, err)
	}
	if got, err := env.squashConflict(t.Context(), d.f.dir, stack[:1]); got != "" || err != nil {
		t.Fatalf("%q %v", got, err)
	}
	d.sh.first(shellRule{prefix: "git merge-tree --write-tree --name-only", err: errors.New("boom")})
	if _, err := env.squashConflict(t.Context(), d.f.dir, stack); err == nil {
		t.Fatal("merge-tree error")
	}
}

func TestLandStack_reportsAGitOrGitHubErrorWhileSimulatingOrReopening(t *testing.T) {
	t.Parallel()
	boom := errors.New("boom")
	for _, tc := range []struct {
		name, want string
		setup      func(d *directFixture)
	}{
		{"merge-tree fails", "boom", func(d *directFixture) {
			d.sh.first(shellRule{prefix: "git merge-tree --write-tree --name-only", err: boom})
		}},
		{"commit-tree fails", "boom", func(d *directFixture) {
			d.sh.first(shellRule{prefix: "git commit-tree simtree", err: boom})
		}},
		{"the reopen is refused", "PATCH /repos/o/r/pulls/2", func(d *directFixture) {
			d.f.hub.on(get("/pulls/2"), `{"state":"closed"}`)
			d.f.hub.status["PATCH /repos/o/r/pulls/2"] = 500
		}},
		{"the mergeability cannot be read after the retarget", "GET /repos/o/r/pulls/2", func(d *directFixture) {
			d.f.hub.hook = func(method, path, _ string, _ int) {
				if method == "PATCH" && strings.HasSuffix(path, "/pulls/2") {
					d.f.hub.mu.Lock()
					d.f.hub.status[get("/pulls/2")] = 500
					d.f.hub.mu.Unlock()
				}
			}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			d := newDirect(t, green(t, 1, "b1", "fb"), green(t, 2, "b2", "b1"))
			tc.setup(d)
			if code, _, stderr := d.f.agents(t, "land-stack", "2"); code != 1 || !strings.Contains(stderr, tc.want) {
				t.Fatalf("%d %q, want %q", code, stderr, tc.want)
			}
		})
	}
}

func TestSquash_refusesAnUpperPRWhoseBaseWasNotRetargeted(t *testing.T) {
	t.Parallel()
	env := newFixture(t).Env(t)
	stack := []stackPR{{gqlPR: gqlPR{Number: 7}, Base: "fb"}, {gqlPR: gqlPR{Number: 8}, Base: "b7"}}
	_, err := env.squash(t.Context(), stack, 1, "fb")
	if got := cliText(err); got != "#8's base b7 is not fb after #7 landed" {
		t.Fatalf("%q", got)
	}
}

func TestStage2_removesTheWorktreeWhenAddIsInterrupted(t *testing.T) {
	t.Parallel()
	d := newDirect(t, green(t, 1, "b1", "fb"), green(t, 2, "b2", "b1"))
	d.sh.first(shellRule{prefix: "git diff --name-only", out: "apps/backend/a.go\n"},
		shellRule{prefix: "git worktree add", err: errors.New("signal: killed")})
	code, _, _ := d.f.agents(t, "land-stack", "2")
	if code != 1 {
		t.Fatalf("exit %d", code)
	}
	wt := d.f.root() + "/.worktrees/.land-2"
	want := []string{
		"git worktree add --detach " + wt + " mergedcommit",
		"git worktree remove --force --force " + wt,
		"git worktree prune",
	}
	if got := d.sh.lines("git worktree"); !slices.Equal(got, want) {
		t.Fatalf("calls %v", got)
	}
}

func TestStage2_removesTheWorktreeWhenReadyFails(t *testing.T) {
	t.Parallel()
	d := newDirect(t, green(t, 1, "b1", "fb"), green(t, 2, "b2", "b1"))
	d.sh.first(shellRule{prefix: "git diff --name-only", out: "apps/backend/a.go\n"},
		shellRule{prefix: "scripts/ci/ready.sh", err: errors.New("exit status 1")})
	d.f.agents(t, "land-stack", "2")
	if got := d.sh.lines("git worktree remove", "git worktree prune"); len(got) != 2 ||
		!strings.Contains(got[0], "--force --force") {
		t.Fatalf("calls %v", got)
	}
}
