package agents

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func runsRoute(sha string) string {
	return fmt.Sprintf("GET /repos/%s/actions/runs?head_sha=%s&per_page=100&page=1", testRepo, sha)
}

func setRuns(f *fixture, sha string, runs ...Run) {
	b, _ := json.Marshal(map[string]any{"workflow_runs": runs})
	f.hub.on(runsRoute(sha), string(b))
}

func rerunRoute(id int64) string {
	return fmt.Sprintf("POST /repos/%s/actions/runs/%d/rerun", testRepo, id)
}

func rerunsInto(f *fixture, sha string, id int64, next ...Run) {
	f.hub.on(rerunRoute(id), "{}")
	var once sync.Once
	prev := f.hub.hook
	f.hub.hook = func(method, path, body string, status int) {
		prev(method, path, body, status)
		if method+" "+path == rerunRoute(id) {
			once.Do(func() { setRuns(f, sha, next...) })
		}
	}
}

func TestLandStack_rerunsALeftoverCancelledRunBeforeAnyLabel(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	newStackGH(t, f, green(t, 1, "b1", "fb"), green(t, 2, "b2", "b1"))
	f.owner(t, Record{Ticket: 40, Worktree: "/w/40", State: Done})
	setRuns(f, "b2-oid",
		Run{ID: 7, WorkflowID: 1, Name: "ci", Status: "completed", Conclusion: "cancelled", Attempt: 1},
		Run{ID: 8, WorkflowID: 2, Name: "pr-format", Status: "completed", Conclusion: "success", Attempt: 1})
	rerunsInto(f, "b2-oid", 7,
		Run{ID: 7, WorkflowID: 1, Name: "ci", Status: "in_progress", Attempt: 2},
		Run{ID: 8, WorkflowID: 2, Name: "pr-format", Status: "completed", Conclusion: "success", Attempt: 1})
	prev := f.hub.hook
	var polls atomic.Int32
	f.hub.hook = func(method, path, body string, status int) {
		prev(method, path, body, status)
		if method+" "+path == runsRoute("b2-oid") && len(f.hub.callsContaining(rerunRoute(7))) > 0 &&
			polls.Add(1) == 2 {
			setRuns(f, "b2-oid",
				Run{ID: 7, WorkflowID: 1, Name: "ci", Status: "completed", Conclusion: "success", Attempt: 2})
		}
	}
	code, stdout, stderr := f.agents(t, "land-stack", "2")
	if code != 0 || !strings.HasPrefix(stdout, "#2 b2-oid: reran \"ci\" (run 7, was cancelled)\nqueued #1 #2\n") {
		t.Fatalf("%d %q %q", code, stdout, stderr)
	}
	calls := f.hub.callsContaining("")
	rerun := slices.Index(calls, rerunRoute(7))
	label := slices.Index(calls, "POST /repos/o/r/issues/1/labels")
	if rerun < 0 || label < rerun {
		t.Fatalf("rerun at %d, first label at %d: %v", rerun, label, calls)
	}
	if len(f.waited) == 0 {
		t.Fatal("did not wait for the rerun to finish")
	}
}

func TestLandStack_refusesWhenARerunFailsAgainAndLabelsNothing(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	newStackGH(t, f, green(t, 1, "b1", "fb"), green(t, 2, "b2", "b1"))
	f.owner(t, Record{Ticket: 40, Worktree: "/w/40", State: Done})
	setRuns(f, "b2-oid", Run{ID: 7, WorkflowID: 1, Name: "ci", Status: "completed", Conclusion: "failure", Attempt: 1})
	rerunsInto(f, "b2-oid", 7,
		Run{
			ID: 7, WorkflowID: 1, Name: "ci", Status: "completed", Conclusion: "failure", Attempt: 2,
			URL: "https://x/runs/7",
		})
	code, stdout, stderr := f.agents(t, "land-stack", "2")
	if code == 0 || !strings.Contains(stderr, `run "ci" ended failure again`) ||
		!strings.Contains(stderr, "https://x/runs/7") {
		t.Fatalf("%d %q %q", code, stdout, stderr)
	}
	if got := f.hub.callsContaining("/labels"); len(got) != 0 {
		t.Fatalf("labelled %v", got)
	}
	if f.owned(t).Queued != nil {
		t.Fatal("marked queued")
	}
}

