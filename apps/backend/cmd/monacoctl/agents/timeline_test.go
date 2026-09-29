package agents

import (
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestTimeline_printsEachStepAndTheBatchTotal(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	f.batch(t, 5, 6, 7)
	f.record(t, Record{Ticket: 5, State: Done, Started: f.now.Add(-2 * time.Hour)})
	f.board(t)
	code, stdout, stderr := f.agents(t, "timeline")
	want := "ticket  dispatch     push         stage 1      verdict      queued       merged       total\n" +
		"#5      09-27 10:00  09-27 10:10  09-27 10:30  09-27 10:40  09-27 10:50  09-27 11:30  1h30m\n" +
		"#6      -            09-27 11:40  -            -            -            -            -\n" +
		"#7      -            -            -            -            -            -            -\n" +
		"batch: 1 of 3 merged, 1h30m from first dispatch to last merge\n"
	if code != 0 || stdout != want {
		t.Fatalf("code=%d stderr=%q stdout:\n%s", code, stderr, stdout)
	}
}

func TestTimeline_readsAnotherBatchFileAndRefusesBadInput(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	other := filepath.Join(t.TempDir(), "old.json")
	writeFile(t, other, `{"tickets":[{"ticket":7,"dispatched":"2026-09-27T11:00:00Z"}]}`)
	f.board(t)
	code, stdout, _ := f.agents(t, "timeline", "--batch", other)
	if code != 0 || !strings.Contains(stdout, "#7      09-27 11:00  -") ||
		!strings.HasSuffix(stdout, "batch: 0 of 1 merged, - from first dispatch to last merge\n") {
		t.Fatalf("code=%d stdout:\n%s", code, stdout)
	}
	for _, args := range [][]string{{"timeline", "x"}, {"timeline", "--file", "x"}} {
		if code, _, stderr := f.agents(t, args...); code != 2 || !strings.Contains(stderr, timelineUse) {
			t.Fatalf("%q: %d %q", args, code, stderr)
		}
	}
	env := f.Env(t)
	if code, _, stderr := f.agents(
		t,
		"timeline",
	); code != 1 ||
		!strings.Contains(stderr, "no batch at "+env.batchPath()) {
		t.Fatalf("missing: %d %q", code, stderr)
	}
	writeFile(t, env.recordPath(9), "{")
	if code, _, stderr := f.agents(t, "timeline", "--batch", other); code != 1 || !strings.Contains(stderr, "9.json") {
		t.Fatalf("records: %d %q", code, stderr)
	}
	writeFile(t, other, "{")
	if code, _, stderr := f.agents(
		t,
		"timeline",
		"--batch",
		other,
	); code != 1 ||
		!strings.Contains(stderr, "old.json") {
		t.Fatalf("decode: %d %q", code, stderr)
	}
}

func TestTimeline_failsWhenGitHubDoes(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	f.batch(t, 5)
	if code, _, stderr := f.agents(t, "timeline"); code != 1 || !strings.Contains(stderr, "graphql") {
		t.Fatalf("%d %q", code, stderr)
	}
}

func TestMarks_countAStackOnceEveryPRReachedTheStep(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 9, 27, 12, 0, 0, 0, time.UTC)
	ago := func(m int) time.Time { return now.Add(-time.Duration(m) * time.Minute) }
	done := ticketPR{
		Opened: ago(50), Stage1: "success", Stage1At: ago(40), Verify: "success", VerifyAt: ago(35),
		Queued: []queueEvent{{true, ago(30)}, {false, ago(20)}, {true, ago(15)}}, Merged: ago(10),
	}
	late := ticketPR{Opened: ago(45), Stage1: "success", Stage1At: ago(38), Verify: "pending", VerifyAt: ago(37)}
	tests := []struct {
		name string
		prs  []ticketPR
		want [6]time.Time
	}{
		{"one PR", []ticketPR{done}, [6]time.Time{ago(60), ago(50), ago(40), ago(35), ago(30), ago(10)}},
		{
			"stack with a verdict pending",
			[]ticketPR{late, done},
			[6]time.Time{ago(60), ago(50), ago(38), {}, ago(30), {}},
		},
		{"no PR", nil, [6]time.Time{ago(60)}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := (ticketView{Dispatched: ago(60), PRs: tt.prs}).marks(); got != tt.want {
				t.Fatalf("marks\n got %v\nwant %v", got, tt.want)
			}
		})
	}
}

func TestBetween(t *testing.T) {
	t.Parallel()
	start := time.Date(2026, 9, 27, 12, 0, 0, 0, time.UTC)
	for end, want := range map[time.Time]string{
		{}:                         "-",
		start.Add(5 * time.Minute): "5m",
		start.Add(90*time.Minute + 29*time.Second): "1h30m",
	} {
		if got := between(start, end); got != want {
			t.Fatalf("between = %q, want %q", got, want)
		}
	}
	if got := between(time.Time{}, start); got != "-" {
		t.Fatal(got)
	}
}
