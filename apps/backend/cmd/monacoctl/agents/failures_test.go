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

const graphiteBot = "graphite-app[bot]"

func unlabel(at time.Time, label, actor string) string {
	return fmt.Sprintf(`{"createdAt":%q,"label":{"name":%q},"actor":{"login":%q}}`,
		at.Format(time.RFC3339), label, actor)
}

func dropped(at time.Time) string { return unlabel(at, "merge-queue", graphiteBot) }

func draftNode(head, title string, at time.Time, commit string) string {
	return fmt.Sprintf(`{"title":%q,"body":"","headRefName":%q,"updatedAt":%q,"commits":{"nodes":[{"commit":%s}]}}`,
		title, head, at.Format(time.RFC3339), commit)
}

func TestFailures_parsesQueueRemovalsAndRedStage1(t *testing.T) {
	t.Parallel()
	since := time.Date(2026, 9, 29, 6, 0, 0, 0, time.UTC)
	after, before := since.Add(time.Minute), since.Add(-time.Minute)
	type want struct {
		why string
		job int64
	}
	for _, tc := range []struct {
		name   string
		nodes  []string
		want   []want
		drafts []string
	}{
		{
			"Graphite dropping the label names the failing job of its gtmq_ draft, not ci-ok",
			[]string{watchNode(1, "fb", rollup(greenOK), dropped(after))},
			[]want{{"dropped from the Graphite merge queue", 11}},
			[]string{
				draftNode("gtmq_old", "Merge queue: #1", before, rollup(lintJob)),
				draftNode("gtmq_1", "Merge queue: #1", after, rollup(okJob, redOK, flakeJob)),
			},
		},
		{
			"without a draft for the PR the PR head's failing job is named",
			[]string{watchNode(1, "fb", rollup(greenOK, lintJob), dropped(after))},
			[]want{{"dropped from the Graphite merge queue", 12}},
			[]string{
				draftNode("gtmq_12", "Merge queue: #12", after, rollup(flakeJob)),
				draftNode("other", "Fix #1", after, rollup(flakeJob)),
				draftNode("gtmq_1", "Merge queue: #1", before, rollup(flakeJob)),
			},
		},
		{
			"a drop before the last run is not news",
			[]string{watchNode(1, "fb", rollup(greenOK), dropped(before))},
			nil, nil,
		},
		{
			"a person removing the label or Graphite removing another label is no failure",
			[]string{
				watchNode(1, "fb", rollup(greenOK), unlabel(after, "merge-queue", "logan")),
				watchNode(2, "fb", rollup(greenOK), unlabel(after, "large-pr", "Graphite-App")),
			},
			nil, nil,
		},
		{
			"only the latest removal of the queue label counts",
			[]string{watchNode(1, "fb", noRollup,
				dropped(after)+","+unlabel(after, "merge-queue", "logan")+","+unlabel(after, "large-pr", graphiteBot))},
			nil, nil,
		},
		{
			"a red stage 1 names its failing job every run",
			[]string{watchNode(1, "fb", rollup(redOK, lintJob), dropped(before))},
			[]want{{"stage 1 is red", 12}},
			nil,
		},
		{
			"a red ci-ok alone names ci-ok",
			[]string{watchNode(1, "fb", rollup(okJob, redOK), "")},
			[]want{{"stage 1 is red", 14}},
			nil,
		},
		{
			"a drop with no failed run has no job",
			[]string{watchNode(1, "fb", noRollup, unlabel(after, "merge-queue", "Graphite-App"))},
			[]want{{"dropped from the Graphite merge queue", 0}},
			[]string{draftNode("gtmq_1", "#1", after, noRollup)},
		},
		{
			"green, pending and unrelated failures stay quiet",
			[]string{
				watchNode(1, "fb", rollup(greenOK, flakeJob), ""),
				watchNode(2, "fb", rollup(`{"name":"ci / ci-ok","conclusion":""}`), ""),
				watchNode(3, "fb", noRollup, ""),
			},
			nil, nil,
		},
		{
			"a stacked PR counts and a PR off the feature branch does not",
			[]string{
				watchNode(1, "fb", rollup(greenOK), ""),
				watchNode(2, "b1", rollup(redOK), ""),
				watchNode(3, "main", rollup(redOK), ""),
			},
			[]want{{"stage 1 is red", 14}},
			nil,
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
			for _, f := range failures(prs, queueRuns{"merge-queue", drafts}, "fb", since) {
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
	failed, _, err := f.Env(t).failures(context.Background())
	if err != nil || len(failed) != 1 || failed[0].PR != 6 || failed[0].Job.DatabaseID != 12 {
		t.Fatalf("%+v %v", failed, err)
	}
	f.hub.onQuery(`c1: object(oid:\"h6\")`, `{"data":null,"errors":[{"message":"rate limited"}]}`)
	if _, _, err := f.Env(t).failures(context.Background()); cliText(err) != "graphql: rate limited" {
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

func (f *fixture) noFailures() { f.hub.on(graphqlRoute, failureData()) }

func TestWatch_printsAFreshOwnerPromptForAnEjectedEntryOnce(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	f.owner(t, Record{Ticket: 40, State: Exited, Worktree: "/wt/40"})
	f.hub.on(graphqlRoute, draftData(
		[]string{draftNode("gtmq_5", "Merge queue: #5", f.now.Add(-time.Minute), rollup(flakeJob))},
		watchNode(5, "fb", rollup(greenOK), dropped(f.now.Add(-time.Minute))),
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
	if _, _, err := env.failures(context.Background()); err == nil || !strings.Contains(err.Error(), "parse watch") {
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
	if _, _, err := env.failures(context.Background()); err == nil || !strings.Contains(err.Error(), "write") {
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
	raw := strings.Replace(numberedDraft(4, time.Unix(1, 0), goCacheJob, flakeJob), `"body":""`, `"body":"#4"`, 1)
	var d queueDraft
	if err := json.Unmarshal([]byte(raw), &d); err != nil {
		t.Fatal(err)
	}
	got := queueJob(4, lastCommits{}, []queueDraft{d}, time.Time{})
	if got.Name != "ci / Flake" {
		t.Fatalf("queueJob = %q, want ci / Flake", got.Name)
	}
}
