package agents

import (
	"context"
	"errors"
	"net/http"
	"os"
	"slices"
	"strings"
	"testing"
	"time"
)

func dequeueStack(t *testing.T, f *fixture, drafts ...string) (*stackGH, *Env) {
	t.Helper()
	s := newStackGH(t, f,
		labeled(green(t, 1, "b1", "fb"), "merge-queue"), labeled(green(t, 2, "b2", "b1"), "merge-queue"))
	f.owner(t, Record{Ticket: 40, Worktree: "/w/40", Queued: Queues{{Top: 2, PRs: []int{1, 2}}}})
	f.hub.on(graphqlRoute, `{"data":{"repository":{"drafts":{"nodes":[`+strings.Join(drafts, ",")+`]}}}}`)
	return s, f.Env(t)
}

func readdOn(f *fixture, env *Env, s *stackGH, waits ...int) {
	n := 0
	env.After = func(d time.Duration) <-chan time.Time {
		n++
		if slices.Contains(waits, n) {
			labeled(s.prs[2], "merge-queue")
		}
		return f.after(d)
	}
}

func TestDequeue_removesTheLabelAndWaitsForGraphiteToLetGo(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name    string
		readd   []int
		removes int
		waits   int
	}{
		{"Graphite lets go at once", nil, 1, 2},
		{"Graphite re-adds the label once", []int{2}, 2, 4},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			f := newFixture(t)
			s, env := dequeueStack(t, f,
				queueDraftNode(90, "[Graphite MQ] Draft PR GROUP:spec_9 (PRs 3, 4)", noRollup))
			readdOn(f, env, s, tc.readd...)
			var out strings.Builder
			if err := dequeueCmd(t.Context(), env, []string{"2"}, &out); err != nil {
				t.Fatal(err)
			}
			if out.String() != "dequeued #1 #2; safe to push\n" || len(f.owned(t).Queued) > 0 {
				t.Fatalf("%q, queued %+v", out.String(), f.owned(t).Queued)
			}
			want := slices.Repeat([]string{
				"DELETE /repos/o/r/issues/1/labels/merge-queue",
				"DELETE /repos/o/r/issues/2/labels/merge-queue",
			}, tc.removes)
			if got := f.hub.callsContaining("/labels"); !slices.Equal(got, want) {
				t.Fatalf("calls %v", got)
			}
			if !slices.Equal(f.waited, slices.Repeat([]time.Duration{30 * time.Second}, tc.waits)) {
				t.Fatalf("waited %v", f.waited)
			}
		})
	}
}

func TestDequeue_aClosedDraftThatTestedTheStackDoesNotHoldIt(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	_, env := dequeueStack(t, f)
	f.hub.on(graphqlRoute, `{"data":{"repository":{"drafts":{"nodes":[]},"closed":{"nodes":[`+
		closedDraft(90, noRollup)+`]}}}}`)
	var out strings.Builder
	err := dequeueCmd(t.Context(), env, []string{"2"}, &out)
	if err != nil || out.String() != "dequeued #1 #2; safe to push\n" {
		t.Fatalf("%q %v", out.String(), err)
	}
}

func TestQueueDrafts_asksForOpenAndClosedDraftsAndWhenAClosedOneWasUpdated(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	f.hub.on(graphqlRoute, draftData(nil))
	if _, err := f.Env(t).queueDrafts(t.Context()); err != nil {
		t.Fatal(err)
	}
	sent := f.hub.body(graphqlRoute)
	for _, want := range []string{
		"drafts: pullRequests(states:OPEN,",
		"closed: pullRequests(states:CLOSED,",
		"headRefName headRefOid updatedAt closedAt}}",
	} {
		if !strings.Contains(sent, want) {
			t.Fatalf("the drafts query lacks %q:\n%s", want, sent)
		}
	}
}

func TestDequeue_givesUpWhileGraphiteStillHoldsTheStack(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name   string
		drafts []string
		readd  []int
	}{
		{"Graphite keeps re-adding the label", nil, []int{1, 2, 3}},
		{"an open draft still tests the stack", []string{
			queueDraftNode(90, "[Graphite MQ] Draft PR GROUP:spec_9 (PRs 1, 2)", noRollup),
		}, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			f := newFixture(t)
			s, env := dequeueStack(t, f, tc.drafts...)
			readdOn(f, env, s, tc.readd...)
			err := dequeueCmd(t.Context(), env, []string{"2"}, &strings.Builder{})
			if cliText(err) != "Graphite still holds #2; remove it from the queue in the Graphite app, then rerun" {
				t.Fatal(err)
			}
			if len(f.owned(t).Queued) == 0 || len(f.hub.callsContaining("/labels")) != 6 {
				t.Fatalf("queued %+v, calls %v", f.owned(t).Queued, f.hub.callsContaining("/labels"))
			}
		})
	}
}

