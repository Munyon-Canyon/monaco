package agents

import (
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
