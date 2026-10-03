package agents

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

const mqTitle = "[Graphite MQ] Draft PR GROUP:spec_de1996 (PRs 1307, 1308, 1309)"

func queueDraftNode(n int, title, commit string) string {
	return fmt.Sprintf(`{"number":%d,"state":"OPEN","title":%q,"body":"","headRefName":"gtmq_%d",`+
		`"updatedAt":"2026-09-27T11:59:00Z","commits":{"nodes":[{"commit":%s}]}}`, n, title, n, commit)
}

func closedDraft(n int, commit string) string {
	title := fmt.Sprintf("[Graphite MQ] Draft PR GROUP:spec_%d (PRs 1, 2)", n)
	return strings.Replace(queueDraftNode(n, title, commit), `"state":"OPEN"`, `"state":"CLOSED"`, 1)
}

func streamRounds(t *testing.T, f *fixture, rounds int, between func(round int)) string {
	t.Helper()
	env := f.Env(t)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	n := 0
	env.After = func(d time.Duration) <-chan time.Time {
		if d == dequeueEvery {
			return f.after(d)
		}
		if d != time.Minute {
			t.Errorf("waited %s between rounds", d)
		}
		n++
		if n >= rounds {
			cancel()
			return make(chan time.Time)
		}
		between(n)
		ch := make(chan time.Time, 1)
		ch <- f.now
		return ch
	}
	var out strings.Builder
	if err := env.watchStream(ctx, time.Minute, &out); err != nil {
		t.Fatal(err)
	}
	return out.String()
}

func queuedStack(t *testing.T, f *fixture, worktree string) *stackGH {
	t.Helper()
	s := newStackGH(t, f,
		labeled(green(t, 1, "b1", "fb"), "merge-queue"), labeled(green(t, 2, "b2", "b1"), "merge-queue"))
	f.owner(t, Record{Ticket: 40, State: Exited, Worktree: worktree, Queued: &Queue{Top: 2, PRs: []int{1, 2}}})
	f.noFailures()
	return s
}

func TestWatchStream_printsTheStateOnceThenOnlyWhatChanged(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	queuedStack(t, f, "/w/40")
	pending := `{"name":"ci / E2E","status":"IN_PROGRESS"}`
	f.hub.on(graphqlRoute, draftData([]string{
		queueDraftNode(90, "[Graphite MQ] Draft PR GROUP:spec_1 (PRs 1, 2)", rollup(greenOK, pending,
			`{"context":"verify","state":"SUCCESS"}`, `{"context":"ext","state":"ERROR"}`,
			`{"context":"slow","state":"PENDING"}`)),
		queueDraftNode(91, "[Graphite MQ] Draft PR GROUP:spec_2 (PRs 12)", rollup(redOK)),
		closedDraft(92, rollup(greenOK, flakeJob)),
	}))
	skipped := `{"name":"Deploy to GitHub Pages","conclusion":"SKIPPED"}`
	got := streamRounds(t, f, 4, func(round int) {
		node := queueDraftNode(90, "[Graphite MQ] Draft PR GROUP:spec_1 (PRs 1, 2)", rollup(greenOK, flakeJob, skipped))
		if round >= 2 {
			node = strings.Replace(node, `"state":"OPEN"`, `"state":"CLOSED"`, 1)
		}
		f.hub.on(graphqlRoute, draftData([]string{node, closedDraft(92, rollup(greenOK, flakeJob))}))
	})
	want := "#1 queued\n#2 queued\ndraft #90 open\ndraft #90 ci / ci-ok: pass\n" +
		"draft #90 verify: pass\ndraft #90 ext: fail\ndraft #90 ci / Flake: fail\ndraft #90 closed\n"
	if got != want {
		t.Fatalf("stream\n got %q\nwant %q", got, want)
	}
}