func textOf(err error) string {
	if err == nil {
		return "no error"
	}
	return cliText(err)
}

func graphiteTook(t *testing.T, f *fixture, s *stackGH, by string, ago time.Duration) {
	t.Helper()
	for _, n := range []int{1, 2} {
		setUnlabels(t, s.prs[n], unlabel(f.now.Add(-ago), "merge-queue", by))
	}
}

func TestDequeue_aStackGraphiteTookIsNotSafeToPushBeforeItsDraftOpens(t *testing.T) {
	t.Parallel()
	const (
		drafted = "Graphite still holds #2; remove it from the queue in the Graphite app, then rerun"
		took    = "Graphite took #1 #2 and has not opened its draft; the hold clears at 2026-09-27T12:28:00Z, then rerun"
	)
	for _, tc := range []struct {
		name   string
		queued bool
		drafts string
		want   string
	}{
		{"an owner record with the stack queued", true, "", took},
		{"an owner record with no queue entry", false, "", took},
		{"an open draft tests a stack with no labels", false, queueDraftNode(90, "(PRs 1, 2)", noRollup), drafted},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			f := newFixture(t)
			s, env := dequeueStack(t, f, tc.drafts)
			if tc.drafts == "" {
				graphiteTook(t, f, s, graphiteApp, 2*time.Minute)
			} else {
				s.prs[1].Labels.Nodes, s.prs[2].Labels.Nodes = nil, nil
			}
			if !tc.queued {
				f.owner(t, Record{Ticket: 40, State: Running, Worktree: "/w/40"})
			}
			var out strings.Builder
			err := dequeueCmd(t.Context(), env, []string{"2"}, &out)
			if textOf(err) != tc.want || strings.Contains(out.String(), "safe to push") {
				t.Fatalf("%q %s", out.String(), textOf(err))
			}
			if tc.queued && len(f.owned(t).Queued) == 0 {
				t.Fatal("dequeue unmarked a stack Graphite still holds")
			}
		})
	}
}

func TestDequeue_aLabeledTopDoesNotHideALowerPRGraphiteTook(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	s, env := dequeueStack(t, f)
	f.owner(t, Record{Ticket: 40, State: Running, Worktree: "/w/40"})
	setUnlabels(t, s.prs[1], unlabel(f.now.Add(-2*time.Minute), "merge-queue", graphiteApp))
	var out strings.Builder
	err := dequeueCmd(t.Context(), env, []string{"2"}, &out)
	const want = "Graphite took #1 and has not opened its draft; the hold clears at 2026-09-27T12:28:00Z, then rerun"
	if textOf(err) != want || strings.Contains(out.String(), "safe to push") {
		t.Fatalf("%q %s", out.String(), textOf(err))
	}
	released := []string{"DELETE /repos/o/r/issues/2/labels/merge-queue"}
	if got := f.hub.callsContaining("/labels"); !slices.Equal(got, released) {
		t.Fatalf("the labeled top was not released alone: %v", got)
	}
}

func TestDequeue_aLandedPRIsNotHeld(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ name, state, trunk string }{
		{"a closed PR", "CLOSED", ""},
		{"an open PR with its squash commit on the trunk", "OPEN", `[{"commit":{"message":"A (#1)"}}]`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			f := newFixture(t)
			s, env := dequeueStack(t, f)
			setUnlabels(t, s.prs[1], unlabel(f.now.Add(-2*time.Minute), "merge-queue", graphiteApp))
			s.prs[1].State = tc.state
			if tc.trunk != "" {
				f.hub.on(list("/commits?sha=fb&since=2026-09-27T10:58:00Z"), tc.trunk)
			}
			var out strings.Builder
			err := dequeueCmd(t.Context(), env, []string{"2"}, &out)
			if err != nil || out.String() != "dequeued #1 #2; safe to push\n" {
				t.Fatalf("%q %s", out.String(), textOf(err))
			}
			if len(f.owned(t).Queued) > 0 {
				t.Fatal("dequeue kept the queued mark")
			}
		})
	}
}

