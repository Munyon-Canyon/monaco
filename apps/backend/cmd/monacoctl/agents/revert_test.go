package agents

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

type landingFixture struct {
	t  *testing.T
	f  *fixture
	sh *fakeShell
	s  *stackGH
}

func newLanding(t *testing.T, mutate func(*Landing)) *landingFixture {
	t.Helper()
	f := newFixture(t)
	s := newStackGH(t, f, green(t, 99, "revert-2", "fb"))
	s.prs[99].Title = "Revert"
	sh := &fakeShell{next: s.run, rules: rules([]shellRule{
		{prefix: "git rev-list --first-parent -n 2 " + lastSHA, out: lastSHA + "\n" + firstSHA + "\n"},
		{prefix: "git diff --name-only " + firstSHA + "^ " + lastSHA, out: "apps/backend/a.go\n"},
		{prefix: "git worktree", out: ""},
		{prefix: "git revert", out: ""},
		{prefix: "cp ", out: ""},
		{prefix: "scripts/pr-body.sh", out: ""},
		{prefix: "gh pr view --json number", out: `{"number":99}`},
	}...)}
	f.run = sh.run
	l := Landing{Top: 2, PRs: []int{1, 2}, SHA: lastSHA, At: f.now.Add(-5 * time.Minute), RunID: 77, Reran: true}
	if mutate != nil {
		mutate(&l)
	}
	f.owner(t, Record{Ticket: 40, State: Exited, Worktree: f.dir, Landed: []Landing{l}})
	f.noFailures()
	f.hub.on(get("/pulls/2"), PR{Number: 2, Title: "Add widget"})
	lf := &landingFixture{t: t, f: f, sh: sh, s: s}
	lf.prior(
		Run{ID: 70, Status: "completed", Conclusion: "success", HeadSHA: "5555555555555555", CreatedAt: priorAt(f)},
	)
	return lf
}

func priorAt(f *fixture) time.Time { return f.now.Add(-time.Hour) }

func (l *landingFixture) prior(runs ...Run) {
	l.f.hub.on(get("/actions/workflows/staging.yml/runs?branch=fb&status=completed&per_page=10"),
		map[string]any{"workflow_runs": runs})
}

func (l *landingFixture) run(conclusion string, jobs ...Job) {
	l.f.hub.on(get("/actions/runs/77"), Run{
		ID: 77, Status: "completed", Conclusion: conclusion, URL: "https://gh/run/77", HeadSHA: lastSHA,
		CreatedAt: l.f.now.Add(-10 * time.Minute),
	})
	l.f.hub.on(get("/actions/runs/77/jobs?per_page=100"), map[string]any{"jobs": jobs})
}

func (l *landingFixture) landing(t *testing.T) Landing {
	t.Helper()
	r := l.f.owned(t)
	if len(r.Landed) != 1 {
		t.Fatalf("landed %+v", r.Landed)
	}
	return r.Landed[0]
}

func TestWatch_revertsALandingWhoseStagingRunFailedOnARelatedJob(t *testing.T) {
	t.Parallel()
	l := newLanding(t, nil)
	l.run("failure", Job{Name: "staging / Backend tests (a)", Conclusion: "failure"},
		Job{Name: "staging / ios", Conclusion: "failure"}, Job{Name: "staging / Lint", Conclusion: "success"})
	code, stdout, stderr := l.f.agents(t, "watch", "--once")
	dir := l.f.root() + "/.worktrees/revert-2"
	if code != 0 || stderr != "" ||
		!strings.Contains(
			stdout,
			"reverted #1 #2 with #99: staging run https://gh/run/77 failed on staging / Backend tests (a)\n",
		) ||
		!strings.Contains(stdout, "revert worktree "+dir+" stays until #99 lands; agents done is not involved\n") ||
		!strings.Contains(stdout, "queued #99\n") {
		t.Fatalf("%d %q %q", code, stdout, stderr)
	}
	want := []string{
		"git fetch --quiet origin fb",
		"git rev-list --first-parent -n 2 " + lastSHA,
		"git diff --name-only " + firstSHA + "^ " + lastSHA,
		"git fetch --quiet origin fb",
		"git worktree add -b revert-2 " + dir + " origin/fb",
		"git rev-list --first-parent -n 2 " + lastSHA,
		"git revert --no-edit " + lastSHA + " " + firstSHA,
		"gt track revert-2 --parent fb --no-interactive",
		"cp -R " + l.f.root() + "/.bin " + dir + "/.bin",
		"go run ./cmd/monacoctl agents check",
		"gt submit --stack --no-interactive --draft",
		"gh pr view --json number",
		"scripts/pr-body.sh 99 Revert Add widget after staging run failed on staging / Backend tests (a) " +
			dir + "/.build/revert-body.md",
	}
	got := l.sh.lines("git ", "gt ", "go ", "cp ", "scripts/", "gh pr view")
	if len(got) < len(want) || !slices.Equal(got[:len(want)], want) {
		t.Fatalf("commands:\n%s", strings.Join(got, "\n"))
	}
	if d := l.sh.dirOf("go run"); d != dir+"/apps/backend" {
		t.Fatalf("check ran in %s", d)
	}
	body, err := os.ReadFile(filepath.Join(dir, ".build", "revert-body.md"))
	if err != nil {
		t.Fatal(err)
	}
	for _, part := range []string{
		"Reverts #1 #2 (" + lastSHA[:12] + "): staging run https://gh/run/77 failed on staging / Backend tests (a) " +
			"after they landed on stage 0.",
		"Part of #40.", "git revert of the squash commits of #1 #2", "Reland after fixing staging / Backend tests (a).",
	} {
		if !strings.Contains(string(body), part) {
			t.Fatalf("body lacks %q:\n%s", part, body)
		}
	}
	if got := l.landing(t); !got.Checked || got.Revert != 99 {
		t.Fatalf("landing %+v", got)
	}
	if r := l.f.owned(t); len(r.Queued) != 1 || r.Queued[0].Top != 99 {
		t.Fatalf("the revert was not landed: %+v", r.Queued)
	}
}