func TestLandStack_reportsWhichPRsGraphiteHeldBack(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	newStackGH(t, f, green(t, 1, "b1", "fb"), green(t, 2, "b2", "b1"))
	f.owner(t, Record{Ticket: 40, Worktree: "/w/40", State: Done})
	draft := PR{Title: "[Graphite MQ] Draft PR GROUP:x (PRs 1)", Head: Ref{Ref: "gtmq_x"}}
	odd := PR{Title: "no pr list", Head: Ref{Ref: "gtmq_y"}}
	f.hub.on(list("/pulls?state=open"), []PR{{Title: "(PRs 1, 2)", Head: Ref{Ref: "other"}}, odd, draft})
	setRuns(f, "b2-oid", Run{ID: 7, Status: "completed", Conclusion: "success", Attempt: 1})
	_, stdout, stderr := f.agents(t, "land-stack", "2")
	want := "Graphite queued only #1; held back: #2 (no cancelled or failed run on its head; Graphite gave no reason)\n"
	if !strings.Contains(stdout, want) {
		t.Fatalf("%q %q", stdout, stderr)
	}
}

func TestLandStack_saysSoWhenNoGraphiteDraftAppears(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	newStackGH(t, f, green(t, 1, "b1", "fb"))
	f.owner(t, Record{Ticket: 40, Worktree: "/w/40", State: Done})
	f.hub.on(list("/pulls?state=open"), "[]")
	_, stdout, stderr := f.agents(t, "land-stack", "1")
	if !strings.Contains(stdout, "no Graphite draft holds #1 after 3m0s") {
		t.Fatalf("%q %q", stdout, stderr)
	}
}

func TestLandStack_rerunFallsBackToRerunningOnlyTheFailedJobs(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	newStackGH(t, f, green(t, 1, "b1", "fb"))
	f.owner(t, Record{Ticket: 40, Worktree: "/w/40", State: Done})
	setRuns(f, "b1-oid", Run{ID: 7, WorkflowID: 1, Name: "ci", Status: "completed", Conclusion: "failure", Attempt: 1})
	f.hub.status[rerunRoute(7)] = 403
	f.hub.on("POST /repos/o/r/actions/runs/7/rerun-failed-jobs", "{}")
	prev := f.hub.hook
	f.hub.hook = func(method, path, body string, status int) {
		prev(method, path, body, status)
		if path == "/repos/o/r/actions/runs/7/rerun-failed-jobs" {
			setRuns(f, "b1-oid",
				Run{ID: 7, WorkflowID: 1, Name: "ci", Status: "completed", Conclusion: "success", Attempt: 2})
		}
	}
	code, stdout, stderr := f.agents(t, "land-stack", "1")
	if code != 0 || !strings.Contains(stdout, `reran "ci" (run 7, was failure)`) {
		t.Fatalf("%d %q %q", code, stdout, stderr)
	}
}

func failRoute(f *fixture, route string) {
	f.hub.mu.Lock()
	defer f.hub.mu.Unlock()
	f.hub.status[route] = 500
}

func TestLandStack_runFailures(t *testing.T) {
	t.Parallel()
	pending := Run{ID: 7, WorkflowID: 1, Name: "ci", Status: "in_progress", Attempt: 1}
	broken := Run{ID: 7, WorkflowID: 1, Name: "ci", Status: "completed", Conclusion: "failure", Attempt: 1}
	for _, tc := range []struct {
		name  string
		setup func(f *fixture)
		want  string
	}{
		{"listing runs fails", func(f *fixture) { failRoute(f, runsRoute("b1-oid")) }, "500"},
		{"both reruns fail", func(f *fixture) {
			setRuns(f, "b1-oid", broken)
			failRoute(f, rerunRoute(7))
			failRoute(f, rerunRoute(7)+"-failed-jobs")
		}, "the stack is not marked queued"},
		{"a run never finishes", func(f *fixture) { setRuns(f, "b1-oid", pending) }, "still not clean after 30m0s"},
		{"a rerun never starts", func(f *fixture) {
			setRuns(f, "b1-oid", broken)
			f.hub.on(rerunRoute(7), "{}")
		}, "still not clean after 30m0s"},
		{"listing pulls fails", func(f *fixture) { failRoute(f, list("/pulls?state=open")) }, "500"},
		{"a held back PR's runs cannot be read", func(f *fixture) {
			f.hub.on(list("/pulls?state=open"), []PR{{Title: "(PRs 1)", Head: Ref{Ref: "gtmq_x"}}})
			prev := f.hub.hook
			f.hub.hook = func(method, path, body string, status int) {
				prev(method, path, body, status)
				if path == "/repos/o/r/issues/2/labels" {
					failRoute(f, runsRoute("b2-oid"))
				}
			}
		}, "500"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			f := newFixture(t)
			newStackGH(t, f, green(t, 1, "b1", "fb"), green(t, 2, "b2", "b1"))
			f.owner(t, Record{Ticket: 40, Worktree: "/w/40", State: Done})
			tc.setup(f)
			code, stdout, stderr := f.agents(t, "land-stack", "2")
			if code == 0 || !strings.Contains(stderr, tc.want) {
				t.Fatalf("%d %q %q", code, stdout, stderr)
			}
		})
	}
}

