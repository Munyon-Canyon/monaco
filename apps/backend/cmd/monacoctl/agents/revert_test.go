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

func TestWatch_ignoresTheCiOkAggregateWhenJudgingALanding(t *testing.T) {
	t.Parallel()
	l := newLanding(t, func(x *Landing) { x.Reran = false })
	l.run("failure", Job{Name: "staging / ios / iOS app build", Conclusion: "failure"},
		Job{Name: "staging / ci-ok", Conclusion: "failure"})
	code, stdout, stderr := l.f.agents(t, "watch", "--once")
	want := "staging run https://gh/run/77 failed only on staging / ios / iOS app build, " +
		"which #1 #2 cannot affect; not reverting\n"
	if code != 0 || stderr != "" || !strings.Contains(stdout, want) || strings.Contains(stdout, "ci-ok") {
		t.Fatalf("%d %q %q", code, stdout, stderr)
	}
	if got := l.sh.lines("git worktree", "git revert", "gt "); len(got) != 0 {
		t.Fatalf("reverted anyway: %v", got)
	}
	if len(l.f.hub.callsContaining("rerun-failed-jobs")) != 0 {
		t.Fatalf("reran: %v", l.f.hub.callsContaining("rerun-failed-jobs"))
	}
	if got := l.landing(t); !got.Checked || got.Reran || got.Revert != 0 {
		t.Fatalf("landing %+v", got)
	}
}

func TestWatch_neverBlamesTheCiOkAggregateAlone(t *testing.T) {
	t.Parallel()
	l := newLanding(t, func(x *Landing) { x.Reran = false })
	l.run("failure", Job{Name: "staging / ci-ok", Conclusion: "failure"})
	_, stdout, _ := l.f.agents(t, "watch", "--once")
	want := "staging run https://gh/run/77 failed only on the ci-ok aggregate " +
		"(a job was cancelled or the run was cut short); not reverting #1 #2\n"
	if !strings.Contains(stdout, want) || len(l.f.hub.callsContaining("rerun-failed-jobs")) != 0 {
		t.Fatalf("%q", stdout)
	}
	if got := l.landing(t); !got.Checked || got.Reran || got.Revert != 0 {
		t.Fatalf("landing %+v", got)
	}
}

