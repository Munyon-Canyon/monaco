package agents

import (
	"strings"
	"testing"
	"time"
)

func TestHandoff_postsThenEditsTheTrackingComment(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	f.batch(t, 5, 6, 7)
	f.record(t, Record{
		Ticket: 5, Model: "opus", State: Running, Worktree: "/w/5", AgentID: "a1",
		Started: f.now.Add(-2 * time.Hour),
	})
	f.record(t, Record{Ticket: 8, State: Exited})
	f.board(t)
	f.hub.on(list("/issues/7/comments?"), []Comment{{ID: 2, Body: statusMarker}})
	f.hub.on("POST /repos/o/r/issues/7/comments", "ok")
	if code, stdout, stderr := f.agents(t, "handoff"); code != 0 || stdout != "handoff posted to #7\n" {
		t.Fatalf("%d %q %q", code, stdout, stderr)
	}
	want := handoffMarker + "\nHandoff at 2026-09-27T12:00:00Z. Feature branch `fb`.\n\n**Batch**\n\n" +
		"| ticket | state | since dispatch |\n| --- | --- | --- |\n" +
		"| #5 | merged | 1h30m |\n| #6 | queued | - |\n| #7 | building | - |\n" +
		"\n**Running agents**\n\n- #5 opus running in `/w/5`, agent `a1`\n" +
		"\n**Next**\n\n`monacoctl agents dispatch 6 --model opus`\n"
	if got := posted(t, f, "POST /repos/o/r/issues/7/comments"); got != want {
		t.Fatalf("body:\n%s", got)
	}
	f.hub.on(list("/issues/7/comments?"), []Comment{authored(3, want, ghUser, "MEMBER")})
	f.hub.on("PATCH /repos/o/r/issues/comments/3", "ok")
	if code, _, stderr := f.agents(t, "handoff"); code != 0 || f.hub.body("PATCH /repos/o/r/issues/comments/3") == "" {
		t.Fatalf("edit: %d %q", code, stderr)
	}
}

func TestHandoff_withNothingInFlight(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	f.hub.on(list("/issues/7/comments?"), []Comment{})
	f.hub.on("POST /repos/o/r/issues/7/comments", "ok")
	if code, _, stderr := f.agents(t, "handoff"); code != 0 {
		t.Fatalf("%d %q", code, stderr)
	}
	got := posted(t, f, "POST /repos/o/r/issues/7/comments")
	for _, want := range []string{"No batch.\n", "**Running agents**\n\nNone.\n", "`monacoctl agents batch <issue>...`"} {
		if !strings.Contains(got, want) {
			t.Fatalf("lacks %q:\n%s", want, got)
		}
	}
}

func TestHandoff_failures(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	if code, _, stderr := f.agents(
		t,
		"handoff",
		"x",
	); code != 2 ||
		!strings.Contains(stderr, "usage: monacoctl agents handoff") {
		t.Fatalf("usage: %d %q", code, stderr)
	}
	if code, _, stderr := f.agents(t, "handoff"); code != 1 || !strings.Contains(stderr, "comments") {
		t.Fatalf("comments: %d %q", code, stderr)
	}
	f.hub.on(list("/issues/7/comments?"), []Comment{})
	if code, _, stderr := f.agents(t, "handoff"); code != 1 || !strings.Contains(stderr, "POST") {
		t.Fatalf("write: %d %q", code, stderr)
	}
	env := f.Env(t)
	writeFile(t, env.recordPath(9), "{")
	if code, _, stderr := f.agents(t, "handoff"); code != 1 || !strings.Contains(stderr, "9.json") {
		t.Fatalf("records: %d %q", code, stderr)
	}
	f.batch(t, 5)
	if code, _, stderr := f.agents(t, "handoff"); code != 1 || !strings.Contains(stderr, "9.json") {
		t.Fatalf("batch records: %d %q", code, stderr)
	}
	g := newFixture(t)
	g.batch(t, 5)
	g.hub.on(list("/issues/7/comments?"), []Comment{})
	if code, _, stderr := g.agents(t, "handoff"); code != 1 || !strings.Contains(stderr, "graphql") {
		t.Fatalf("views: %d %q", code, stderr)
	}
}

func TestNextCommand(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 9, 27, 12, 0, 0, 0, time.UTC)
	merged := ticketView{Ticket: 1, Dispatched: now, PRs: []ticketPR{{Merged: now}}}
	tests := []struct {
		views []ticketView
		want  string
	}{
		{nil, "monacoctl agents batch <issue>..."},
		{[]ticketView{merged, {Ticket: 2}}, "monacoctl agents dispatch 2 --model opus"},
		{[]ticketView{merged, {Ticket: 3, Dispatched: now}}, "monacoctl agents watch"},
		{[]ticketView{merged}, "monacoctl agents batch <issue>..."},
	}
	for _, tt := range tests {
		if got := nextCommand(tt.views); got != tt.want {
			t.Fatalf("next = %q, want %q", got, tt.want)
		}
	}
}