func TestDequeue_namesWhenTheLastPRGraphiteTookStopsBeingHeld(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name         string
		lower, upper time.Duration
	}{
		{"the upper PR was taken later", 5 * time.Minute, 2 * time.Minute},
		{"the lower PR was taken later", 2 * time.Minute, 5 * time.Minute},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			f := newFixture(t)
			s, env := dequeueStack(t, f)
			f.owner(t, Record{Ticket: 40, State: Running, Worktree: "/w/40"})
			setUnlabels(t, s.prs[1], unlabel(f.now.Add(-tc.lower), "merge-queue", graphiteApp))
			setUnlabels(t, s.prs[2], unlabel(f.now.Add(-tc.upper), "merge-queue", graphiteApp))
			err := dequeueCmd(t.Context(), env, []string{"2"}, &strings.Builder{})
			const want = "Graphite took #1 #2 and has not opened its draft; the hold clears at 2026-09-27T12:28:00Z, then rerun"
			if textOf(err) != want {
				t.Fatal(textOf(err))
			}
		})
	}
}

func TestDequeue_aTopThatIsNotOpenHasNoStackToHold(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	s, _ := dequeueStack(t, f)
	f.owner(t, Record{Ticket: 40, State: Running, Worktree: "/w/40"})
	s.prs[2].State = "MERGED"
	var queries []string
	f.run = func(ctx context.Context, dir, stdin, name string, args ...string) ([]byte, error) {
		queries = append(queries, args...)
		return s.run(ctx, dir, stdin, name, args...)
	}
	var out strings.Builder
	err := dequeueCmd(t.Context(), f.Env(t), []string{"2"}, &out)
	if err != nil || out.String() != "no PR of the stack under #2 carries merge-queue; safe to push\n" {
		t.Fatalf("%q %v", out.String(), err)
	}
	if i := slices.IndexFunc(queries, func(q string) bool { return strings.Contains(q, "{}}") }); i >= 0 {
		t.Fatalf("sent a query that selects nothing: %s", queries[i])
	}
}

func TestDequeue_aStackIsSafeToPushOnceGraphiteHasLetGo(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name   string
		by     string
		ago    time.Duration
		closed func(*fixture) string
	}{
		{"a person removed the labels", "logan", 2 * time.Minute, nil},
		{"Graphite took them a whole hold ago", graphiteApp, takenFor, nil},
		{"a closed draft ran them since", graphiteApp, 2 * time.Minute, func(f *fixture) string {
			return closedDraftOf(t, 92, "1, 2", f.now.Add(-time.Minute))
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			f := newFixture(t)
			s, env := dequeueStack(t, f)
			graphiteTook(t, f, s, tc.by, tc.ago)
			f.owner(t, Record{Ticket: 40, State: Running, Worktree: "/w/40"})
			if tc.closed != nil {
				f.hub.on(graphqlRoute, closedDraftData(tc.closed(f)))
			}
			var out strings.Builder
			err := dequeueCmd(t.Context(), env, []string{"2"}, &out)
			if err != nil || out.String() != "no PR of the stack under #2 carries merge-queue; safe to push\n" {
				t.Fatalf("%q %v", out.String(), err)
			}
		})
	}
}

func TestDequeue_aStackWithNoLabelsReportsUnreadableDrafts(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	s, env := dequeueStack(t, f)
	s.prs[1].Labels.Nodes, s.prs[2].Labels.Nodes = nil, nil
	f.owner(t, Record{Ticket: 40, State: Running, Worktree: "/w/40"})
	f.hub.on(graphqlRoute, `{"data":null,"errors":[{"message":"rate limited"}]}`)
	var out strings.Builder
	err := dequeueCmd(t.Context(), env, []string{"2"}, &out)
	if err == nil || cliText(err) != "graphql: rate limited" || out.String() != "" {
		t.Fatalf("%q %v", out.String(), err)
	}
	if len(f.hub.callsContaining("/labels")) != 0 || len(f.waited) != 0 {
		t.Fatalf("labels %v, waited %v", f.hub.callsContaining("/labels"), f.waited)
	}
}