func TestWatchStream_settlesAStackOnceEveryPRLanded(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	wt := t.TempDir()
	s := queuedStack(t, f, wt)
	f.hub.on(get("/compare/fb...b2-oid"), `{"status":"identical"}`)
	got := streamRounds(t, f, 3, func(round int) {
		if round == 1 {
			s.prs[1].State, s.prs[2].State = "MERGED", "CLOSED"
		}
	})
	want := "#1 queued\n#2 queued\n#1 landed\n#2 landed\nstack #2 landed (#1 #2)\n"
	if got != want {
		t.Fatalf("stream\n got %q\nwant %q", got, want)
	}
	if lines := s.lines(); len(lines) != 1 || lines[0] != "gt sync --no-interactive --delete-all --no-restack" {
		t.Fatalf("calls %v", lines)
	}
	if f.owned(t).Queued != nil {
		t.Fatal("kept the queued mark")
	}
}

func TestWatchStream_ejectsAStackOnlyAfterTwoRoundsWithoutTheLabel(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name    string
		back    bool
		ejected bool
		closed  bool
	}{
		{"the label stays gone", false, true, false},
		{"the label comes back after one round", true, false, false},
		{"the PR was closed by hand", false, true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			f := newFixture(t)
			s := queuedStack(t, f, "/w/40")
			f.hub.on(get("/compare/fb...b2-oid"), `{"status":"diverged"}`)
			got := streamRounds(t, f, 4, func(round int) {
				switch {
				case round == 1 && tc.closed:
					s.prs[2].State = "CLOSED"
				case round == 1:
					s.prs[2].Labels.Nodes = nil
				case round == 2 && tc.back:
					labeled(s.prs[2], "merge-queue")
				case round == 2 && f.owned(t).Queued == nil:
					t.Error("unmarked after one round")
				}
			})
			why := "left the Graphite merge queue"
			if tc.closed {
				why = "was closed without landing"
			}
			block := "stack #2 ejected: #2 " + why + "\n#2 " + why + "\n" +
				"  failing job: none\n  fresh owner\n  ticket: 40\n  worktree: /w/40\n  head: b2-oid\n"
			if strings.Contains(got, block) != tc.ejected || (f.owned(t).Queued == nil) != tc.ejected {
				t.Fatalf("ejected = %v, want %v:\n%s", f.owned(t).Queued == nil, tc.ejected, got)
			}
			if !strings.HasPrefix(got, "#1 queued\n#2 queued\n#2 ejected\n") {
				t.Fatalf("stream:\n%s", got)
			}
			if tc.ejected && (s.prs[1].labeled("merge-queue") || s.prs[2].labeled("merge-queue")) {
				t.Fatalf("an ejected stack kept the label: #1 %v, #2 %v", s.prs[1].Labels, s.prs[2].Labels)
			}
		})
	}
}

func TestWatchStream_aStackListedByAnOpenDraftIsNotEjectedWithoutTheLabel(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	s := queuedStack(t, f, "/w/40")
	f.hub.on(get("/compare/fb...b2-oid"), `{"status":"diverged"}`)
	s.prs[2].Labels.Nodes = nil
	f.hub.on(graphqlRoute, draftData(
		[]string{queueDraftNode(90, "[Graphite MQ] Draft PR GROUP:spec_1 (PRs 1, 2)", rollup(greenOK))},
	))
	got := streamRounds(t, f, 4, func(int) {})
	if strings.Contains(got, "ejected") || f.owned(t).Queued == nil || !strings.Contains(got, "#2 queued\n") {
		t.Fatalf("stream:\n%s", got)
	}
}

func TestWatchStream_printsAFailureBlockOnce(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	nodes := func(at time.Time) string {
		return failureData(
			strings.Replace(watchNode(6, "fb", rollup(redOK, lintJob), ""), "Part of #40", "no ticket", 1),
			watchNode(7, "fb", rollup(greenOK), dropped(at)),
			strings.Replace(watchNode(8, "fb", rollup(greenOK), dropped(at)),
				`"commits"`, `"labels":{"nodes":[{"name":"merge-queue"}]},"commits"`, 1),
			watchNode(9, "fb", rollup(greenOK), dropped(f.now.Add(-time.Hour))),
		)
	}
	f.hub.on(graphqlRoute, nodes(f.now.Add(-time.Hour)))
	got := streamRounds(t, f, 4, func(round int) {
		if round == 1 {
			f.hub.on(graphqlRoute, nodes(f.now.Add(time.Minute)))
		}
	})
	if strings.Count(got, "#6 stage 1 is red\n  failing job: https://gh/job/12\n") != 1 ||
		strings.Count(got, "#7 dropped from the Graphite merge queue\n") != 1 || strings.Contains(got, "#8 ") ||
		strings.Contains(got, "#9 ") || strings.Index(got, "#7 dropped") < strings.Index(got, "#6 stage") {
		t.Fatalf("stream:\n%s", got)
	}
}

