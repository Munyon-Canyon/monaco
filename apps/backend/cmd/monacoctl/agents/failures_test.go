package agents

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

const (
	flakeJob   = `{"name":"ci / Flake","conclusion":"FAILURE","databaseId":11,"detailsUrl":"https://gh/job/11"}`
	lintJob    = `{"name":"ci / Lint","conclusion":"TIMED_OUT","databaseId":12,"detailsUrl":"https://gh/job/12"}`
	okJob      = `{"name":"ci / Lint","conclusion":"SUCCESS","databaseId":13,"detailsUrl":"https://gh/job/13"}`
	redOK      = `{"name":"ci / ci-ok","conclusion":"FAILURE","databaseId":14,"detailsUrl":"https://gh/job/14"}`
	goCacheJob = `{"name":"Build apps/backend and save the Go cache","conclusion":"FAILURE","databaseId":16}`
	readyJob   = `{"name":"ci / Ready (staging)","conclusion":"FAILURE","databaseId":17}`
	greenOK    = `{"name":"ci / ci-ok","conclusion":"SUCCESS","databaseId":15,"detailsUrl":"https://gh/job/15"}`
	noRollup   = `{"statusCheckRollup":null}`
)

func rollup(runs ...string) string {
	return `{"statusCheckRollup":{"contexts":{"nodes":[` + strings.Join(runs, ",") + `]}}}`
}

func watchNode(n int, base, head, removals string) string {
	return fmt.Sprintf(
		`{"number":%d,"body":"Part of #40","headRefName":"b%d","baseRefName":%q,"headRefOid":"sha%d",`+
			`"commits":{"nodes":[{"commit":%s}]},"timelineItems":{"nodes":[%s]}}`,
		n, n, base, n, head, removals,
	)
}

const (
	graphiteBot = "graphite-app[bot]"
	graphiteApp = "graphite-app"
)

func unlabel(at time.Time, label, actor string) string {
	return fmt.Sprintf(`{"createdAt":%q,"label":{"name":%q},"actor":{"login":%q}}`,
		at.Format(time.RFC3339), label, actor)
}

func dropped(at time.Time) string { return unlabel(at, "merge-queue", graphiteBot) }

func draftNode(head, title string, at time.Time, commit string) string {
	return fmt.Sprintf(
		`{"title":%q,"body":"","headRefName":%q,"updatedAt":%q,"closedAt":%q,"commits":{"nodes":[{"commit":%s}]}}`,
		title, head, at.Format(time.RFC3339), at.Format(time.RFC3339), commit)
}

func movedUpdate(t *testing.T, node string, closed, updated time.Time) string {
	t.Helper()
	from := fmt.Sprintf(`"updatedAt":%q`, closed.Format(time.RFC3339))
	if !strings.Contains(node, from) {
		t.Fatalf("draftNode no longer carries %s", from)
	}
	return strings.Replace(node, from, fmt.Sprintf(`"updatedAt":%q`, updated.Format(time.RFC3339)), 1)
}