func TestWatch_ignoresAStagingRunFailedOnlyOnMobileJobsForABackendLanding(t *testing.T) {
	t.Parallel()
	l := newLanding(t, nil)
	l.run("failure", Job{Name: "staging / ios / build", Conclusion: "failure"},
		Job{Name: "staging / mobile-core", Conclusion: "failure"})
	code, stdout, stderr := l.f.agents(t, "watch", "--once")
	want := "staging run https://gh/run/77 failed only on staging / ios / build, staging / mobile-core, " +
		"which #1 #2 cannot affect; not reverting\n"
	if code != 0 || stderr != "" || !strings.Contains(stdout, want) {
		t.Fatalf("%d %q %q", code, stdout, stderr)
	}
	if got := l.sh.lines("git worktree", "git revert", "gt "); len(got) != 0 {
		t.Fatalf("reverted anyway: %v", got)
	}
	if got := l.landing(t); !got.Checked || got.Revert != 0 {
		t.Fatalf("landing %+v", got)
	}
}

func TestWatch_revertsAMobileFailureWhenTheLandingTouchedMobile(t *testing.T) {
	t.Parallel()
	l := newLanding(t, nil)
	l.sh.first(shellRule{prefix: "git diff --name-only", out: "apps/mobile/Monaco/A.swift\n"})
	l.run("failure", Job{Name: "staging / ios", Conclusion: "failure"})
	_, stdout, _ := l.f.agents(t, "watch", "--once")
	if !strings.Contains(stdout, "reverted #1 #2 with #99: staging run https://gh/run/77 failed on staging / ios\n") {
		t.Fatalf("%q", stdout)
	}
}

func TestWatch_reportsAPassedStagingRunOnce(t *testing.T) {
	t.Parallel()
	l := newLanding(t, nil)
	l.run("success")
	_, stdout, _ := l.f.agents(t, "watch", "--once")
	if !strings.Contains(stdout, "staging run for "+lastSHA[:12]+" passed (#1 #2)\n") {
		t.Fatalf("%q", stdout)
	}
	_, stdout, _ = l.f.agents(t, "watch", "--once")
	if strings.Contains(stdout, "passed") || len(l.f.hub.callsContaining("/actions/runs/77")) != 1 {
		t.Fatalf("second run %q after %v", stdout, l.f.hub.callsContaining("/actions/runs/77"))
	}
	if !l.landing(t).Checked {
		t.Fatal("the landing stayed unchecked")
	}
}

func TestWatch_waitsForARunThatHasNotConcluded(t *testing.T) {
	t.Parallel()
	l := newLanding(t, nil)
	l.f.hub.on(get("/actions/runs/77"), Run{ID: 77, Status: "in_progress", HeadSHA: lastSHA})
	_, stdout, _ := l.f.agents(t, "watch", "--once")
	if strings.Contains(stdout, "staging run") || l.landing(t).Checked {
		t.Fatalf("%q", stdout)
	}
}