func TestWatchArgs(t *testing.T) {
	t.Parallel()
	for args, want := range map[string]string{
		"":                  "false 30s",
		"--once":            "true 30s",
		"--every 10s":       "false 10s",
		"--once --every 1m": "true 1m0s",
		"--every 5s":        "usage",
		"--every":           "usage",
		"x":                 "usage",
	} {
		once, every, err := watchArgs(strings.Fields(args))
		got := fmt.Sprint(once, " ", every)
		if err != nil {
			got = "usage"
			if !strings.Contains(cliText(err), "usage: monacoctl agents watch [--once]") {
				t.Errorf("%q: %v", args, err)
			}
		}
		if got != want {
			t.Errorf("watchArgs(%q) = %s, want %s", args, got, want)
		}
	}
}

func TestStreams_onlyCheckAndTheStreamingWatchBypassTheOutputBuffer(t *testing.T) {
	t.Parallel()
	for args, want := range map[string]bool{"watch": true, "watch --every 1m": true, "watch --once": false, "status": false, "check": true, "check --fresh": true} {
		if got := streams(strings.Fields(args)); got != want {
			t.Errorf("streams(%q) = %v", args, got)
		}
	}
}

func TestMentions_readsGraphiteDraftTitles(t *testing.T) {
	t.Parallel()
	for pr, want := range map[int]bool{1307: true, 1308: true, 1309: true, 130: false, 13070: false, 1: false} {
		if got := mentions(mqTitle, pr); got != want {
			t.Errorf("mentions(%q, %d) = %v", mqTitle, pr, got)
		}
	}
	if !mentions("Merge queue: #12", 12) || mentions("Merge queue: #12", 1) {
		t.Error("#N references")
	}
	since := time.Date(2026, 9, 29, 6, 0, 0, 0, time.UTC)
	d := queueDraft{State: "CLOSED", HeadRefName: "gtmq_spec_de1996", Title: mqTitle, UpdatedAt: since.Add(time.Minute)}
	if !d.runs(1308, since) || d.runs(1310, since) {
		t.Error("a closed draft runs the PRs its title lists")
	}
	d.State = "OPEN"
	if d.runs(1308, since) || !d.tests(1308) {
		t.Error("an open draft is still running")
	}
}

func TestWatchStream_printsEachErrorAsALineAndKeepsGoing(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	s := newStackGH(t, f, green(t, 5, "b5", "fb"))
	s.prs[5].State = "CLOSED"
	delete(f.hub.routes, graphqlRoute)
	f.owner(t, Record{Ticket: 1, State: Running, Worktree: filepath.Join(f.dir, "gone")})
	f.record(t, Record{Ticket: 2, State: Running, Worktree: f.dir})
	f.record(t, Record{Ticket: 40, State: Exited, Queued: &Queue{Top: 9, PRs: []int{9}}})
	f.record(t, Record{Ticket: 41, State: Exited, Queued: &Queue{Top: 5, PRs: []int{5}}})
	env := f.Env(t)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	rounds := 0
	env.After = func(d time.Duration) <-chan time.Time {
		rounds++
		if d != 10*time.Second || rounds == 2 {
			cancel()
			return make(chan time.Time)
		}
		writeFile(t, env.recordPath(42), "{")
		ch := make(chan time.Time, 1)
		ch <- f.now
		return ch
	}
	var out strings.Builder
	if err := watchCmd(ctx, env, []string{"--every", "10s"}, &out); err != nil {
		t.Fatal(err)
	}
	got := out.String()
	for _, want := range []string{
		"watch error: stack #9: ", "#9 is not a PR", "watch error: stack #5: compare b5-oid with fb",
		"graphql", "decode ", "\nwatch error: ",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("stream lacks %q:\n%s", want, got)
		}
	}
	if strings.Count(got, "watch error: ") != 5 || strings.Count(got, "git log") != 1 || rounds != 2 {
		t.Fatalf("%d rounds:\n%s", rounds, got)
	}
}