func TestFailures_parsesQueueRemovalsAndRedStage1(t *testing.T) {
	t.Parallel()
	since := time.Date(2026, 9, 29, 6, 0, 0, 0, time.UTC)
	after, before := since.Add(time.Minute), since.Add(-time.Minute)
	ran := after.Add(30 * time.Second)
	waiting, lapsed := after.Add(time.Minute), after.Add(takenFor)
	type want struct {
		why string
		job int64
	}
	for _, tc := range []struct {
		name   string
		nodes  []string
		want   []want
		drafts []string
		now    time.Time
	}{
		{
			"Graphite dropping the label names the failing job of its gtmq_ draft, not ci-ok",
			[]string{watchNode(1, "fb", rollup(greenOK), dropped(after))},
			[]want{{"dropped from the Graphite merge queue", 11}},
			[]string{
				draftNode("gtmq_old", "Merge queue: #1", before, rollup(lintJob)),
				draftNode("gtmq_1", "Merge queue: #1", ran, rollup(okJob, redOK, flakeJob)),
			},
			waiting,
		},
		{
			"without a draft for the PR the PR head's failing job is named once the hold has run out",
			[]string{watchNode(1, "fb", rollup(greenOK, lintJob), dropped(after))},
			[]want{{"dropped from the Graphite merge queue", 12}},
			[]string{
				draftNode("gtmq_12", "Merge queue: #12", ran, rollup(flakeJob)),
				draftNode("other", "Fix #1", ran, rollup(flakeJob)),
				draftNode("gtmq_1", "Merge queue: #1", before, rollup(flakeJob)),
			},
			lapsed,
		},
		{
			"a take whose hold ended before the last run is not news",
			[]string{watchNode(1, "fb", rollup(greenOK), dropped(since.Add(-takenFor-time.Minute)))},
			nil, nil, lapsed,
		},
		{
			"a take before the last run is a drop once its hold runs out",
			[]string{watchNode(1, "fb", noRollup, dropped(before))},
			[]want{{"dropped from the Graphite merge queue", 0}},
			nil, before.Add(takenFor),
		},
		{
			"a take before the last run is no drop while its hold runs",
			[]string{watchNode(1, "fb", noRollup, dropped(before))},
			nil, nil, waiting,
		},
		{
			"a PR that a closed draft ran since the last run is a drop, with that draft's failing job",
			[]string{watchNode(1, "fb", rollup(greenOK), dropped(before))},
			[]want{{"dropped from the Graphite merge queue", 11}},
			[]string{draftNode("gtmq_1", "Merge queue: #1", ran, rollup(flakeJob))},
			waiting,
		},
		{
			"a PR that a closed draft ran before the last run is not news",
			[]string{watchNode(1, "fb", rollup(greenOK), dropped(before))},
			nil,
			[]string{draftNode("gtmq_1", "Merge queue: #1", since.Add(-30*time.Second), rollup(flakeJob))},
			waiting,
		},
		{
			"the newest closed draft that ran the PR decides when it is listed first",
			[]string{watchNode(1, "fb", rollup(greenOK), dropped(before.Add(-time.Minute)))},
			[]want{{"dropped from the Graphite merge queue", 11}},
			[]string{
				draftNode("gtmq_1", "Merge queue: #1", ran, rollup(flakeJob)),
				draftNode("gtmq_1", "Merge queue: #1", since.Add(-30*time.Second), rollup(lintJob)),
			},
			waiting,
		},
		{
			"the newest closed draft that ran the PR decides when it is listed last",
			[]string{watchNode(1, "fb", rollup(greenOK), dropped(before.Add(-time.Minute)))},
			[]want{{"dropped from the Graphite merge queue", 11}},
			[]string{
				draftNode("gtmq_1", "Merge queue: #1", since.Add(-30*time.Second), rollup(lintJob)),
				draftNode("gtmq_1", "Merge queue: #1", ran, rollup(flakeJob)),
			},
			waiting,
		},
		{
			"the draft that closed last decides when deleting an older draft's branch moved its updatedAt past it",
			[]string{watchNode(1, "fb", rollup(greenOK), dropped(before.Add(-time.Minute)))},
			[]want{{"dropped from the Graphite merge queue", 11}},
			[]string{
				draftNode("gtmq_1", "Merge queue: #1", ran, rollup(flakeJob)),
				movedUpdate(t, draftNode("gtmq_1", "Merge queue: #1", after.Add(10*time.Second), rollup(lintJob)),
					after.Add(10*time.Second), after.Add(50*time.Second)),
			},
			waiting,
		},
		{
			"Graphite taking the PR is no drop while its draft has not run it",
			[]string{watchNode(1, "fb", rollup(greenOK), dropped(after))},
			nil, nil, waiting,
		},
		{
			"Graphite's GraphQL login is Graphite too",
			[]string{watchNode(1, "fb", rollup(greenOK), unlabel(after, "merge-queue", graphiteApp))},
			nil, nil, waiting,
		},
		{
			"a take just inside the hold is no drop",
			[]string{watchNode(1, "fb", rollup(greenOK), dropped(after))},
			nil, nil, lapsed.Add(-time.Second),
		},
		{
			"a take a whole hold ago with no draft is a drop",
			[]string{watchNode(1, "fb", noRollup, dropped(after))},
			[]want{{"dropped from the Graphite merge queue", 0}},
			nil, lapsed,
		},
		{
			"an open draft that tests the PR holds it past the hold",
			[]string{watchNode(1, "fb", rollup(greenOK), dropped(after))},
			nil,
			[]string{strings.Replace(draftNode("gtmq_1", "Merge queue: #1", ran, noRollup), `{"title"`, `{"state":"OPEN","title"`, 1)},
			lapsed,
		},
		{
			"a closed draft that ran other PRs does not end the hold",
			[]string{watchNode(1, "fb", rollup(greenOK), dropped(after))},
			nil,
			[]string{draftNode("gtmq_12", "Merge queue: #12", ran, rollup(flakeJob))},
			waiting,
		},
		{
			"a closed draft that ran the PR before Graphite took it does not end the hold",
			[]string{watchNode(1, "fb", rollup(greenOK), dropped(after))},
			nil,
			[]string{draftNode("gtmq_1", "Merge queue: #1", since.Add(30*time.Second), rollup(flakeJob))},
			waiting,
		},
		{
			"deleting that draft's branch after the take does not make it a run since the take",
			[]string{watchNode(1, "fb", rollup(greenOK), dropped(after))},
			nil,
			[]string{movedUpdate(t,
				draftNode("gtmq_1", "Merge queue: #1", since.Add(30*time.Second), rollup(flakeJob)),
				since.Add(30*time.Second), after.Add(time.Second))},
			waiting,
		},
		{
			"a person removing the label or Graphite removing another label is no failure",
			[]string{
				watchNode(1, "fb", rollup(greenOK), unlabel(after, "merge-queue", "logan")),
				watchNode(2, "fb", rollup(greenOK), unlabel(after, "large-pr", "Graphite-App")),
			},
			nil, nil, lapsed,
		},
		{
			"only the latest removal of the queue label counts",
			[]string{watchNode(1, "fb", noRollup,
				dropped(after)+","+unlabel(after, "merge-queue", "logan")+","+unlabel(after, "large-pr", graphiteBot))},
			nil, nil, lapsed,
		},
		{
			"a red stage 1 names its failing job every run",
			[]string{watchNode(1, "fb", rollup(redOK, lintJob), dropped(before))},
			[]want{{"stage 1 is red", 12}},
			nil, waiting,
		},
		{
			"a red ci-ok alone names ci-ok",
			[]string{watchNode(1, "fb", rollup(okJob, redOK), "")},
			[]want{{"stage 1 is red", 14}},
			nil, waiting,
		},
		{
			"a drop with no failed run has no job",
			[]string{watchNode(1, "fb", noRollup, unlabel(after, "merge-queue", "Graphite-App"))},
			[]want{{"dropped from the Graphite merge queue", 0}},
			[]string{draftNode("gtmq_1", "#1", ran, noRollup)},
			waiting,
		},
		{
			"green, pending and unrelated failures stay quiet",
			[]string{
				watchNode(1, "fb", rollup(greenOK, flakeJob), ""),
				watchNode(2, "fb", rollup(`{"name":"ci / ci-ok","conclusion":""}`), ""),
				watchNode(3, "fb", noRollup, ""),
			},
			nil, nil, waiting,
		},
		{
			"a stacked PR counts and a PR off the feature branch does not",
			[]string{
				watchNode(1, "fb", rollup(greenOK), ""),
				watchNode(2, "b1", rollup(redOK), ""),
				watchNode(3, "main", rollup(redOK), ""),
			},
			[]want{{"stage 1 is red", 14}},
			nil, waiting,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var prs []watchPR
			if err := json.Unmarshal([]byte("["+strings.Join(tc.nodes, ",")+"]"), &prs); err != nil {
				t.Fatal(err)
			}
			var drafts []queueDraft
			if err := json.Unmarshal([]byte("["+strings.Join(tc.drafts, ",")+"]"), &drafts); err != nil {
				t.Fatal(err)
			}
			got := make([]want, 0, len(tc.want))
			for _, f := range failures(prs, queueRuns{label: "merge-queue", drafts: drafts, now: tc.now}, "fb", since) {
				got = append(got, want{f.Why, f.Job.DatabaseID})
			}
			if !slices.Equal(got, tc.want) {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
		})
	}
}

