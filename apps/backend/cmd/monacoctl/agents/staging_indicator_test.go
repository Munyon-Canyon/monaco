package agents

import (
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

func dispatchedFor(t *testing.T, r *trunkRepo, lines []string) {
	t.Helper()
	if len(lines) != 2 || !strings.HasPrefix(lines[0], "staging fast-forwarded to ") {
		t.Fatalf("lines %q", lines)
	}
	sha := strings.TrimPrefix(lines[0], "staging fast-forwarded to ")
	if lines[1] != "dispatched staging indicator for "+sha || !strings.HasPrefix(r.head(), sha) {
		t.Fatalf("lines %q at %s", lines, r.head())
	}
}

func dispatches(r *trunkRepo) int {
	return len(r.hub.callsContaining("/actions/workflows/staging.yml/dispatches"))
}

func TestStagingIndicator_dispatchesOncePerHourAfterAFastForward(t *testing.T) {
	t.Parallel()
	r := newTrunkRepo(t)
	s := newStream(r.env)
	r.land("a.txt")
	dispatchedFor(t, r, s.trunkLines(t.Context()))
	if got := r.hub.body(indicatorRoute); got != `{"ref":"staging"}` {
		t.Fatalf("dispatch body %q", got)
	}
	r.now = r.now.Add(10 * time.Minute)
	r.land("b.txt")
	if got := s.trunkLines(t.Context()); len(got) != 1 || dispatches(r) != 1 {
		t.Fatalf("10 minutes later printed %q after %d dispatches", got, dispatches(r))
	}
	r.now = r.now.Add(51 * time.Minute)
	r.land("c.txt")
	dispatchedFor(t, r, s.trunkLines(t.Context()))
	if dispatches(r) != 2 {
		t.Fatalf("%d dispatches, want 2", dispatches(r))
	}
}

func TestStagingIndicator_printsTheErrorAndRetriesOnALaterPoll(t *testing.T) {
	t.Parallel()
	r := newTrunkRepo(t)
	s := newStream(r.env)
	r.hub.status[indicatorRoute] = 500
	r.land("a.txt")
	got := s.trunkLines(t.Context())
	if len(got) != 2 || !strings.HasPrefix(got[1], "watch error: dispatch staging indicator: POST ") {
		t.Fatalf("lines %q", got)
	}
	r.hub.mu.Lock()
	delete(r.hub.status, indicatorRoute)
	r.hub.mu.Unlock()
	r.now = r.now.Add(time.Minute)
	retry := s.trunkLines(t.Context())
	want := "dispatched staging indicator for " + strings.TrimPrefix(got[0], "staging fast-forwarded to ")
	if !slices.Equal(retry, []string{want}) {
		t.Fatalf("retry printed %q, want %q", retry, want)
	}
	if got := s.trunkLines(t.Context()); len(got) != 0 {
		t.Fatalf("after the retry printed %q", got)
	}
}

func TestStagingIndicator_staysIdleWhileALandingJustDispatchedItsOwnRun(t *testing.T) {
	t.Parallel()
	r := newTrunkRepo(t)
	landing := Landing{Top: 2, PRs: []int{1, 2}, SHA: strings.Repeat("a", 40), At: r.now.Add(-10 * time.Minute)}
	if err := r.env.saveRecord(Record{Ticket: 40, Landed: []Landing{landing}}); err != nil {
		t.Fatal(err)
	}
	s := newStream(r.env)
	r.land("a.txt")
	if got := s.trunkLines(t.Context()); len(got) != 1 || dispatches(r) != 0 {
		t.Fatalf("printed %q after %d dispatches", got, dispatches(r))
	}
	r.now = r.now.Add(51 * time.Minute)
	r.land("b.txt")
	dispatchedFor(t, r, s.trunkLines(t.Context()))
}

func TestStagingIndicator_dispatchesWhenAnOwnerRecordCannotBeRead(t *testing.T) {
	t.Parallel()
	r := newTrunkRepo(t)
	writeFile(t, filepath.Join(r.env.Common, recordsDir, "40.json"), "{")
	s := newStream(r.env)
	r.land("a.txt")
	dispatchedFor(t, r, s.trunkLines(t.Context()))
}

func listRunsRoute(r *trunkRepo) string {
	at := r.now.Add(-indicatorEvery).UTC().Format(time.RFC3339)
	return get("/actions/workflows/staging.yml/runs?event=workflow_dispatch&branch=staging&created=>=" + at +
		"&per_page=100")
}

func alreadyDispatched(t *testing.T, r *trunkRepo, lines []string, sha, url string) {
	t.Helper()
	sha = sha[:7]
	want := []string{"staging fast-forwarded to " + sha, "staging run for " + sha + " already dispatched (" + url + ")"}
	if !slices.Equal(lines, want) || dispatches(r) != 0 {
		t.Fatalf("lines %q after %d dispatches, want %q", lines, dispatches(r), want)
	}
}

func TestStagingIndicator_skipsTheDispatchWhenARunForTheNewHeadExists(t *testing.T) {
	t.Parallel()
	r := newTrunkRepo(t)
	s := newStream(r.env)
	sha := r.land("a.txt")
	r.hub.on(listRunsRoute(r), map[string]any{"workflow_runs": []Run{
		{ID: 1, Status: "completed", HeadSHA: sha, URL: "https://runs/1"},
	}})
	alreadyDispatched(t, r, s.trunkLines(t.Context()), sha, "https://runs/1")
	r.now = r.now.Add(10 * time.Minute)
	r.land("b.txt")
	if got := s.trunkLines(t.Context()); len(got) != 1 || dispatches(r) != 0 {
		t.Fatalf("10 minutes later printed %q after %d dispatches", got, dispatches(r))
	}
}

func TestStagingIndicator_skipsTheDispatchWhileADispatchedRunIsQueued(t *testing.T) {
	t.Parallel()
	r := newTrunkRepo(t)
	s := newStream(r.env)
	sha := r.land("a.txt")
	r.hub.on(listRunsRoute(r), map[string]any{"workflow_runs": []Run{
		{ID: 2, Status: "queued", HeadSHA: strings.Repeat("b", 40), URL: "https://runs/2"},
	}})
	alreadyDispatched(t, r, s.trunkLines(t.Context()), sha, "https://runs/2")
}

func TestStagingIndicator_dispatchesWhenItCannotListTheRuns(t *testing.T) {
	t.Parallel()
	r := newTrunkRepo(t)
	s := newStream(r.env)
	r.hub.status[listRunsRoute(r)] = 500
	r.land("a.txt")
	dispatchedFor(t, r, s.trunkLines(t.Context()))
	if dispatches(r) != 1 {
		t.Fatalf("%d dispatches, want 1", dispatches(r))
	}
}
