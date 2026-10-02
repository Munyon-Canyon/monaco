package agents

import (
	"context"
	"errors"
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
				"gh pr edit 1 --remove-label merge-queue -R o/r", "gh pr edit 2 --remove-label merge-queue -R o/r",
			}, tc.removes)
			if got := s.lines(); !slices.Equal(got, want) {
				t.Fatalf("calls %v", got)
			}
			if !slices.Equal(f.waited, slices.Repeat([]time.Duration{30 * time.Second}, tc.waits)) {
				t.Fatalf("waited %v", f.waited)
			}
		})
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
			if f.owned(t).Queued == nil || len(s.lines()) != 6 {
				t.Fatalf("queued %+v, calls %v", f.owned(t).Queued, s.lines())
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
		{name: "not queued", args: []string{"1"}, want: "#40 has no queued stack with top #1"},
		{name: "label removal fails", want: "boom", edit: func(_ *fixture, s *stackGH, _ *Env) {
			s.fail = "gh pr edit 2 --remove-label"
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
		{name: "unwritable record", want: "write owner record", edit: func(f *fixture, _ *stackGH, _ *Env) {
			if err := os.Chmod(f.Env(t).recordPath(40), 0o400); err != nil {
				t.Fatal(err)
			}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			f := newFixture(t)
			s, env := dequeueStack(t, f)
			if tc.edit != nil {
				tc.edit(f, s, env)
			}
			args := tc.args
			if args == nil {
				args = []string{"2"}
			}
			err := dequeueCmd(t.Context(), env, args, &strings.Builder{})
			if err == nil || !strings.Contains(cliText(err)+err.Error(), tc.want) {
				t.Fatalf("%v", err)
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