func TestWatch_readsEveryCheckAndTheNewestRunOfEach(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	f.hub.on(graphqlRoute, failureData(
		watchNode(5, "fb", firstPage("h5", ciOK("FAILURE", 1), flakeJob), ""),
		watchNode(6, "fb", firstPage("h6", ciOK("SUCCESS", 1)), ""),
	))
	f.hub.onQuery(`c1: object(oid:\"h6\")`, `{"data":{"repository":{`+
		`"c0":`+rollup(ciOK("SUCCESS", 3), `{"name":"ci / Flake","conclusion":"SUCCESS","completedAt":"2026-09-29T11:03:00Z"}`)+
		`,"c1":`+rollup(ciOK("FAILURE", 3), lintJob)+`}}}`)
	failed, err := f.failures(t)
	if err != nil || len(failed) != 1 || failed[0].PR != 6 || failed[0].Job.DatabaseID != 12 {
		t.Fatalf("%+v %v", failed, err)
	}
	f.hub.onQuery(`c1: object(oid:\"h6\")`, `{"data":null,"errors":[{"message":"rate limited"}]}`)
	if _, err := f.failures(t); cliText(err) != "graphql: rate limited" {
		t.Fatal(err)
	}
}

func failureData(nodes ...string) string {
	return draftData(nil, nodes...)
}