func TestWatch_findsTheRunByTheLandingSHA(t *testing.T) {
	t.Parallel()
	l := newLanding(t, func(x *Landing) { x.RunID = 0 })
	at := l.f.now.Add(-5 * time.Minute).UTC().Format(time.RFC3339)
	l.f.hub.on(
		get("/actions/workflows/staging.yml/runs?event=workflow_dispatch&branch=fb&created=>="+at+"&per_page=100"),
		map[string]any{"workflow_runs": []Run{
			{ID: 70, Status: "completed", Conclusion: "failure", HeadSHA: firstSHA},
			{ID: 77, Status: "completed", Conclusion: "success", URL: "https://gh/run/77", HeadSHA: lastSHA},
		}},
	)
	_, stdout, _ := l.f.agents(t, "watch", "--once")
	if !strings.Contains(stdout, "passed (#1 #2)") {
		t.Fatalf("%q", stdout)
	}
	if got := l.landing(t); got.RunID != 77 || got.RunURL != "https://gh/run/77" {
		t.Fatalf("landing %+v", got)
	}
}

func TestWatch_redispatchesWhenNoRunAppearsInTenMinutes(t *testing.T) {
	t.Parallel()
	l := newLanding(t, func(x *Landing) { x.RunID = 0; x.At = x.At.Add(-6 * time.Minute) })
	route := "POST /repos/o/r/actions/workflows/staging.yml/dispatches"
	l.f.hub.on(route, "")
	_, stdout, _ := l.f.agents(t, "watch", "--once")
	if !strings.Contains(stdout, "no staging run for "+lastSHA[:12]+"; dispatched one\n") {
		t.Fatalf("%q", stdout)
	}
	if got := l.f.hub.body(route); got != `{"inputs":{"sha":"`+lastSHA+`"},"ref":"fb"}` {
		t.Fatalf("dispatch body %q", got)
	}
	_, stdout, _ = l.f.agents(t, "watch", "--once")
	if strings.Contains(stdout, "dispatched one") || len(l.f.hub.callsContaining("staging.yml/dispatches")) != 1 {
		t.Fatalf("second run %q", stdout)
	}
	if got := l.landing(t); got.Checked || !got.Redispatched {
		t.Fatalf("landing %+v", got)
	}
}

func TestWatch_doesNotDispatchBeforeTenMinutesAndGivesUpAfterThirty(t *testing.T) {
	t.Parallel()
	l := newLanding(t, func(x *Landing) { x.RunID = 0 })
	_, stdout, _ := l.f.agents(t, "watch", "--once")
	if strings.Contains(stdout, "no staging run") || len(l.f.hub.callsContaining("dispatches")) != 0 {
		t.Fatalf("%q", stdout)
	}
	l.f.now = l.f.now.Add(26 * time.Minute)
	_, stdout, _ = l.f.agents(t, "watch", "--once")
	want := "no staging run for " + lastSHA[:12] + " after 30m; dispatch it: gh workflow run staging.yml -f sha=" + lastSHA
	if !strings.Contains(stdout, want) || !l.landing(t).Checked {
		t.Fatalf("%q", stdout)
	}
}

func TestWatch_reportsWhatItCannotReadAboutAStagingRun(t *testing.T) {
	t.Parallel()
	boom := errors.New("boom")
	for _, tc := range []struct {
		name, want   string
		undiscovered bool
		setup        func(l *landingFixture)
	}{
		{"the run cannot be read", "watch error: landing 222222222222: GET /repos/o/r/actions/runs/77", false, func(l *landingFixture) {
			l.f.hub.on(get("/actions/runs/77"), "{}")
			l.f.hub.status[get("/actions/runs/77")] = 500
		}},
		{"the runs cannot be listed", "watch error: landing 222222222222: GET", true, func(l *landingFixture) {
			route := get("/actions/workflows/staging.yml/runs?event=workflow_dispatch&branch=fb&created=>=" +
				l.f.now.Add(-5*time.Minute).UTC().Format(time.RFC3339) + "&per_page=100")
			l.f.hub.on(route, "{}")
			l.f.hub.status[route] = 500
		}},
		{"the jobs cannot be listed", "watch error: revert #1 #2: GET /repos/o/r/actions/runs/77/jobs", false, func(l *landingFixture) {
			l.f.hub.on(get("/actions/runs/77"), Run{ID: 77, Status: "completed", Conclusion: "failure", HeadSHA: lastSHA})
			l.f.hub.on(get("/actions/runs/77/jobs?per_page=100"), "{}")
			l.f.hub.status[get("/actions/runs/77/jobs?per_page=100")] = 500
		}},
		{"the landing's files cannot be read", "watch error: revert #1 #2: boom", false, func(l *landingFixture) {
			l.run("failure", Job{Name: "staging / Backend tests", Conclusion: "failure"})
			l.sh.first(shellRule{prefix: "git fetch --quiet origin fb", err: boom})
		}},
		{"the record cannot be written", "watch error: landing 222222222222: write owner record", false, func(l *landingFixture) {
			l.run("success")
			freeze(l.t, l.f.Env(l.t).recordPath(40))
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			l := newLanding(t, func(x *Landing) {
				if tc.undiscovered {
					x.RunID = 0
				}
			})
			tc.setup(l)
			_, stdout, _ := l.f.agents(t, "watch", "--once")
			if !strings.Contains(stdout, tc.want) {
				t.Fatalf("%q, want %q", stdout, tc.want)
			}
		})
	}
}

