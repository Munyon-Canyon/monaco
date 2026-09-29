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
	flakeJob = `{"name":"ci / Flake","conclusion":"FAILURE","databaseId":11,"detailsUrl":"https://gh/job/11"}`
	lintJob  = `{"name":"ci / Lint","conclusion":"TIMED_OUT","databaseId":12,"detailsUrl":"https://gh/job/12"}`
	okJob    = `{"name":"ci / Lint","conclusion":"SUCCESS","databaseId":13,"detailsUrl":"https://gh/job/13"}`
	redOK    = `{"name":"ci / ci-ok","conclusion":"FAILURE","databaseId":14,"detailsUrl":"https://gh/job/14"}`
	greenOK  = `{"name":"ci / ci-ok","conclusion":"SUCCESS","databaseId":15,"detailsUrl":"https://gh/job/15"}`
	noRollup = `{"statusCheckRollup":null}`
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

func removal(at time.Time, reason, commit string) string {
	return fmt.Sprintf(`{"createdAt":%q,"reason":%q,"beforeCommit":%s}`, at.Format(time.RFC3339), reason, commit)
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
		name  string
		nodes []string
		want  []want
	}{
		{
			"failed checks since the last run name the failing job, not ci-ok",
			[]string{watchNode(1, "fb", rollup(greenOK), removal(after, "FAILED_CHECKS", rollup(okJob, redOK, flakeJob)))},
			[]want{{"removed from the merge queue (failed_checks)", 11}},
		},
		{
			"a removal before the last run is not news",
			[]string{watchNode(1, "fb", rollup(greenOK), removal(before, "failed_checks", rollup(flakeJob)))},
			nil,
		},
		{
			"merged and manual removals need no owner",
			[]string{
				watchNode(1, "fb", rollup(greenOK), removal(after, "merged", rollup())),
				watchNode(2, "fb", rollup(greenOK), removal(after, "manual", rollup())),
			},
			nil,
		},
		{
			"only the latest removal counts",
			[]string{watchNode(1, "fb", noRollup,
				removal(after, "failed_checks", rollup(flakeJob))+","+removal(after, "manual", rollup()))},
			nil,
		},
		{
			"a red stage 1 names its failing job every run",
			[]string{watchNode(1, "fb", rollup(redOK, lintJob), removal(before, "failed_checks", rollup()))},
			[]want{{"stage 1 is red", 12}},
		},
		{
			"a red ci-ok alone names ci-ok",
			[]string{watchNode(1, "fb", rollup(okJob, redOK), "")},
			[]want{{"stage 1 is red", 14}},
		},
		{
			"a removal with no failed run has no job",
			[]string{watchNode(1, "fb", noRollup, removal(after, "conflict", noRollup))},
			[]want{{"removed from the merge queue (conflict)", 0}},
		},
		{
			"green, pending and unrelated failures stay quiet",
			[]string{
				watchNode(1, "fb", rollup(greenOK, flakeJob), ""),
				watchNode(2, "fb", rollup(`{"name":"ci / ci-ok","conclusion":""}`), ""),
				watchNode(3, "fb", noRollup, ""),
			},
			nil,
		},
		{
			"a stacked PR counts and a PR off the feature branch does not",
			[]string{
				watchNode(1, "fb", rollup(greenOK), ""),
				watchNode(2, "b1", rollup(redOK), ""),
				watchNode(3, "main", rollup(redOK), ""),
			},
			[]want{{"stage 1 is red", 14}},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var prs []watchPR
			if err := json.Unmarshal([]byte("["+strings.Join(tc.nodes, ",")+"]"), &prs); err != nil {
				t.Fatal(err)
			}
			got := make([]want, 0, len(tc.want))
			for _, f := range failures(prs, "fb", since) {
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
	failed, err := f.Env(t).failures(context.Background())
	if err != nil || len(failed) != 1 || failed[0].PR != 6 || failed[0].Job.DatabaseID != 12 {
		t.Fatalf("%+v %v", failed, err)
	}
	f.hub.onQuery(`c1: object(oid:\"h6\")`, `{"data":null,"errors":[{"message":"rate limited"}]}`)
	if _, err := f.Env(t).failures(context.Background()); cliText(err) != "graphql: rate limited" {
		t.Fatal(err)
	}
}

func failureData(nodes ...string) string {
	return `{"data":{"repository":{"pullRequests":{"nodes":[` + strings.Join(nodes, ",") + `]}}}}`
}

func (f *fixture) noFailures() { f.hub.on(graphqlRoute, failureData()) }

func TestWatch_printsAFreshOwnerPromptForAnEjectedEntryOnce(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	f.owner(t, Record{Ticket: 40, State: Exited, Worktree: "/wt/40"})
	f.hub.on(graphqlRoute, failureData(
		watchNode(5, "fb", rollup(greenOK), removal(f.now.Add(-time.Minute), "FAILED_CHECKS", rollup(flakeJob))),
		strings.Replace(watchNode(6, "fb", rollup(redOK, lintJob), ""), "Part of #40", "no ticket", 1),
	))
	f.hub.on(get("/actions/jobs/11/logs"), "--- FAIL: TestFlaky\n")
	code, stdout, stderr := f.agents(t, "watch", "--verbose")
	logPath := f.Env(t).statePath("logs", "job-11.log")
	want := "#5 removed from the merge queue (failed_checks)\n  failing job: https://gh/job/11\n  fresh owner\n" +
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
	_, stdout, _ = f.agents(t, "watch")
	if strings.Contains(stdout, "#5") || !strings.Contains(stdout, "#6 stage 1 is red") {
		t.Fatalf("second run:\n%s", stdout)
	}
}

func TestWatch_failuresSurfaceStateQueryAndRecordErrors(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	env := f.Env(t)
	state := env.statePath("watch", lastRunState)
	writeFile(t, state, "yesterday\n")
	if _, err := env.failures(context.Background()); err == nil || !strings.Contains(err.Error(), "parse watch") {
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
	if _, err := env.failures(context.Background()); err == nil || !strings.Contains(err.Error(), "write") {
		t.Fatalf("state write: %v", err)
	}
	if err := os.Remove(state); err != nil {
		t.Fatal(err)
	}
	f.hub.on(graphqlRoute, `{"data":null,"errors":[{"message":"rate limited"}]}`)
	if code, _, stderr := f.agents(t, "watch"); code != 1 || !strings.Contains(stderr, "rate limited") {
		t.Fatalf("graphql: %d %q", code, stderr)
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