func draftData(drafts []string, nodes ...string) string {
	return `{"data":{"repository":{"pullRequests":{"nodes":[` + strings.Join(nodes, ",") + `]},` +
		`"drafts":{"nodes":[` + strings.Join(drafts, ",") + `]}}}}`
}

func (f *fixture) failures(t *testing.T) ([]failure, error) {
	t.Helper()
	env := f.Env(t)
	data, err := env.watchData(context.Background())
	if err != nil {
		return nil, err
	}
	return env.failures(context.Background(), nil, data)
}

func (f *fixture) noFailures() { f.hub.on(graphqlRoute, failureData()) }

func TestWatch_printsAFreshOwnerPromptForAnEjectedEntryOnce(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	f.owner(t, Record{Ticket: 40, State: Exited, Worktree: "/wt/40"})
	f.hub.on(graphqlRoute, draftData(
		[]string{draftNode("gtmq_5", "Merge queue: #5", f.now.Add(-time.Minute), rollup(flakeJob))},
		watchNode(5, "fb", rollup(greenOK), dropped(f.now.Add(-3*time.Minute))),
		strings.Replace(watchNode(6, "fb", rollup(redOK, lintJob), ""), "Part of #40", "no ticket", 1),
	))
	f.hub.on(get("/actions/jobs/11/logs"), "--- FAIL: TestFlaky\n")
	code, stdout, stderr := f.agents(t, "watch", "--once", "--verbose")
	logPath := f.Env(t).statePath("logs", "job-11.log")
	want := "#5 dropped from the Graphite merge queue\n  failing job: https://gh/job/11\n  fresh owner\n" +
		"  ticket: 40\n  worktree: /wt/40\n  head: sha5\n  log: " + logPath + "\n  brief: docs/agents/owner.md\n" +
		"#6 stage 1 is red\n  failing job: https://gh/job/12\n  fresh owner\n" +
		"  ticket: unknown\n  worktree: unknown\n  head: sha6\n  log: unavailable ("
	if code != 1 || stderr != "" || !strings.HasPrefix(stdout, want) {
		t.Fatalf("code=%d stderr=%q stdout=\n%s", code, stderr, stdout)
	}
	if b, err := os.ReadFile(logPath); err != nil || string(b) != "--- FAIL: TestFlaky\n" {
		t.Fatalf("log %q %v", b, err)
	}
	f.now = f.now.Add(time.Minute)
	_, stdout, _ = f.agents(t, "watch", "--once")
	if strings.Contains(stdout, "#5") || !strings.Contains(stdout, "#6 stage 1 is red") {
		t.Fatalf("second run:\n%s", stdout)
	}
}

func TestFailures_watchOnceLeavesAPRGraphiteTookWhileItWaitsForADraft(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		by   string
		ago  time.Duration
		want string
	}{
		{"Graphite took the label two minutes ago", graphiteApp, 2 * time.Minute, ""},
		{"Graphite's REST login", graphiteBot, 2 * time.Minute, ""},
		{"Graphite took it a whole hold ago", graphiteApp, takenFor, "#5 dropped from the Graphite merge queue\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			f := newFixture(t)
			f.hub.on(graphqlRoute, failureData(
				watchNode(5, "fb", rollup(greenOK), unlabel(f.now.Add(-tc.ago), "merge-queue", tc.by))))
			code, stdout, stderr := f.agents(t, "watch", "--once")
			if tc.want == "" && (code != 0 || stdout != "" || stderr != "") ||
				tc.want != "" && (code != 1 || !strings.HasPrefix(stdout, tc.want)) {
				t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout, stderr)
			}
		})
	}
}

func closedDraftWith(head, title string, at time.Time, state, oid, commit string) string {
	node := draftNode(head, title, at, commit)
	return strings.Replace(node, `{"title"`, fmt.Sprintf(`{"state":%q,"headRefOid":%q,"title"`, state, oid), 1)
}