func TestWatch_describesARunThatEndedWithoutAFailureItCanBlame(t *testing.T) {
	t.Parallel()
	l := newLanding(t, nil)
	l.run("skipped")
	_, stdout, _ := l.f.agents(t, "watch", "--once")
	if !strings.Contains(stdout, "staging run https://gh/run/77 ended skipped; not reverting #1 #2\n") {
		t.Fatalf("%q", stdout)
	}
	l = newLanding(t, nil)
	l.run("cancelled", Job{Name: "staging / Lint", Conclusion: "success"})
	_, stdout, _ = l.f.agents(t, "watch", "--once")
	if !strings.Contains(
		stdout,
		"staging run https://gh/run/77 ended cancelled with no failed job; not reverting #1 #2\n",
	) ||
		!l.landing(t).Checked {
		t.Fatalf("%q", stdout)
	}
}

func TestWatch_printsTheDispatchErrorAndRetriesWhenNoRunAppears(t *testing.T) {
	t.Parallel()
	l := newLanding(t, func(x *Landing) { x.RunID = 0; x.At = x.At.Add(-6 * time.Minute) })
	route := "POST /repos/o/r/actions/workflows/staging.yml/dispatches"
	l.f.hub.on(route, "")
	l.f.hub.status[route] = 500
	_, stdout, _ := l.f.agents(t, "watch", "--once")
	if !strings.Contains(stdout, "watch error: dispatch staging run for "+lastSHA[:12]+": POST ") ||
		l.landing(t).Redispatched {
		t.Fatalf("%q", stdout)
	}
}

func TestWatch_leavesTheRevertWorktreeAndSaysWhyWhenARevertStepFails(t *testing.T) {
	t.Parallel()
	boom := errors.New("boom")
	for _, tc := range []struct {
		name, want string
		setup      func(l *landingFixture)
	}{
		{"git cannot add the worktree", "watch error: revert #1 #2: revert setup in ", func(l *landingFixture) {
			l.sh.first(shellRule{prefix: "git worktree add", err: boom})
		}},
		{"git cannot revert", "revert setup in ", func(l *landingFixture) {
			l.sh.first(shellRule{prefix: "git revert", err: boom})
		}},
		{"gt cannot track the branch", "revert setup in ", func(l *landingFixture) {
			l.sh.first(shellRule{prefix: "gt track", err: boom})
		}},
		{"stage 0 fails on the revert", "revert of #1 #2 failed stage 0 in ", func(l *landingFixture) {
			l.sh.first(shellRule{prefix: "go run", out: "FAIL: TestX\n", err: boom})
		}},
		{"gt cannot submit", "open the revert PR from ", func(l *landingFixture) {
			l.sh.first(shellRule{prefix: "gt submit", err: boom})
		}},
		{"the PR number is not JSON", "no revert PR number in \"nope\"", func(l *landingFixture) {
			l.sh.first(shellRule{prefix: "gh pr view", out: "nope"})
		}},
		{"the landed PR cannot be read", "read #2", func(l *landingFixture) {
			l.f.hub.status[get("/pulls/2")] = 500
		}},
		{"the body cannot be written", "write revert body", func(l *landingFixture) {
			writeFile(l.t, l.f.root()+"/.worktrees/revert-2/.build", "in the way")
		}},
		{"pr-body.sh refuses", "pr-body in ", func(l *landingFixture) {
			l.sh.first(shellRule{prefix: "scripts/pr-body.sh", err: boom})
		}},
		{"the revert PR cannot be landed", "watch error: land revert #99: ", func(l *landingFixture) {
			delete(l.s.prs, 99)
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			l := newLanding(t, nil)
			l.run("failure", Job{Name: "staging / Backend tests (a)", Conclusion: "failure"})
			tc.setup(l)
			_, stdout, _ := l.f.agents(t, "watch", "--once")
			if !strings.Contains(stdout, tc.want) || !l.landing(t).Checked {
				t.Fatalf("%q, want %q", stdout, tc.want)
			}
		})
	}
}

