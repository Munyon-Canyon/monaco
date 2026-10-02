package agents

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
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
		Run{ID: 7, Name: "ci", Status: "completed", Conclusion: "cancelled", Attempt: 1},
		Run{ID: 8, Name: "pr-format", Status: "completed", Conclusion: "success", Attempt: 1})
	rerunsInto(f, "b2-oid", 7,
		Run{ID: 7, Name: "ci", Status: "in_progress", Attempt: 2},
		Run{ID: 8, Name: "pr-format", Status: "completed", Conclusion: "success", Attempt: 1})
	prev := f.hub.hook
	var polls atomic.Int32
	f.hub.hook = func(method, path, body string, status int) {
		prev(method, path, body, status)
		if method+" "+path == runsRoute("b2-oid") && len(f.hub.callsContaining(rerunRoute(7))) > 0 &&
			polls.Add(1) == 2 {
			setRuns(f, "b2-oid", Run{ID: 7, Name: "ci", Status: "completed", Conclusion: "success", Attempt: 2})
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
	setRuns(f, "b2-oid", Run{ID: 7, Name: "ci", Status: "completed", Conclusion: "failure", Attempt: 1})
	rerunsInto(f, "b2-oid", 7,
		Run{ID: 7, Name: "ci", Status: "completed", Conclusion: "failure", Attempt: 2, URL: "https://x/runs/7"})
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
	f.hub.on(list("/pulls?state=open"), []PR{draft})
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
	setRuns(f, "b1-oid", Run{ID: 7, Name: "ci", Status: "completed", Conclusion: "failure", Attempt: 1})
	f.hub.status[rerunRoute(7)] = 403
	f.hub.on("POST /repos/o/r/actions/runs/7/rerun-failed-jobs", "{}")
	prev := f.hub.hook
	f.hub.hook = func(method, path, body string, status int) {
		prev(method, path, body, status)
		if path == "/repos/o/r/actions/runs/7/rerun-failed-jobs" {
			setRuns(f, "b1-oid", Run{ID: 7, Name: "ci", Status: "completed", Conclusion: "success", Attempt: 2})
		}
	}
	code, stdout, stderr := f.agents(t, "land-stack", "1")
	if code != 0 || !strings.Contains(stdout, `reran "ci" (run 7, was failure)`) {
		t.Fatalf("%d %q %q", code, stdout, stderr)
	}
}