func closedDraftNode(head, title string, at time.Time, state, oid string) string {
	return closedDraftWith(head, title, at, state, oid, rollup(greenOK))
}

func TestWatchOnce_reportsAGraphiteDropOnceWhenItsHoldRunsOut(t *testing.T) {
	t.Parallel()
	const (
		every    = 5 * time.Minute
		squash   = `[{"commit":{"message":"A (#5)"}}]`
		onTrunk  = `{"status":"behind"}`
		offTrunk = `{"status":"diverged"}`
	)
	for _, tc := range []struct {
		name                 string
		opens, closes, moved time.Duration
		state                string
		trunk, compare       string
		want                 []time.Duration
		reads                int
	}{
		{name: "no draft ever opens", want: []time.Duration{30 * time.Minute}},
		{
			name:  "a draft ran it and failed, leaving nothing of it on the trunk",
			opens: 5 * time.Minute, closes: 25 * time.Minute, state: "CLOSED", compare: offTrunk,
			want: []time.Duration{25 * time.Minute}, reads: 1,
		},
		{
			name:  "Graphite deletes the failed draft's branch a minute after closing it, which moves its updatedAt",
			opens: 5 * time.Minute, closes: 25 * time.Minute, moved: time.Minute, state: "CLOSED", compare: offTrunk,
			want: []time.Duration{25 * time.Minute}, reads: 1,
		},
		{
			name:  "a draft ran it and merged",
			opens: 5 * time.Minute, closes: 25 * time.Minute, state: "MERGED", reads: 1,
		},
		{
			name:  "a draft ran it and closed with the PR's squash commit on the trunk",
			opens: 5 * time.Minute, closes: 25 * time.Minute, state: "CLOSED", trunk: squash, reads: 1,
		},
		{
			name:  "a draft ran it and closed with its head on the trunk",
			opens: 5 * time.Minute, closes: 25 * time.Minute, state: "CLOSED", compare: onTrunk, reads: 1,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			f := newFixture(t)
			first := f.now
			if tc.trunk != "" {
				f.hub.on(list("/commits?sha=fb&since=2026-09-27T10:58:00Z"), tc.trunk)
			}
			if tc.compare != "" {
				f.hub.on(get("/compare/fb...d5"), tc.compare)
			}
			var reported []time.Duration
			for offset := time.Duration(0); offset <= time.Hour; offset += every {
				f.now = first.Add(offset)
				var drafts []string
				switch {
				case tc.opens == 0 || offset < tc.opens:
				case offset < tc.closes:
					open := draftNode("gtmq_5", "Merge queue: #5", first, noRollup)
					drafts = []string{strings.Replace(open, `{"title"`, `{"state":"OPEN","title"`, 1)}
				default:
					closed := first.Add(tc.closes)
					node := closedDraftNode("gtmq_5", "Merge queue: #5", closed, tc.state, "d5")
					if offset >= tc.closes+tc.moved {
						node = movedUpdate(t, node, closed, closed.Add(tc.moved))
					}
					drafts = []string{node}
				}
				f.hub.on(graphqlRoute, draftData(drafts,
					watchNode(5, "fb", rollup(greenOK), dropped(first.Add(-2*time.Minute)))))
				code, stdout, stderr := f.agents(t, "watch", "--once")
				if strings.Contains(stdout, "#5 dropped from the Graphite merge queue\n") {
					reported = append(reported, offset)
					continue
				}
				if code != 0 || stdout != "" || stderr != "" {
					t.Fatalf("after %s: code=%d stdout=%q stderr=%q", offset, code, stdout, stderr)
				}
			}
			if !slices.Equal(reported, tc.want) {
				t.Fatalf("reported after %v, want after %v", reported, tc.want)
			}
			if got := len(f.hub.callsContaining("/commits?sha=")); got != tc.reads {
				t.Fatalf("read the trunk %d times, want %d: %v", got, tc.reads, f.hub.callsContaining("/commits?sha="))
			}
		})
	}
}

