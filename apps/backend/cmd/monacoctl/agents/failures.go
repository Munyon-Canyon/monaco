package agents

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	lastRunState = "last-run"
	failureQuery = `query($owner:String!,$name:String!){repository(owner:$owner,name:$name){` +
		`pullRequests(states:OPEN,first:100){nodes{number body headRefName baseRefName headRefOid ` +
		`commits(last:1){nodes{commit{...runs}}} ` +
		`timelineItems(itemTypes:[REMOVED_FROM_MERGE_QUEUE_EVENT],last:5){nodes{` +
		`... on RemovedFromMergeQueueEvent{createdAt reason beforeCommit{...runs}}}}}}}}` +
		"\nfragment runs on Commit{statusCheckRollup{contexts(first:50){nodes{" +
		"... on CheckRun{name conclusion databaseId detailsUrl}}}}}"
)

type gqlRun struct {
	Name       string `json:"name"`
	Conclusion string `json:"conclusion"`
	DatabaseID int64  `json:"databaseId"`
	DetailsURL string `json:"detailsUrl"`
}

type gqlCommit struct {
	StatusCheckRollup *struct {
		Contexts struct {
			Nodes []gqlRun `json:"nodes"`
		} `json:"contexts"`
	} `json:"statusCheckRollup"`
}

type watchPR struct {
	Number      int    `json:"number"`
	Body        string `json:"body"`
	HeadRefName string `json:"headRefName"`
	BaseRefName string `json:"baseRefName"`
	HeadRefOid  string `json:"headRefOid"`
	Commits     struct {
		Nodes []struct {
			Commit gqlCommit `json:"commit"`
		} `json:"nodes"`
	} `json:"commits"`
	TimelineItems struct {
		Nodes []struct {
			CreatedAt    time.Time `json:"createdAt"`
			Reason       string    `json:"reason"`
			BeforeCommit gqlCommit `json:"beforeCommit"`
		} `json:"nodes"`
	} `json:"timelineItems"`
}

type failure struct {
	PR   int
	Head string
	Body string
	Why  string
	Job  gqlRun
}

func (c gqlCommit) runs() []gqlRun {
	if c.StatusCheckRollup == nil {
		return nil
	}
	return c.StatusCheckRollup.Contexts.Nodes
}

func red(r gqlRun) bool { return r.Conclusion == "FAILURE" || r.Conclusion == "TIMED_OUT" }

func (c gqlCommit) stage1Red() bool {
	for _, r := range c.runs() {
		if r.Name == stage1Check {
			return red(r)
		}
	}
	return false
}

func (c gqlCommit) failedJob() gqlRun {
	var agg gqlRun
	for _, r := range c.runs() {
		switch {
		case !red(r):
		case r.Name == stage1Check:
			agg = r
		default:
			return r
		}
	}
	return agg
}

func failures(prs []watchPR, trunk string, since time.Time) []failure {
	flat := make([]PR, len(prs))
	for i, p := range prs {
		flat[i] = PR{Number: p.Number, Head: Ref{Ref: p.HeadRefName}, Base: Ref{Ref: p.BaseRefName}}
	}
	inStack := map[int]bool{}
	for _, stack := range stacks(flat, trunk) {
		for _, p := range stack {
			inStack[p.Number] = true
		}
	}
	var out []failure
	for _, p := range prs {
		if !inStack[p.Number] {
			continue
		}
		if f, ok := p.failure(since); ok {
			out = append(out, f)
		}
	}
	return out
}

func (p watchPR) failure(since time.Time) (failure, bool) {
	f := failure{PR: p.Number, Head: p.HeadRefOid, Body: p.Body}
	events := p.TimelineItems.Nodes
	if n := len(events); n > 0 {
		last := events[n-1]
		reason := strings.ToLower(last.Reason)
		if last.CreatedAt.After(since) && reason != "merged" && reason != "manual" {
			f.Why, f.Job = "removed from the merge queue ("+reason+")", last.BeforeCommit.failedJob()
			return f, true
		}
	}
	for _, c := range p.Commits.Nodes {
		if c.Commit.stage1Red() {
			f.Why, f.Job = "stage 1 is red", c.Commit.failedJob()
			return f, true
		}
	}
	return failure{}, false
}

func (env *Env) failures(ctx context.Context) ([]failure, error) {
	since, err := env.lastRun()
	if err != nil {
		return nil, err
	}
	var data struct {
		Repository struct {
			PullRequests struct {
				Nodes []watchPR `json:"nodes"`
			} `json:"pullRequests"`
		} `json:"repository"`
	}
	if err := env.GitHub.graphql(ctx, failureQuery, &data); err != nil {
		return nil, err
	}
	stamp := env.Now().UTC().Format(time.RFC3339Nano)
	if _, err := env.writeState("watch", lastRunState, []byte(stamp+"\n")); err != nil {
		return nil, err
	}
	return failures(data.Repository.PullRequests.Nodes, env.Config.FeatureBranch, since), nil
}

func (env *Env) lastRun() (time.Time, error) {
	b, err := os.ReadFile(env.statePath("watch", lastRunState))
	if errors.Is(err, fs.ErrNotExist) {
		return time.Time{}, nil
	}
	if err != nil {
		return time.Time{}, fmt.Errorf("read watch state: %w", err)
	}
	t, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(string(b)))
	if err != nil {
		return time.Time{}, fmt.Errorf("parse watch state: %w", err)
	}
	return t, nil
}

func (env *Env) writeFailures(ctx context.Context, failed []failure, stdout io.Writer) {
	for _, f := range failed {
		_, _ = io.WriteString(stdout, env.freshOwnerFor(ctx, f))
	}
}

func (env *Env) freshOwnerFor(ctx context.Context, f failure) string {
	ticket, worktree := "unknown", "unknown"
	if n, ok := (PR{Body: f.Body}).Ticket(); ok {
		ticket = strconv.Itoa(n)
		if r, err := env.record(n); err == nil {
			worktree = r.Worktree
		}
	}
	job, log := "none", "none"
	if f.Job.DatabaseID != 0 {
		job, log = f.Job.DetailsURL, env.jobLog(ctx, f.Job.DatabaseID)
	}
	return fmt.Sprintf(
		"#%d %s\n  failing job: %s\n  fresh owner\n  ticket: %s\n  worktree: %s\n  head: %s\n  log: %s\n  brief: %s\n",
		f.PR, f.Why, job, ticket, worktree, f.Head, log, ownerBrief,
	)
}

func (env *Env) jobLog(ctx context.Context, id int64) string {
	var body []byte
	if err := env.GitHub.call(ctx, "GET", env.GitHub.repo("/actions/jobs/%d/logs", id), "", nil, &body); err != nil {
		return "unavailable (" + err.Error() + ")"
	}
	path, err := env.writeState("logs", fmt.Sprintf("job-%d.log", id), body)
	if err != nil {
		return "unavailable (" + err.Error() + ")"
	}
	return path
}
