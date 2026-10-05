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
	f.owner(t, Record{Ticket: 40, Worktree: "/w/40", Queued: &Queue{Top: 2, PRs: []int{1, 2}}})
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
			if out.String() != "dequeued #1 #2; safe to push\n" || f.owned(t).Queued != nil {
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
		"headRefName updatedAt}}",
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
			if f.owned(t).Queued == nil || len(f.hub.callsContaining("/labels")) != 6 {
				t.Fatalf("queued %+v, calls %v", f.owned(t).Queued, f.hub.callsContaining("/labels"))
			}
		})
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
	f.owner(t, Record{Ticket: 40, Worktree: "/w/40", Armed: &Arm{Top: 2, PRs: []int{1, 2}}})
	var out strings.Builder
	if err := dequeueCmd(t.Context(), f.Env(t), []string{"2"}, &out); err != nil {
		t.Fatal(err)
	}
	if out.String() != "disarmed #2; agents watch will not land it\n" || f.owned(t).Armed != nil {
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