func TestWatchOnce_aFailedTrunkReadFailsThePassAndKeepsTheDropForTheNextOne(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	f.hub.on(graphqlRoute, draftData(
		[]string{closedDraftNode("gtmq_5", "Merge queue: #5", f.now.Add(-time.Minute), "CLOSED", "d5")},
		watchNode(5, "fb", rollup(greenOK), dropped(f.now.Add(-5*time.Minute))),
	))
	f.hub.on(get("/compare/fb...d5"), `{"status":"diverged"}`)
	route := list("/commits?sha=fb&since=2026-09-27T10:55:00Z")
	f.hub.status[route] = http.StatusInternalServerError
	f.hub.on(route, "boom")
	code, stdout, stderr := f.agents(t, "watch", "--once")
	if code != 1 || !strings.Contains(stderr, "list fb commits") || strings.Contains(stdout, "dropped") {
		t.Fatalf("a failed read: code=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
	delete(f.hub.status, route)
	f.hub.on(route, `[]`)
	code, stdout, stderr = f.agents(t, "watch", "--once")
	if code != 1 || !strings.HasPrefix(stdout, "#5 dropped from the Graphite merge queue\n") || stderr != "" {
		t.Fatalf("the next pass: code=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
}

func TestWatchOnce_aDraftThatClosesWhileTheWatchReadsGitHubIsReportedByTheNextPass(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	node := watchNode(5, "fb", rollup(greenOK), dropped(f.now.Add(-5*time.Minute)))
	f.hub.on(graphqlRoute, failureData(node))
	f.hub.on(get("/compare/fb...d5"), `{"status":"diverged"}`)
	advanced := false
	f.hub.hook = func(method, _, _ string, _ int) {
		if method == http.MethodPost && !advanced {
			advanced = true
			f.now = f.now.Add(10 * time.Second)
		}
	}
	closedAt := f.now.Add(5 * time.Second)
	if code, stdout, stderr := f.agents(t, "watch", "--once"); code != 0 || stdout != "" || stderr != "" {
		t.Fatalf("the read: code=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
	f.hub.on(graphqlRoute, draftData(
		[]string{closedDraftNode("gtmq_5", "Merge queue: #5", closedAt, "CLOSED", "d5")}, node,
	))
	code, stdout, stderr := f.agents(t, "watch", "--once")
	if code != 1 || !strings.HasPrefix(stdout, "#5 dropped from the Graphite merge queue\n") || stderr != "" {
		t.Fatalf("the next pass: code=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
}

func TestWatchOnce_printsOneDropBlockForAnOwnedStackThatOneFailedDraftDropped(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	s := queuedStack(t, f, "/w/40")
	took := unlabel(f.now.Add(-5*time.Minute), "merge-queue", graphiteApp)
	setUnlabels(t, s.prs[1], took)
	setUnlabels(t, s.prs[2], took)
	failed := closedDraftWith("gtmq_1", "Merge queue: #1 #2", f.now.Add(-time.Minute), "CLOSED", "d1", rollup(flakeJob))
	f.hub.on(graphqlRoute, draftData([]string{failed},
		watchNode(1, "fb", rollup(greenOK), took),
		watchNode(2, "b1", rollup(greenOK), took),
	))
	f.hub.on(get("/compare/fb...d1"), `{"status":"diverged"}`)
	f.hub.on(get("/actions/jobs/11/logs"), "--- FAIL: TestFlaky\n")
	code, stdout, stderr := f.agents(t, "watch", "--once")
	want := "unqueued: #40; #2 left the Graphite merge queue. " +
		"Fix the stack with gt modify and gt submit --stack --draft, then run land-stack 2\n" +
		"#1 dropped from the Graphite merge queue\n  failing job: https://gh/job/11\n  fresh owner\n" +
		"  ticket: 40\n  worktree: /w/40\n  head: sha1\n  log: " + f.Env(t).statePath("logs", "job-11.log") +
		"\n  brief: docs/agents/owner.md\n"
	if code != 1 || stderr != "" || stdout != want {
		t.Fatalf("code=%d stderr=%q stdout=\n%s\nwant\n%s", code, stderr, stdout, want)
	}
	if f.owned(t).Queued != nil {
		t.Fatal("kept the queued mark")
	}
}

func TestFailureQuery_asksForTheHeadOfEveryDraft(t *testing.T) {
	t.Parallel()
	_, drafts, found := strings.Cut(failureQuery(""), "drafts:")
	if !found || !strings.Contains(drafts, "headRefOid") {
		t.Fatalf("the drafts selection lacks headRefOid:\n%s", drafts)
	}
}

func TestFailureQuery_asksForWhenEveryDraftClosed(t *testing.T) {
	t.Parallel()
	_, drafts, found := strings.Cut(failureQuery(""), "drafts:")
	if !found || !strings.Contains(drafts, " closedAt ") {
		t.Fatalf("the drafts selection lacks closedAt:\n%s", drafts)
	}
}

func TestOnePerOwnedStack_keepsTheLowestDropOfEachStackAndEachDraft(t *testing.T) {
	t.Parallel()
	rs := []Record{
		{Ticket: 40, Queued: &Queue{Top: 3, PRs: []int{1, 2, 3}}},
		{Ticket: 41, Armed: &Arm{Top: 6, PRs: []int{5, 6}}},
	}
	drop := func(pr int, job int64) failure {
		return failure{PR: pr, Why: droppedWhy, Job: gqlContext{DatabaseID: job}}
	}
	red := failure{PR: 3, Why: "stage 1 is red", Job: gqlContext{DatabaseID: 11}}
	for _, tc := range []struct {
		name     string
		in, want []failure
	}{
		{
			"the lowest dropped PR speaks for its stack",
			[]failure{drop(3, 11), drop(1, 11), drop(2, 11)},
			[]failure{drop(1, 11)},
		},
		{
			"the lowest PR that dropped speaks when a lower one of the stack did not",
			[]failure{drop(3, 11), drop(2, 11)},
			[]failure{drop(2, 11)},
		},
		{
			"a PR that another draft dropped is its own report",
			[]failure{drop(1, 11), drop(2, 12), drop(3, 12)},
			[]failure{drop(1, 11), drop(2, 12)},
		},
		{
			"each owned stack keeps its own, armed or queued",
			[]failure{drop(2, 11), drop(5, 11), drop(1, 11), drop(6, 11)},
			[]failure{drop(5, 11), drop(1, 11)},
		},
		{
			"a red stage 1 and the drops of PRs no record covers stay",
			[]failure{drop(1, 11), red, drop(2, 11), drop(9, 11), drop(8, 11)},
			[]failure{drop(1, 11), red, drop(9, 11), drop(8, 11)},
		},
		{"no failure stays none", nil, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := onePerOwnedStack(tc.in, rs); !slices.Equal(got, tc.want) {
				t.Fatalf("got %+v, want %+v", got, tc.want)
			}
		})
	}
}

func TestWatch_reportsARedPROfAnotherRootsTicketWithoutRebuildingItsRecord(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	f.ownerComments(40, ownerComment(2, Record{Ticket: 40, Model: opus, State: Running}))
	f.hub.on(graphqlRoute, failureData(watchNode(5, "fb", rollup(redOK), "")))
	code, stdout, stderr := f.agents(t, "watch", "--once", "--verbose")
	want := "#5 stage 1 is red\n  failing job: https://gh/job/14\n  fresh owner\n" +
		"  ticket: 40\n  worktree: unknown\n  head: sha5\n"
	if code != 1 || stderr != "" || !strings.HasPrefix(stdout, want) {
		t.Errorf("code=%d stderr=%q stdout=\n%s", code, stderr, stdout)
	}
	kept, err := os.ReadDir(filepath.Join(f.Env(t).Common, recordsDir))
	if err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	for _, e := range kept {
		t.Errorf("watch wrote %s for a ticket this clone has no record of", e.Name())
	}
}

func TestWatch_failuresSurfaceStateQueryAndRecordErrors(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	env := f.Env(t)
	state := env.statePath("watch", lastRunState)
	writeFile(t, state, "yesterday\n")
	f.noFailures()
	read := func() error {
		_, err := f.failures(t)
		return err
	}
	if err := read(); err == nil || !strings.Contains(err.Error(), "parse watch") {
		t.Fatalf("corrupt state: %v", err)
	}
	if err := os.Remove(state); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(state, 0o750); err != nil {
		t.Fatal(err)
	}
	if _, err := env.lastRun(); err == nil || !strings.Contains(err.Error(), "read watch") {
		t.Fatalf("unreadable state: %v", err)
	}
	if err := os.Remove(state); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(t.TempDir(), "missing", "last-run"), state); err != nil {
		t.Fatal(err)
	}
	f.noFailures()
	if err := read(); err == nil || !strings.Contains(err.Error(), "write") {
		t.Fatalf("state write: %v", err)
	}
	if err := os.Remove(state); err != nil {
		t.Fatal(err)
	}
	f.hub.on(graphqlRoute, `{"data":null,"errors":[{"message":"rate limited"}]}`)
	if code, _, stderr := f.agents(t, "watch", "--once"); code != 1 || !strings.Contains(stderr, "rate limited") {
		t.Fatalf("graphql: %d %q", code, stderr)
	}
	if len(f.hub.callsContaining("/events")) != 0 || len(f.hub.callsContaining("/timeline")) != 0 {
		t.Fatalf("rest reads %v", f.hub.callsContaining("/issues/"))
	}
	writeFile(t, env.recordPath(40), "{")
	got := env.freshOwnerFor(context.Background(), failure{PR: 5, Body: "Part of #40"})
	if !strings.Contains(got, "ticket: 40\n  worktree: unknown\n") {
		t.Fatalf("unreadable record:\n%s", got)
	}
}

func TestJobLog_reportsATruncatedOrUnwritableLog(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	env := f.Env(t)
	short := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Length", "100")
		_, _ = io.WriteString(w, "cut")
	}))
	t.Cleanup(short.Close)
	env.GitHub.API = short.URL
	if got := env.jobLog(context.Background(), 1); !strings.HasPrefix(got, "unavailable (read GET") {
		t.Fatalf("truncated: %q", got)
	}
	f.hub.on(get("/actions/jobs/2/logs"), "log")
	env = f.Env(t)
	writeFile(t, env.statePath("logs", "job-2.log")+"/x", "")
	if got := env.jobLog(context.Background(), 2); !strings.HasPrefix(got, "unavailable (write") {
		t.Fatalf("unwritable: %q", got)
	}
}

func TestQueueJob_isEmptyWithNoDraftAndNoCommit(t *testing.T) {
	t.Parallel()
	if got := (watchPR{Number: 7}).queueJob(nil, time.Time{}); got != (gqlContext{}) {
		t.Fatalf("queueJob = %+v, want none", got)
	}
}

func TestStage1_aFailureFromACancelledRunWaitsForTheNewerRun(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name  string
		other gqlContext
		want  string
	}{
		{"a newer run's job is in progress", gqlContext{Name: "ci / Lint", Status: "IN_PROGRESS"}, "pending"},
		{"a newer run's job is queued", gqlContext{Name: "ci / Plan", Status: "QUEUED"}, "pending"},
		{"every ci job finished", gqlContext{Name: "ci / Lint", Status: "COMPLETED", Conclusion: "CANCELLED"}, "failure"},
		{"only a non-ci check is running", gqlContext{Name: formatCheck, Status: "IN_PROGRESS"}, "failure"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			p := stackOf(t, 1, "b1", "fb", "FAILURE", "")
			rollup := &p.Commits.Nodes[0].Commit.StatusCheckRollup.Contexts
			rollup.Nodes = append(rollup.Nodes, tc.other)
			if got := p.flat("").Stage1; got != tc.want {
				t.Fatalf("stage 1 = %q, want %q", got, tc.want)
			}
			if red := p.Commits.Nodes[0].Commit.stage1Red(); red != (tc.want == "failure") {
				t.Fatalf("stage1Red = %v", red)
			}
		})
	}
}

func TestStage1_aRunWhoseCIOKHasNotStartedIsPending(t *testing.T) {
	t.Parallel()
	p := stackOf(t, 1, "b1", "fb", "", "")
	rollup := &p.Commits.Nodes[0].Commit.StatusCheckRollup.Contexts
	if got := p.flat("").Stage1; got != "" {
		t.Fatalf("stage 1 with no ci jobs = %q, want missing", got)
	}
	rollup.Nodes = append(rollup.Nodes, gqlContext{Name: "ci / Plan", Status: "IN_PROGRESS"})
	if got := p.flat("").Stage1; got != "pending" {
		t.Fatalf("stage 1 = %q, want pending", got)
	}
}

func TestQueueJob_namesTheQueueCIJobNotAPushOnlyWorkflow(t *testing.T) {
	t.Parallel()
	raw := closedDraftWith("gtmq_4", "Merge queue: #4", time.Unix(1, 0), "CLOSED", "d4", rollup(goCacheJob, flakeJob))
	var d queueDraft
	if err := json.Unmarshal([]byte(raw), &d); err != nil {
		t.Fatal(err)
	}
	got := queueJob(4, lastCommits{}, []queueDraft{d}, time.Time{})
	if got.Name != "ci / Flake" {
		t.Fatalf("queueJob = %q, want ci / Flake", got.Name)
	}
}