func TestWatchStream_reportsAFailedSettleOrUnmark(t *testing.T) {
	t.Parallel()
	t.Run("a failing gt sync still lands", func(t *testing.T) {
		t.Parallel()
		f := newFixture(t)
		s := queuedStack(t, f, t.TempDir())
		s.prs[1].State, s.prs[2].State = "MERGED", "MERGED"
		s.fail = "gt sync"
		got := streamRounds(t, f, 1, func(int) {})
		if !strings.Contains(got, "stack #2 landed (#1 #2)\n") || f.owned(t).Queued != nil {
			t.Fatalf("stream:\n%s", got)
		}
	})
	t.Run("a landed stack whose record cannot be written", func(t *testing.T) {
		t.Parallel()
		f := newFixture(t)
		s := queuedStack(t, f, "/w/40")
		s.prs[1].State, s.prs[2].State = "MERGED", "MERGED"
		freeze(t, f.Env(t).recordPath(40))
		got := streamRounds(t, f, 1, func(int) {})
		if !strings.Contains(got, "watch error: settle #2: ") {
			t.Fatalf("stream:\n%s", got)
		}
	})
	t.Run("an ejected stack whose record cannot be written", func(t *testing.T) {
		t.Parallel()
		f := newFixture(t)
		s := queuedStack(t, f, "/w/40")
		s.prs[2].Labels.Nodes = nil
		freeze(t, f.Env(t).recordPath(40))
		got := streamRounds(t, f, 2, func(int) {})
		if !strings.Contains(got, "watch error: eject #2: ") || strings.Contains(got, "stack #2 ejected") {
			t.Fatalf("stream:\n%s", got)
		}
	})
}

func TestWatchStream_anEjectedStackPrintsOneFreshOwnerBlock(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	s := queuedStack(t, f, "/w/40")
	got := streamRounds(t, f, 5, func(round int) {
		if round == 1 {
			s.prs[2].Labels.Nodes = nil
			f.hub.on(graphqlRoute, failureData(
				watchNode(1, "fb", rollup(greenOK), ""),
				watchNode(2, "b1", rollup(greenOK), dropped(f.now.Add(time.Minute))),
			))
		}
	})
	if strings.Count(got, "  fresh owner\n") != 1 ||
		!strings.Contains(got, "stack #2 ejected: #2 left the Graphite merge queue") {
		t.Fatalf("stream:\n%s", got)
	}
}

func TestWatchStream_rereadsTheTrunkEachRoundForASquashNotYetVisible(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	s := queuedStack(t, f, t.TempDir())
	for _, n := range []int{1, 2} {
		s.prs[n].State, s.prs[n].ClosedAt = "CLOSED", f.now.Add(-time.Minute)
	}
	commits := list("/commits?sha=fb&since=2026-09-27T10:59:00Z")
	f.hub.on(commits, `[]`)
	f.hub.on(get("/compare/fb...b1-oid"), `{"status":"diverged"}`)
	f.hub.on(get("/compare/fb...b2-oid"), `{"status":"diverged"}`)
	got := streamRounds(t, f, 3, func(round int) {
		if round == 1 {
			f.hub.on(commits, `[{"commit":{"message":"A (#1)"}},{"commit":{"message":"B (#2)"}}]`)
		}
	})
	want := "#1 ejected\n#2 ejected\n#1 landed\n#2 landed\nstack #2 landed (#1 #2)\n"
	if got != want {
		t.Fatalf("stream\n got %q\nwant %q", got, want)
	}
}

func armedWatch(t *testing.T, f *fixture) *stackGH {
	t.Helper()
	s := newStackGH(t, f, green(t, 1, "b1", "fb"), stackOf(t, 2, "b2", "b1", "pending", "SUCCESS"))
	f.owner(t, Record{Ticket: 40, State: Exited, Worktree: "/w/40", Armed: &Arm{Top: 2, PRs: []int{1, 2}}})
	f.noFailures()
	return s
}