func TestDequeue_failures(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, want string
		args       []string
		edit       func(f *fixture, s *stackGH, env *Env)
		freeze     bool
	}{
		{name: "usage", args: []string{}, want: "usage: monacoctl agents dequeue <top-pr>"},
		{name: "not a PR", args: []string{"9"}, want: "#9 is not a PR"},
		{name: "no ticket", want: "#2 links no ticket", edit: func(_ *fixture, s *stackGH, _ *Env) {
			s.prs[2].Body = "## TLDR"
		}},
		{name: "no owner record", want: "no owner record for #40", edit: func(f *fixture, _ *stackGH, _ *Env) {
			f.ownerComments(40)
			if err := os.Remove(f.Env(t).recordPath(40)); err != nil {
				t.Fatal(err)
			}
		}},
		{name: "not queued", args: []string{"1"}, want: "#40 has no queued or armed stack with top #1"},
		{name: "label removal fails", want: "boom", edit: func(f *fixture, _ *stackGH, _ *Env) {
			route := "DELETE /repos/o/r/issues/2/labels/merge-queue"
			f.hub.status[route] = http.StatusInternalServerError
			f.hub.on(route, "boom")
		}},
		{name: "drafts unreadable", want: "graphql", edit: func(f *fixture, _ *stackGH, _ *Env) {
			f.hub.on(graphqlRoute, `{"data":null,"errors":[{"message":"rate limited"}]}`)
		}},
		{name: "trunk unreadable", want: "list fb commits", edit: func(f *fixture, s *stackGH, _ *Env) {
			setUnlabels(t, s.prs[1], unlabel(f.now.Add(-2*time.Minute), "merge-queue", graphiteApp))
			route := list("/commits?sha=fb&since=2026-09-27T10:58:00Z")
			f.hub.status[route] = http.StatusInternalServerError
			f.hub.on(route, "boom")
		}},
		{name: "stack unreadable", want: "#1 is not a PR", edit: func(_ *fixture, s *stackGH, env *Env) {
			env.After = func(time.Duration) <-chan time.Time {
				delete(s.prs, 1)
				ch := make(chan time.Time, 1)
				ch <- time.Time{}
				return ch
			}
		}},
		{name: "unwritable record", want: "write owner record", freeze: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			f := newFixture(t)
			s, env := dequeueStack(t, f)
			if tc.edit != nil {
				tc.edit(f, s, env)
			}
			if tc.freeze {
				freeze(t, f.Env(t).recordPath(40))
			}
			args := tc.args
			if args == nil {
				args = []string{"2"}
			}
			err := dequeueCmd(t.Context(), env, args, &strings.Builder{})
			if err == nil || !strings.Contains(cliText(err)+err.Error(), tc.want) {
				t.Fatalf("%v", err)
			}
			if tc.name == "drafts unreadable" && len(f.hub.callsContaining("/pulls")) != 0 {
				t.Fatalf("a non-403 graphql error read REST %v", f.hub.callsContaining("/pulls"))
			}
		})
	}
}

func TestDequeue_stopsWaitingWhenTheContextEnds(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	_, env := dequeueStack(t, f)
	ctx, cancel := context.WithCancel(t.Context())
	env.After = func(time.Duration) <-chan time.Time {
		cancel()
		return make(chan time.Time)
	}
	if err := dequeueCmd(ctx, env, []string{"2"}, &strings.Builder{}); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}

func TestDequeue_clearsTheArmOfAnArmedStack(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	newStackGH(t, f, green(t, 1, "b1", "fb"), stackOf(t, 2, "b2", "b1", "pending", "SUCCESS"))
	f.owner(t, Record{Ticket: 40, Worktree: "/w/40", Armed: Arms{{Top: 2, PRs: []int{1, 2}}}})
	var out strings.Builder
	if err := dequeueCmd(t.Context(), f.Env(t), []string{"2"}, &out); err != nil {
		t.Fatal(err)
	}
	if out.String() != "disarmed #2; agents watch will not land it\n" || len(f.owned(t).Armed) > 0 {
		t.Fatalf("%q, armed %+v", out.String(), f.owned(t).Armed)
	}
	if calls := f.hub.callsContaining("/labels"); len(calls) != 0 || len(f.waited) != 0 {
		t.Fatalf("labels %v, waited %v", calls, f.waited)
	}
}

func TestDequeue_unlabelsAStackWhoseRecordLostItsQueueEntry(t *testing.T) {
	t.Parallel()
	for _, top := range []string{"1840", "1839"} {
		t.Run("top #"+top, func(t *testing.T) {
			t.Parallel()
			f := newFixture(t)
			below := labeled(green(t, 1839, "b1839", "graphite-base/1839"), "merge-queue")
			above := green(t, 1840, "b1840", "b1839")
			below.Body, above.Body = "Part of #590", "Part of #590"
			s := newStackGH(t, f, below, above)
			f.owner(t, Record{Ticket: 590, State: Exited, Worktree: "/w/590"})
			var out strings.Builder
			if err := dequeueCmd(t.Context(), f.Env(t), []string{top}, &out); err != nil {
				t.Fatal(err)
			}
			if out.String() != "dequeued #1839; safe to push\n" || s.prs[1839].labeled("merge-queue") {
				t.Fatalf("%q, labels %+v", out.String(), s.prs[1839].Labels.Nodes)
			}
			if got := f.hub.callsContaining("/labels"); !slices.Equal(got,
				[]string{"DELETE /repos/o/r/issues/1839/labels/merge-queue"}) {
				t.Fatalf("calls %v", got)
			}
		})
	}
}