func TestWatch_stillRerunsWhenCiOkFailsBesideABackendShard(t *testing.T) {
	t.Parallel()
	l := newLanding(t, func(x *Landing) { x.Reran = false })
	l.f.hub.on("POST /repos/o/r/actions/runs/77/rerun-failed-jobs", "")
	l.run("failure", Job{Name: "staging / Backend tests (a)", Conclusion: "failure"},
		Job{Name: "staging / ci-ok", Conclusion: "failure"})
	_, stdout, _ := l.f.agents(t, "watch", "--once")
	want := "failed on staging / Backend tests (a); reran the failed jobs once before deciding on #1 #2\n"
	if !strings.Contains(stdout, want) || strings.Contains(stdout, "ci-ok") {
		t.Fatalf("%q", stdout)
	}
	if got := l.landing(t); got.Checked || !got.Reran {
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
		{"stage 0 fails on the revert", "revert of #1 #2 failed stage 0 (https://gh/run/77): ", func(l *landingFixture) {
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
			l.sh.before = func(line string) {
				if strings.HasPrefix(line, "git worktree add") {
					writeFile(l.t, l.f.root()+"/.worktrees/revert-2/.build", "in the way")
				}
			}
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
	l := newLanding(t, func(x *Landing) { x.Reran = false })
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

func TestWatch_goesOnToTheRerunWhenThePriorRunWasRedOnOtherJobs(t *testing.T) {
	t.Parallel()
	l := newLanding(t, func(x *Landing) { x.Reran = false })
	l.prior(
		Run{ID: 70, Status: "completed", Conclusion: "failure", HeadSHA: "5555555555555555", CreatedAt: priorAt(l.f)},
	)
	l.f.hub.on(get("/actions/runs/70/jobs?per_page=100"), map[string]any{"jobs": []Job{
		{Name: "staging / Lint", Conclusion: "failure"},
	}})
	l.f.hub.on("POST /repos/o/r/actions/runs/77/rerun-failed-jobs", "")
	l.run("failure", Job{Name: "staging / Backend tests (a)", Conclusion: "failure"})
	_, stdout, _ := l.f.agents(t, "watch", "--once")
	if strings.Contains(stdout, "already red") || !strings.Contains(stdout, "reran the failed jobs once") {
		t.Fatalf("%q", stdout)
	}
}

func TestWatch_reportsWhatItCannotReadAboutThePriorRunAndWhatItCannotRerun(t *testing.T) {
	t.Parallel()
	prior := get("/actions/workflows/staging.yml/runs?branch=fb&status=completed&per_page=10")
	jobs := get("/actions/runs/70/jobs?per_page=100")
	rerun := "POST /repos/o/r/actions/runs/77/rerun-failed-jobs"
	for _, tc := range []struct {
		name, want string
		checked    bool
		setup      func(l *landingFixture)
	}{
		{"the prior run list fails", "watch error: revert #1 #2: GET ", true, func(l *landingFixture) {
			l.f.hub.status[prior] = 500
		}},
		{"the prior run jobs fail", "watch error: revert #1 #2: GET ", true, func(l *landingFixture) {
			l.prior(Run{ID: 70, Status: "completed", Conclusion: "failure", HeadSHA: "5555", CreatedAt: priorAt(l.f)})
			l.f.hub.status[jobs] = 500
		}},
		{"the later runs cannot be listed", "watch error: revert #1 #2: GET ", true, func(l *landingFixture) {
			at := l.f.now.Add(-5 * time.Minute).UTC().Format(time.RFC3339)
			l.f.hub.status[get("/actions/workflows/staging.yml/runs?event=workflow_dispatch&branch=fb&created=>="+
				at+"&per_page=100")] = 500
		}},
		{"the rerun is refused", "watch error: rerun https://gh/run/77: POST ", false, func(l *landingFixture) {
			l.f.hub.on(rerun, "")
			l.f.hub.status[rerun] = 500
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			l := newLanding(t, func(x *Landing) { x.Reran = false })
			l.run("failure", Job{Name: "staging / Backend tests (a)", Conclusion: "failure"})
			l.f.hub.on(jobs, map[string]any{"jobs": []Job{}})
			tc.setup(l)
			_, stdout, _ := l.f.agents(t, "watch", "--once")
			got := l.landing(t)
			if !strings.Contains(stdout, tc.want) || got.Checked != tc.checked || got.Reran {
				t.Fatalf("%q, want %q; %+v", stdout, tc.want, got)
			}
		})
	}
}

func sharedRunLanding(t *testing.T, headSHA string, jobs ...Job) *landingFixture {
	t.Helper()
	l := newLanding(t, func(x *Landing) { x.RunID = 0 })
	l.sh.first(shellRule{prefix: "git merge-base " + lastSHA + " " + headSHA, out: lastSHA + "\n"},
		shellRule{prefix: "git merge-base " + lastSHA + " " + firstSHA, out: "0000\n"})
	at := l.f.now.Add(-5 * time.Minute).UTC().Format(time.RFC3339)
	older := Run{ID: 70, Status: "completed", Conclusion: "success", HeadSHA: firstSHA}
	older.CreatedAt = l.f.now.Add(-4 * time.Minute)
	shared := Run{ID: 77, Status: "completed", Conclusion: "failure", URL: "https://gh/run/77", HeadSHA: headSHA}
	shared.CreatedAt = l.f.now.Add(-3 * time.Minute)
	l.f.hub.on(
		get("/actions/workflows/staging.yml/runs?event=workflow_dispatch&branch=fb&created=>="+at+"&per_page=100"),
		map[string]any{"workflow_runs": []Run{older, shared}},
	)
	l.f.hub.on(get("/actions/runs/77/jobs?per_page=100"), map[string]any{"jobs": jobs})
	return l
}

func TestWatch_doesNotRevertOneLandingOfSeveralCoveredByASharedRun(t *testing.T) {
	t.Parallel()
	l := sharedRunLanding(t, "3333333333333333333333333333333333333333",
		Job{Name: "staging / Backend tests (a)", Conclusion: "failure"})
	l.sh.first(shellRule{prefix: "git merge-base " + lastSHA + " " + firstSHA, out: "0000\n"})
	_, stdout, _ := l.f.agents(t, "watch", "--once")
	want := "staging run https://gh/run/77 covers #1 #2 and other landings; not reverting one of several\n"
	if !strings.Contains(stdout, want) || strings.Contains(stdout, "reverted") {
		t.Fatalf("%q", stdout)
	}
	if got := l.landing(t); !got.Checked || got.Revert != 0 || got.RunID != 77 {
		t.Fatalf("landing %+v", got)
	}
	if len(l.f.hub.callsContaining("/rerun")) != 0 || len(l.sh.lines("git revert")) != 0 {
		t.Fatal("the shared run was rerun or reverted")
	}
}

func TestWatch_stillRevertsWhenTheRunHeadIsTheLanding(t *testing.T) {
	t.Parallel()
	l := sharedRunLanding(t, lastSHA, Job{Name: "staging / Backend tests (a)", Conclusion: "failure"})
	_, stdout, _ := l.f.agents(t, "watch", "--once")
	want := "reverted #1 #2 with #99: staging run https://gh/run/77 failed on staging / Backend tests (a)\n"
	if !strings.Contains(stdout, want) {
		t.Fatalf("%q", stdout)
	}
}

func TestWatch_reportsWhatItCannotReadWhileLookingForASharedRun(t *testing.T) {
	t.Parallel()
	l := sharedRunLanding(t, "3333333333333333333333333333333333333333")
	l.sh.first(shellRule{prefix: "git merge-base", err: errors.New("bad object")})
	_, stdout, _ := l.f.agents(t, "watch", "--once")
	if !strings.Contains(stdout, "watch error: landing "+lastSHA[:12]+": ") || l.landing(t).Checked {
		t.Fatalf("%q", stdout)
	}
}

func (l *landingFixture) realWorktrees() {
	l.sh.before = func(line string) {
		switch {
		case strings.HasPrefix(line, "git worktree add"):
			_ = os.MkdirAll(l.f.root()+"/.worktrees/revert-2", 0o750)
		case strings.HasPrefix(line, "git worktree remove"):
			_ = os.RemoveAll(l.f.root() + "/.worktrees/revert-2")
		}
	}
	l.sh.first(shellRule{prefix: "git branch", out: ""})
}

func (l *landingFixture) count(prefix string) int {
	l.sh.mu.Lock()
	defer l.sh.mu.Unlock()
	n := 0
	for _, c := range l.sh.calls {
		if strings.HasPrefix(c.line, prefix) {
			n++
		}
	}
	return n
}

func TestWatch_doesNotRetryARevertThatFailedStage0(t *testing.T) {
	t.Parallel()
	l := newLanding(t, nil)
	l.realWorktrees()
	l.run("failure", Job{Name: "staging / Backend tests (a)", Conclusion: "failure"})
	l.sh.first(shellRule{prefix: "go run", out: "FAIL: TestX\n", err: errors.New("boom")})
	for range 3 {
		l.f.agents(t, "watch", "--once")
	}
	if n := l.count("go run"); n != 1 {
		t.Fatalf("stage 0 ran %d times, want 1", n)
	}
	got := l.landing(t)
	if !got.Checked || !got.RevertTried || got.RunURL != "https://gh/run/77" {
		t.Fatalf("landing %+v", got)
	}

	again := newLanding(t, func(x *Landing) { x.RevertTried = true })
	again.run("failure", Job{Name: "staging / Backend tests (a)", Conclusion: "failure"})
	_, stdout, _ := again.f.agents(t, "watch", "--once")
	if !strings.Contains(stdout, "revert of #1 #2 not retried: it failed stage 0 at ") ||
		!strings.Contains(stdout, "; root: fix by hand\n") || again.count("git worktree add") != 0 ||
		again.count("go run") != 0 || !again.landing(t).Checked {
		t.Fatalf("%q", stdout)
	}
}

func TestWatch_leavesNoWorktreeAfterAFailedRevert(t *testing.T) {
	t.Parallel()
	l := newLanding(t, nil)
	l.realWorktrees()
	l.run("failure", Job{Name: "staging / Backend tests (a)", Conclusion: "failure"})
	l.sh.first(shellRule{prefix: "go run", out: "FAIL: TestX\n", err: errors.New("boom")})
	_, stdout, _ := l.f.agents(t, "watch", "--once")
	if !strings.Contains(stdout, "revert of #1 #2 failed stage 0 (https://gh/run/77): ") ||
		!strings.Contains(stdout, "FAIL: TestX") {
		t.Fatalf("%q", stdout)
	}
	if _, err := os.Stat(l.f.root() + "/.worktrees/revert-2"); !os.IsNotExist(err) {
		t.Fatalf("worktree left behind: %v", err)
	}
	if l.count("git worktree remove --force ") != 1 || l.count("git branch -D revert-2") != 1 {
		t.Fatalf("calls %+v", l.sh.calls)
	}
}

func TestWatch_doesNotRevertAMobileOnlyLandingForABackendJob(t *testing.T) {
	t.Parallel()
	l := newLanding(t, func(x *Landing) { x.Reran = false })
	l.sh.first(shellRule{prefix: "git diff --name-only", out: "apps/mobile/Monaco/A.swift\ndocs/design/f/notes.md\n"})
	l.run("failure",
		Job{Name: "staging / E2E (e2e 1 of 3)", Conclusion: "failure"},
		Job{Name: "staging / Backend tests (short 3 of 8)", Conclusion: "failure"},
		Job{Name: "staging / Lint (golangci-lint)", Conclusion: "failure"},
		Job{Name: "staging / Lint (nogo, comments, OpenAPI, migrations, vuln)", Conclusion: "failure"},
	)
	code, stdout, stderr := l.f.agents(t, "watch", "--once")
	if code != 0 || stderr != "" || !strings.Contains(stdout, "which #1 #2 cannot affect; not reverting\n") {
		t.Fatalf("%d %q %q", code, stdout, stderr)
	}
	if len(l.f.hub.callsContaining("rerun-failed-jobs")) != 0 || l.count("git worktree add") != 0 {
		t.Fatalf("reran or reverted: %v %+v", l.f.hub.callsContaining("rerun-failed-jobs"), l.sh.calls)
	}
	if got := l.landing(t); !got.Checked || got.Reran || got.Revert != 0 {
		t.Fatalf("landing %+v", got)
	}
}

func TestWatch_stillRevertsAMobileOnlyLandingForTheIOSJob(t *testing.T) {
	t.Parallel()
	for _, job := range []string{
		"staging / ios / iOS app build", "staging / mobile-core / Swift (mobile-core, Linux)", "staging / Journeys",
		"staging / scripts / Go tests (scripts 1 of 2)",
	} {
		l := newLanding(t, nil)
		l.sh.first(
			shellRule{
				prefix: "git diff --name-only",
				out:    "apps/mobile/Monaco/A.swift\npackages/mobile-core/B.swift\n",
			},
		)
		l.run("failure", Job{Name: job, Conclusion: "failure"},
			Job{Name: "staging / E2E (e2e 2 of 3)", Conclusion: "failure"})
		_, stdout, _ := l.f.agents(t, "watch", "--once")
		want := "reverted #1 #2 with #99: staging run https://gh/run/77 failed on " + job + "\n"
		if !strings.Contains(stdout, want) {
			t.Fatalf("%s: %q", job, stdout)
		}
	}
}

func TestWatch_aMobileAndBackendLandingKeepsBackendJobsRelated(t *testing.T) {
	t.Parallel()
	l := newLanding(t, nil)
	l.sh.first(shellRule{prefix: "git diff --name-only", out: "apps/mobile/Monaco/A.swift\napps/backend/a.go\n"})
	l.run("failure", Job{Name: "staging / E2E (e2e 1 of 3)", Conclusion: "failure"})
	_, stdout, _ := l.f.agents(t, "watch", "--once")
	if !strings.Contains(
		stdout,
		"reverted #1 #2 with #99: staging run https://gh/run/77 failed on staging / E2E (e2e 1 of 3)\n",
	) {
		t.Fatalf("%q", stdout)
	}
}

func TestWatch_doesNotRevertADocsOnlyLandingForACodeJob(t *testing.T) {
	t.Parallel()
	for _, job := range []string{"staging / Backend tests (a)", "staging / ios", "staging / e2e"} {
		l := newLanding(t, nil)
		l.sh.first(shellRule{prefix: "git diff --name-only", out: "docs/a/b.png\ndocs/design/f/notes.md\nREADME.md\n"})
		l.run("failure", Job{Name: job, Conclusion: "failure"})
		_, stdout, _ := l.f.agents(t, "watch", "--once")
		if !strings.Contains(stdout, "which #1 #2 cannot affect; not reverting") || l.count("git worktree add") != 0 {
			t.Fatalf("%s: %q", job, stdout)
		}
	}
	l := newLanding(t, nil)
	l.sh.first(shellRule{prefix: "git diff --name-only", out: "docs/a.md\n"})
	l.run("failure", Job{Name: "staging / mkdocs build --strict", Conclusion: "failure"})
	_, stdout, _ := l.f.agents(t, "watch", "--once")
	if !strings.Contains(stdout, "reverted #1 #2 with #99") {
		t.Fatalf("%q", stdout)
	}
	code := newLanding(t, nil)
	code.sh.first(shellRule{prefix: "git diff --name-only", out: "docs/a.md\napps/backend/a.go\n"})
	code.run("failure", Job{Name: "staging / Backend tests (a)", Conclusion: "failure"})
	_, stdout, _ = code.f.agents(t, "watch", "--once")
	if !strings.Contains(stdout, "reverted #1 #2 with #99") {
		t.Fatalf("%q", stdout)
	}
}

func TestWatch_removesTheRevertWorktreeOnceTheRevertPRLanded(t *testing.T) {
	t.Parallel()
	l := newLanding(t, nil)
	l.realWorktrees()
	l.f.hub.on(get("/pulls/99"), PR{Number: 99, State: "MERGED"})
	l.run("failure", Job{Name: "staging / Backend tests (a)", Conclusion: "failure"})
	_, stdout, _ := l.f.agents(t, "watch", "--once")
	if !strings.Contains(stdout, "reverted #1 #2 with #99") || strings.Contains(stdout, "stays until") {
		t.Fatalf("%q", stdout)
	}
	if _, err := os.Stat(l.f.root() + "/.worktrees/revert-2"); !os.IsNotExist(err) {
		t.Fatalf("worktree left behind: %v", err)
	}
	if l.count("git branch -D revert-2") != 1 {
		t.Fatalf("calls %+v", l.sh.calls)
	}
}

func TestWatch_buildsNoRevertWhenItCannotRecordTheAttempt(t *testing.T) {
	t.Parallel()
	l := newLanding(t, nil)
	l.run("failure", Job{Name: "staging / Backend tests (a)", Conclusion: "failure"})
	if err := os.MkdirAll(l.f.Env(t).recordPath(40)+".lock", 0o750); err != nil {
		t.Fatal(err)
	}
	_, stdout, _ := l.f.agents(t, "watch", "--once")
	if !strings.Contains(stdout, "record the revert attempt: ") || l.count("git worktree add") != 0 {
		t.Fatalf("%q", stdout)
	}
}

func laterRuns(l *landingFixture, runs ...Run) {
	at := l.f.now.Add(-5 * time.Minute).UTC().Format(time.RFC3339)
	l.f.hub.on(
		get("/actions/workflows/staging.yml/runs?event=workflow_dispatch&branch=fb&created=>="+at+"&per_page=100"),
		map[string]any{"workflow_runs": runs},
	)
}

func TestWatch_aLaterGreenRunContainingTheLandingSettlesIt(t *testing.T) {
	t.Parallel()
	l := newLanding(t, func(x *Landing) { x.Reran = false })
	l.run("failure", Job{Name: "staging / Backend tests (a)", Conclusion: "failure"})
	laterRuns(l, Run{
		ID: 80, Status: "completed", Conclusion: "success", URL: "https://gh/run/80", HeadSHA: baseSHA,
		CreatedAt: l.f.now.Add(-2 * time.Minute),
	})
	l.sh.first(shellRule{prefix: "git merge-base " + lastSHA + " " + baseSHA, out: lastSHA + "\n"})
	_, stdout, _ := l.f.agents(t, "watch", "--once")
	want := "staging run https://gh/run/77 failed, but https://gh/run/80 at " + baseSHA[:12] +
		" contains #1 #2 and passed; not reverting\n"
	if !strings.Contains(stdout, want) || strings.Contains(stdout, "reverted") || strings.Contains(stdout, "reran") {
		t.Fatalf("%q", stdout)
	}
	if got := l.landing(t); !got.Checked || got.Revert != 0 || got.Reran {
		t.Fatalf("landing %+v", got)
	}
	if len(l.f.hub.callsContaining("/rerun")) != 0 || len(l.sh.lines("git revert", "gt submit")) != 0 {
		t.Fatal("the landing was rerun or reverted")
	}
}

func TestWatch_aLaterGreenRunNotContainingTheLandingDoesNot(t *testing.T) {
	t.Parallel()
	l := newLanding(t, func(x *Landing) { x.Reran = false })
	l.run("failure", Job{Name: "staging / Backend tests (a)", Conclusion: "failure"})
	laterRuns(l,
		Run{
			ID: 80, Status: "completed", Conclusion: "success", URL: "https://gh/run/80", HeadSHA: baseSHA,
			CreatedAt: l.f.now.Add(-2 * time.Minute),
		},
		Run{
			ID: 81, Status: "completed", Conclusion: "failure", URL: "https://gh/run/81", HeadSHA: firstSHA,
			CreatedAt: l.f.now.Add(-time.Minute),
		},
	)
	l.f.hub.on("POST /repos/o/r/actions/runs/77/rerun-failed-jobs", "")
	l.sh.first(shellRule{prefix: "git merge-base " + lastSHA + " " + baseSHA, out: "0000\n"})
	_, stdout, _ := l.f.agents(t, "watch", "--once")
	if strings.Contains(stdout, "not reverting") || !strings.Contains(stdout, "reran the failed jobs once") {
		t.Fatalf("%q", stdout)
	}
	if got := l.landing(t); got.Checked || !got.Reran {
		t.Fatalf("landing %+v", got)
	}
	if lines := l.sh.lines("git merge-base " + lastSHA + " " + firstSHA); len(lines) != 0 {
		t.Fatalf("looked at a failed run: %v", lines)
	}
	_, stdout, _ = l.f.agents(t, "watch", "--once")
	if !strings.Contains(stdout, "reverted #1 #2 with #99") {
		t.Fatalf("second failure: %q", stdout)
	}
}
