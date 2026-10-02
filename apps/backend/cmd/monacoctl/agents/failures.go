package agents

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const (
	lastRunState = "last-run"
	draftPrefix  = "gtmq_"
	failureQuery = `query($owner:String!,$name:String!){repository(owner:$owner,name:$name){` +
		`pullRequests(states:OPEN,first:100){nodes{number body headRefName baseRefName headRefOid ` +
		`commits(last:1){nodes{commit{...runs}}} ` +
		`timelineItems(itemTypes:[UNLABELED_EVENT],last:20){nodes{` +
		`... on UnlabeledEvent{createdAt label{name} actor{login}}}}}} ` +
		`drafts: pullRequests(states:CLOSED,last:30,orderBy:{field:UPDATED_AT,direction:ASC}){nodes{` +
		`title body headRefName updatedAt commits(last:1){nodes{commit{...runs}}}}}}}` +
		"\nfragment runs on Commit{" + commitChecks + "}"
)

type queueDraft struct {
	Title       string    `json:"title"`
	Body        string    `json:"body"`
	HeadRefName string    `json:"headRefName"`
	UpdatedAt   time.Time `json:"updatedAt"`
	Commits     struct {
		Nodes []struct {
			Commit gqlCommit `json:"commit"`
		} `json:"nodes"`
	} `json:"commits"`
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
			CreatedAt time.Time `json:"createdAt"`
			Label     gqlName   `json:"label"`
			Actor     struct {
				Login string `json:"login"`
			} `json:"actor"`
		} `json:"nodes"`
	} `json:"timelineItems"`
}

type failure struct {
	PR   int
	Head string
	Body string
	Why  string
	Job  gqlContext
}

func (p *watchPR) commits() []*gqlCommit {
	out := make([]*gqlCommit, 0, len(p.Commits.Nodes))
	for i := range p.Commits.Nodes {
		out = append(out, &p.Commits.Nodes[i].Commit)
	}
	return out
}

func (d *queueDraft) commits() []*gqlCommit {
	out := make([]*gqlCommit, 0, len(d.Commits.Nodes))
	for i := range d.Commits.Nodes {
		out = append(out, &d.Commits.Nodes[i].Commit)
	}
	return out
}

func (d queueDraft) runs(pr int, since time.Time) bool {
	ref := regexp.MustCompile(`#` + strconv.Itoa(pr) + `\b`)
	return strings.HasPrefix(d.HeadRefName, draftPrefix) && d.UpdatedAt.After(since) &&
		(ref.MatchString(d.Title) || ref.MatchString(d.Body))
}

func red(r gqlContext) bool { return r.Conclusion == "FAILURE" || r.Conclusion == "TIMED_OUT" }

func (c gqlCommit) stage1Red() bool {
	for _, r := range c.latest() {
		if r.Name == stage1Check {
			return red(r)
		}
	}
	return false
}

func (c gqlCommit) failedJob() gqlContext {
	var agg gqlContext
	for _, r := range c.latest() {
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

type queueRuns struct {
	label  string
	drafts []queueDraft
}

func failures(prs []watchPR, queue queueRuns, trunk string, since time.Time) []failure {
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
		if f, ok := p.failure(queue, since); ok {
			out = append(out, f)
		}
	}
	return out
}

func (p watchPR) failure(queue queueRuns, since time.Time) (failure, bool) {
	f := failure{PR: p.Number, Head: p.HeadRefOid, Body: p.Body}
	if p.droppedByGraphite(queue.label, since) {
		f.Why, f.Job = "dropped from the Graphite merge queue", p.queueJob(queue.drafts, since)
		return f, true
	}
	for _, c := range p.Commits.Nodes {
		if c.Commit.stage1Red() {
			f.Why, f.Job = "stage 1 is red", c.Commit.failedJob()
			return f, true
		}
	}
	return failure{}, false
}

func (p watchPR) droppedByGraphite(label string, since time.Time) bool {
	events := p.TimelineItems.Nodes
	for i := len(events) - 1; i >= 0; i-- {
		if events[i].Label.Name == label {
			return events[i].CreatedAt.After(since) &&
				strings.Contains(strings.ToLower(events[i].Actor.Login), "graphite")
		}
	}
	return false
}

func (p watchPR) queueJob(drafts []queueDraft, since time.Time) gqlContext {
	for i := len(drafts) - 1; i >= 0; i-- {
		if nodes := drafts[i].Commits.Nodes; drafts[i].runs(p.Number, since) && len(nodes) > 0 {
			return nodes[len(nodes)-1].Commit.failedJob()
		}
	}
	if nodes := p.Commits.Nodes; len(nodes) > 0 {
		return nodes[len(nodes)-1].Commit.failedJob()
	}
	return gqlContext{}
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
			Drafts struct {
				Nodes []queueDraft `json:"nodes"`
			} `json:"drafts"`
		} `json:"repository"`
	}
	if err := env.GitHub.graphql(ctx, failureQuery, &data); err != nil {
		return nil, err
	}
	var commits []*gqlCommit
	for i := range data.Repository.PullRequests.Nodes {
		commits = append(commits, data.Repository.PullRequests.Nodes[i].commits()...)
	}
	drafts := data.Repository.Drafts.Nodes
	for i := range drafts {
		commits = append(commits, drafts[i].commits()...)
	}
	if err := readAllChecks(ctx, env.GitHub.graphql, commits); err != nil {
		return nil, err
	}
	stamp := env.Now().UTC().Format(time.RFC3339Nano)
	if _, err := env.writeState("watch", lastRunState, []byte(stamp+"\n")); err != nil {
		return nil, err
	}
	queue := queueRuns{label: env.Config.QueueLabel, drafts: drafts}
	return failures(data.Repository.PullRequests.Nodes, queue, env.Config.FeatureBranch, since), nil
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
		if r, err := env.record(ctx, n); err == nil {
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