func TestWatchStream_landsAnArmedStackOnceStage1Passes(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	s := armedWatch(t, f)
	got := streamRounds(t, f, 3, func(round int) {
		if round == 1 {
			*s.prs[2] = *green(t, 2, "b2", "b1")
		}
	})
	want := "armed stack #2 landing\nqueued #1 #2\nfollow it: monacoctl agents watch (under Claude Code's Monitor tool)\n" +
		"queued together: #1 #2\n#1 queued\n#2 queued\n"
	if got != want {
		t.Fatalf("stream\n got %q\nwant %q", got, want)
	}
	if calls := f.hub.callsContaining("/labels"); !slices.Equal(calls, []string{
		"POST /repos/o/r/issues/1/labels", "POST /repos/o/r/issues/2/labels",
	}) {
		t.Fatalf("labels %v", calls)
	}
	if r := f.owned(t); r.Armed != nil || r.Queued == nil || !slices.Equal(r.Queued.PRs, []int{1, 2}) {
		t.Fatalf("queued %+v armed %+v", r.Queued, r.Armed)
	}
}

func TestWatchStream_disarmsAnArmedStackWhoseStage1FailsOnce(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	s := armedWatch(t, f)
	got := streamRounds(t, f, 4, func(round int) {
		if round == 1 {
			*s.prs[2] = *stackOf(t, 2, "b2", "b1", "FAILURE", "SUCCESS")
		}
	})
	if want := "armed stack #2 disarmed: #2 stage 1 failed\n"; got != want {
		t.Fatalf("stream\n got %q\nwant %q", got, want)
	}
	if calls := f.hub.callsContaining("/labels"); len(calls) != 0 {
		t.Fatalf("labels %v", calls)
	}
	if r := f.owned(t); r.Armed != nil || r.Queued != nil {
		t.Fatalf("queued %+v armed %+v", r.Queued, r.Armed)
	}
}

func TestWatchStream_disarmsAnArmedStackThatCannotLand(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	s := armedWatch(t, f)
	s.git["merge-tree"] = errors.New("merge-tree: boom")
	got := streamRounds(t, f, 3, func(round int) {
		if round == 1 {
			*s.prs[2] = *green(t, 2, "b2", "b1")
		}
	})
	if want := "armed stack #2 landing\narmed stack #2 disarmed: merge-tree: boom; the stack is not marked queued: " +
		"run land-stack again, which relabels every PR\n"; got != want {
		t.Fatalf("stream %q", got)
	}
	if r := f.owned(t); r.Armed != nil || r.Queued != nil {
		t.Fatalf("queued %+v armed %+v", r.Queued, r.Armed)
	}
}

func TestArm_failures(t *testing.T) {
	t.Parallel()
	t.Run("land-stack cannot write the arm", func(t *testing.T) {
		t.Parallel()
		f := newFixture(t)
		armedStack(t, f)
		freeze(t, f.Env(t).recordPath(40))
		code, _, stderr := f.agents(t, "land-stack", "2")
		if code != 1 || !strings.Contains(stderr, "write owner record") {
			t.Fatalf("%d %q", code, stderr)
		}
	})
	t.Run("watch cannot write the disarm", func(t *testing.T) {
		t.Parallel()
		f := newFixture(t)
		s := armedWatch(t, f)
		*s.prs[2] = *stackOf(t, 2, "b2", "b1", "FAILURE", "SUCCESS")
		freeze(t, f.Env(t).recordPath(40))
		got := streamRounds(t, f, 1, func(int) {})
		if !strings.HasPrefix(got, "watch error: disarm #2: write owner record: ") || strings.Count(got, "\n") != 1 {
			t.Fatalf("stream %q", got)
		}
	})
	t.Run("watch cannot read the armed stack", func(t *testing.T) {
		t.Parallel()
		f := newFixture(t)
		s := armedWatch(t, f)
		s.prs[2].State = "CLOSED"
		got := streamRounds(t, f, 1, func(int) {})
		if got != "watch error: armed stack #2: #2 is not an open PR\n" || f.owned(t).Armed == nil {
			t.Fatalf("stream %q", got)
		}
	})
	t.Run("dequeue cannot write the disarm", func(t *testing.T) {
		t.Parallel()
		f := newFixture(t)
		armedWatch(t, f)
		freeze(t, f.Env(t).recordPath(40))
		var out strings.Builder
		err := dequeueCmd(t.Context(), f.Env(t), []string{"2"}, &out)
		if err == nil || !strings.Contains(err.Error(), "write owner record") || out.Len() != 0 {
			t.Fatalf("%v %q", err, out.String())
		}
	})
}