func TestWatch_remembersARunItFoundBeforeItFinishes(t *testing.T) {
	t.Parallel()
	l := newLanding(t, func(x *Landing) { x.RunID = 0 })
	at := l.f.now.Add(-5 * time.Minute).UTC().Format(time.RFC3339)
	l.f.hub.on(
		get("/actions/workflows/staging.yml/runs?event=workflow_dispatch&branch=fb&created=>="+at+"&per_page=100"),
		map[string]any{"workflow_runs": []Run{
			{ID: 77, Status: "in_progress", URL: "https://gh/run/77", HeadSHA: lastSHA},
		}},
	)
	_, stdout, _ := l.f.agents(t, "watch", "--once")
	if got := l.landing(t); strings.Contains(stdout, "staging run") || got.Checked || got.RunID != 77 {
		t.Fatalf("%q %+v", stdout, got)
	}
}

func TestWatch_doesNotRevertAJobThatWasAlreadyRedBeforeTheLanding(t *testing.T) {
	t.Parallel()
	l := newLanding(t, nil)
	l.prior(
		Run{ID: 70, Status: "completed", Conclusion: "failure", HeadSHA: "5555555555555555", CreatedAt: priorAt(l.f)},
		Run{
			ID:         71,
			Status:     "completed",
			Conclusion: "failure",
			HeadSHA:    lastSHA,
			CreatedAt:  priorAt(l.f).Add(-time.Minute),
		},
	)
	l.f.hub.on(get("/actions/runs/70/jobs?per_page=100"), map[string]any{"jobs": []Job{
		{Name: "staging / Mobile integration", Conclusion: "failure"}, {Name: "staging / Lint", Conclusion: "success"},
	}})
	l.run("failure", Job{Name: "staging / Mobile integration", Conclusion: "failure"})
	_, stdout, _ := l.f.agents(t, "watch", "--once")
	want := "staging run https://gh/run/77 failed on staging / Mobile integration, already red on 55555555 " +
		"before #1 #2 landed; not reverting for them\n"
	if !strings.Contains(stdout, want) || strings.Contains(stdout, "reverted") || strings.Contains(stdout, "reran") {
		t.Fatalf("%q", stdout)
	}
	if got := l.landing(t); !got.Checked || got.Revert != 0 || got.Reran {
		t.Fatalf("landing %+v", got)
	}
	if len(l.sh.lines("git revert", "gt submit")) != 0 {
		t.Fatalf("opened a revert: %v", l.sh.lines("git revert", "gt submit"))
	}
}

func TestWatch_rerunsFailedJobsOnceBeforeReverting(t *testing.T) {
	t.Parallel()
	l := newLanding(t, func(x *Landing) { x.Reran = false })
	route := "POST /repos/o/r/actions/runs/77/rerun-failed-jobs"
	l.f.hub.on(route, "")
	l.run("failure", Job{Name: "staging / Backend tests (a)", Conclusion: "failure"})
	_, stdout, _ := l.f.agents(t, "watch", "--once")
	want := "staging run https://gh/run/77 failed on staging / Backend tests (a); " +
		"reran the failed jobs once before deciding on #1 #2\n"
	if !strings.Contains(stdout, want) || strings.Contains(stdout, "reverted") {
		t.Fatalf("first failure: %q", stdout)
	}
	if got := l.landing(t); got.Checked || !got.Reran || got.Revert != 0 {
		t.Fatalf("landing %+v", got)
	}
	if len(l.f.hub.callsContaining("rerun-failed-jobs")) != 1 {
		t.Fatalf("calls %v", l.f.hub.callsContaining("rerun-failed-jobs"))
	}
	_, stdout, _ = l.f.agents(t, "watch", "--once")
	if !strings.Contains(stdout, "reverted #1 #2 with #99: staging run https://gh/run/77 failed on "+
		"staging / Backend tests (a)\n") || len(l.f.hub.callsContaining("rerun-failed-jobs")) != 1 {
		t.Fatalf("second failure: %q", stdout)
	}
	if got := l.landing(t); !got.Checked || got.Revert != 99 {
		t.Fatalf("landing %+v", got)
	}
}