func TestDequeue_aRecordWithNoQueueEntryAndNoLabelsLeavesGitHubAlone(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	newStackGH(t, f, green(t, 1, "b1", "fb"), green(t, 2, "b2", "b1"))
	f.owner(t, Record{Ticket: 40, State: Running, Worktree: "/w/40"})
	var out strings.Builder
	if err := dequeueCmd(t.Context(), f.Env(t), []string{"2"}, &out); err != nil {
		t.Fatal(err)
	}
	if out.String() != "no PR of the stack under #2 carries merge-queue; safe to push\n" ||
		len(f.hub.callsContaining("/labels")) != 0 || len(f.waited) != 0 {
		t.Fatalf("%q, labels %v, waited %v", out.String(), f.hub.callsContaining("/labels"), f.waited)
	}
}

func TestDequeue_aRecordWithNoQueueEntryReportsGitHubFailures(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ name, want string }{
		{"open PRs unreadable", "open PRs: boom"},
		{"label removal fails", "boom"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			f := newFixture(t)
			s := newStackGH(t, f, labeled(green(t, 1, "b1", "fb"), "merge-queue"), green(t, 2, "b2", "b1"))
			f.owner(t, Record{Ticket: 40, State: Running, Worktree: "/w/40"})
			if tc.name == "open PRs unreadable" {
				f.run = func(ctx context.Context, dir, stdin, name string, args ...string) ([]byte, error) {
					openQuery := func(a string) bool { return strings.Contains(a, "open: pullRequests") }
					if slices.ContainsFunc(args, openQuery) {
						return nil, errors.New("open PRs: boom")
					}
					return s.run(ctx, dir, stdin, name, args...)
				}
			} else {
				route := "DELETE /repos/o/r/issues/1/labels/merge-queue"
				f.hub.status[route] = http.StatusInternalServerError
				f.hub.on(route, "boom")
			}
			err := dequeueCmd(t.Context(), f.Env(t), []string{"2"}, &strings.Builder{})
			if err == nil || !strings.Contains(cliText(err)+err.Error(), tc.want) {
				t.Fatalf("%v", err)
			}
		})
	}
}

func TestDequeue_removesOnlyTheNamedStackFromARecordHoldingSeveral(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	newStackGH(t, f,
		labeled(green(t, 1, "b1", "fb"), "merge-queue"), labeled(green(t, 2, "b2", "b1"), "merge-queue"),
		labeled(green(t, 3, "b3", "fb"), "merge-queue"), stackOf(t, 5, "b5", "fb", "pending", "SUCCESS"))
	f.owner(t, Record{
		Ticket: 40, Worktree: "/w/40",
		Queued: Queues{{Top: 2, PRs: []int{1, 2}}, {Top: 3, PRs: []int{3}}}, Armed: Arms{{Top: 5, PRs: []int{5}}},
	})
	f.hub.on(graphqlRoute, `{"data":{"repository":{"drafts":{"nodes":[]}}}}`)
	var out strings.Builder
	if err := dequeueCmd(t.Context(), f.Env(t), []string{"2"}, &out); err != nil {
		t.Fatal(err)
	}
	r := f.owned(t)
	if out.String() != "dequeued #1 #2; safe to push\n" || len(r.Queued) != 1 || r.Queued[0].Top != 3 ||
		len(r.Armed) != 1 || r.Armed[0].Top != 5 {
		t.Fatalf("%q, queued %+v armed %+v", out.String(), r.Queued, r.Armed)
	}
	out.Reset()
	if err := dequeueCmd(t.Context(), f.Env(t), []string{"5"}, &out); err != nil {
		t.Fatal(err)
	}
	if r := f.owned(t); out.String() != "disarmed #5; agents watch will not land it\n" ||
		len(r.Queued) != 1 || len(r.Armed) != 0 {
		t.Fatalf("%q, queued %+v armed %+v", out.String(), r.Queued, r.Armed)
	}
}