func TestWatchStream_everyStreamReportsAStackThatAnotherStreamSettled(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		line string
		end  func(s *stackGH)
	}{
		{"landed", "stack #2 landed (#1 #2)", func(s *stackGH) { s.prs[1].State, s.prs[2].State = "MERGED", "CLOSED" }},
		{"ejected", "stack #2 ejected: #2 left the Graphite merge queue", func(s *stackGH) { s.prs[2].Labels.Nodes = nil }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			f := newFixture(t)
			s := queuedStack(t, f, t.TempDir())
			f.hub.on(get("/compare/fb...b2-oid"), `{"status":"identical"}`)
			a, b := newStream(f.Env(t)), newStream(f.Env(t))
			a.next(t.Context())
			b.next(t.Context())
			tc.end(s)
			settled, other := make([]string, 0, 16), make([]string, 0, 16)
			for range 3 {
				f.now = f.now.Add(time.Minute)
				settled = append(settled, a.next(t.Context())...)
				other = append(other, b.next(t.Context())...)
			}
			if f.owned(t).Queued != nil || f.owned(t).Settled == nil {
				t.Fatalf("record %+v", f.owned(t))
			}
			if n := slices.Index(settled, tc.line); n < 0 || slices.Contains(settled[n+1:], tc.line) {
				t.Fatalf("settling stream: %q", settled)
			}
			if n := slices.Index(other, tc.line); n < 0 || slices.Contains(other[n+1:], tc.line) {
				t.Fatalf("other stream: %q", other)
			}
		})
	}
}

func TestWatchStream_aStreamStartedAfterASettlementPrintsNothingForIt(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	settled := &Settlement{
		Top: 2, PRs: []int{1, 2}, Outcome: outcomeLanded, Detail: "stack #2 landed (#1 #2)",
		At: f.now.Add(-time.Second),
	}
	f.owner(t, Record{Ticket: 40, State: Exited, Worktree: "/w/40", Settled: settled})
	f.noFailures()
	s := newStream(f.Env(t))
	f.now = f.now.Add(time.Minute)
	if got := append(s.next(t.Context()), s.next(t.Context())...); slices.Contains(got, settled.Detail) {
		t.Fatalf("stream: %q", got)
	}
}

func TestWatchStream_aDroppedLabelIsNotReportedWhileAnOpenDraftListsThePR(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	f.owner(t, Record{Ticket: 40, State: Exited, Worktree: "/w/40"})
	node := watchNode(7, "fb", rollup(greenOK), dropped(f.now.Add(time.Minute)))
	draft := queueDraftNode(90, "[Graphite MQ] Draft PR GROUP:spec_1 (PRs 7)", rollup(greenOK))
	f.hub.on(graphqlRoute, draftData([]string{draft}, node))
	s := newStream(f.Env(t))
	f.now = f.now.Add(2 * time.Minute)
	if got := strings.Join(s.next(t.Context()), "\n"); strings.Contains(got, "#7 dropped") {
		t.Fatalf("stream:\n%s", got)
	}
	f.hub.on(graphqlRoute, draftData([]string{strings.Replace(draft, `"OPEN"`, `"CLOSED"`, 1)}, node))
	if got := strings.Join(
		s.next(t.Context()),
		"\n",
	); !strings.Contains(
		got,
		"#7 dropped from the Graphite merge queue",
	) {
		t.Fatalf("no drop once the draft closed:\n%s", got)
	}
}