func TestLandStack_countsTheBrokenRunsOnAHeldBackPR(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	newStackGH(t, f, green(t, 1, "b1", "fb"), green(t, 2, "b2", "b1"))
	f.owner(t, Record{Ticket: 40, Worktree: "/w/40", State: Done})
	f.hub.on(list("/pulls?state=open"), []PR{{Title: "(PRs 1)", Head: Ref{Ref: "gtmq_x"}}})
	prev := f.hub.hook
	f.hub.hook = func(method, path, body string, status int) {
		prev(method, path, body, status)
		if path == "/repos/o/r/issues/2/labels" {
			setRuns(f, "b2-oid", Run{ID: 7, WorkflowID: 1, Status: "completed", Conclusion: "cancelled", Attempt: 1},
				Run{ID: 9, WorkflowID: 2, Status: "completed", Conclusion: "success", Attempt: 1})
		}
	}
	_, stdout, stderr := f.agents(t, "land-stack", "2")
	if !strings.Contains(stdout, "held back: #2 (1 cancelled or failed runs on its head)") {
		t.Fatalf("%q %q", stdout, stderr)
	}
}

func TestLandStack_stopsWaitingOnRunsOrTheDraftWhenTheContextEnds(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name  string
		setup func(f *fixture)
	}{
		{"runs", func(f *fixture) {
			setRuns(f, "b1-oid", Run{ID: 7, WorkflowID: 1, Name: "ci", Status: "in_progress", Attempt: 1})
		}},
		{"draft", func(f *fixture) { f.hub.on(list("/pulls?state=open"), "[]") }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			f := newFixture(t)
			newStackGH(t, f, green(t, 1, "b1", "fb"))
			f.owner(t, Record{Ticket: 40, Worktree: "/w/40", State: Done})
			tc.setup(f)
			ctx, cancel := context.WithCancel(t.Context())
			env := f.Env(t)
			env.After = func(time.Duration) <-chan time.Time {
				cancel()
				return make(chan time.Time)
			}
			err := landStackCmd(ctx, env, []string{"1"}, &strings.Builder{})
			if !errors.Is(err, context.Canceled) {
				t.Fatalf("%v", err)
			}
		})
	}
}

func doneRun(id, workflow int64, conclusion string, at time.Time) Run {
	return Run{
		ID: id, WorkflowID: workflow, Name: "pr-format", Status: "completed", Conclusion: conclusion,
		Attempt: 1, CreatedAt: at,
	}
}

func TestLandStack_ignoresASupersededBrokenRunWhenANewerRunOfTheWorkflowPassed(t *testing.T) {
	t.Parallel()
	for _, old := range []string{"cancelled", "failure"} {
		f := newFixture(t)
		newStackGH(t, f, green(t, 1, "b1", "fb"), green(t, 2, "b2", "b1"))
		f.owner(t, Record{Ticket: 40, Worktree: "/w/40", State: Done})
		at := time.Unix(1_700_000_000, 0)
		setRuns(f, "b2-oid",
			doneRun(7, 2, old, at),
			doneRun(9, 2, "success", at.Add(time.Minute)),
			doneRun(10, 3, "skipped", at))
		code, stdout, stderr := f.agents(t, "land-stack", "2")
		if code != 0 || !strings.Contains(stdout, "queued #1 #2") {
			t.Fatalf("%s: %d %q %q", old, code, stdout, stderr)
		}
		if got := f.hub.callsContaining("/rerun"); len(got) != 0 {
			t.Fatalf("%s: reran %v", old, got)
		}
		if got := f.hub.callsContaining("/labels"); len(got) == 0 {
			t.Fatalf("%s: no labels added", old)
		}
	}
}

func TestNewestPerWorkflow_breaksCreatedAtTiesById(t *testing.T) {
	t.Parallel()
	at := time.Unix(1_700_000_000, 0)
	got := newestPerWorkflow([]Run{{ID: 5, WorkflowID: 1, CreatedAt: at}, {ID: 6, WorkflowID: 1, CreatedAt: at}})
	if len(got) != 1 || got[0].ID != 6 {
		t.Fatalf("%v", got)
	}
}
