package agents

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
)

const (
	lastRunState = "last-run"
	draftPrefix  = "gtmq_"
)

func failureQuery(after string) string {
	drafts := ""
	if after == "" {
		drafts = `drafts: pullRequests(states:[OPEN,CLOSED],last:30,orderBy:{field:UPDATED_AT,direction:ASC}){nodes{` +
			`number state title body headRefName headRefOid updatedAt closedAt commits(last:1){nodes{commit{...runs}}}}}`
	}
	return `query($owner:String!,$name:String!){repository(owner:$owner,name:$name){` +
		`pullRequests(` + openPage(after) + `){pageInfo{hasNextPage endCursor} ` +
		`nodes{number body isDraft mergeable headRefName baseRefName headRefOid ` +
		labelFields + ` commits(last:1){nodes{commit{...runs}}} ` +
		`timelineItems(itemTypes:[UNLABELED_EVENT],last:20){nodes{` +
		`... on UnlabeledEvent{createdAt label{name} actor{login}}}}}} ` +
		drafts + `}}` +
		"\nfragment runs on Commit{" + commitChecks + "}"
}

type lastCommits = struct {
	Nodes []struct {
		Commit gqlCommit `json:"commit"`
	} `json:"nodes"`
}

type queueDraft struct {
	Number      int         `json:"number"`
	State       string      `json:"state"`
	Title       string      `json:"title"`
	Body        string      `json:"body"`
	HeadRefName string      `json:"headRefName"`
	HeadRefOID  string      `json:"headRefOid"`
	UpdatedAt   time.Time   `json:"updatedAt"`
	ClosedAt    time.Time   `json:"closedAt"`
	Commits     lastCommits `json:"commits"`
}

type watchData struct {
	prs    []watchPR
	drafts []queueDraft
}

type watchPR struct {
	Number      int    `json:"number"`
	Body        string `json:"body"`
	HeadRefName string `json:"headRefName"`
	BaseRefName string `json:"baseRefName"`
	HeadRefOid  string `json:"headRefOid"`
	IsDraft     bool   `json:"isDraft"`
	Mergeable   string `json:"mergeable"`
	Labels      struct {
		Nodes []gqlName `json:"nodes"`
	} `json:"labels"`
	Commits       lastCommits `json:"commits"`
	TimelineItems gqlTimeline `json:"timelineItems"`
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
	return d.State != "OPEN" && d.ClosedAt.After(since) && d.tests(pr)
}

func draftHolds(drafts []queueDraft, pr int) bool {
	return slices.ContainsFunc(drafts, func(d queueDraft) bool { return d.State == "OPEN" && d.tests(pr) })
}

func (d queueDraft) tests(pr int) bool {
	return strings.HasPrefix(d.HeadRefName, draftPrefix) && (mentions(d.Title, pr) || mentions(d.Body, pr))
}

func mentions(text string, pr int) bool {
	n := strconv.Itoa(pr)
	if regexp.MustCompile(`#` + n + `\b`).MatchString(text) {
		return true
	}
	for _, m := range regexp.MustCompile(`\(PRs ([0-9, ]+)\)`).FindAllStringSubmatch(text, -1) {
		for _, f := range strings.Split(m[1], ",") {
			if strings.TrimSpace(f) == n {
				return true
			}
		}
	}
	return false
}

func red(r gqlContext) bool { return r.Conclusion == "FAILURE" || r.Conclusion == "TIMED_OUT" }

func (c gqlCommit) stage1Red() bool {
	for _, r := range c.latest() {
		if r.Name == stage1Check {
			return red(r) && !c.newerRunPending()
		}
	}
	return false
}

func (c gqlCommit) stage1Green() bool {
	for _, r := range c.latest() {
		if r.Name == stage1Check {
			return r.Conclusion == "SUCCESS"
		}
	}
	return false
}

func (c gqlCommit) newerRunPending() bool {
	for _, r := range c.latest() {
		if strings.HasPrefix(r.Name, "ci / ") && r.Name != stage1Check && r.Conclusion == "" {
			return true
		}
	}
	return false
}

const queueCIPrefix = "ci / "

func (c gqlCommit) failedJob() gqlContext {
	var agg gqlContext
	for _, r := range c.latest() {
		switch {
		case !red(r) || !strings.HasPrefix(r.Name, queueCIPrefix):
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
	now    time.Time
	landed map[int]bool
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
	if p.droppedByGraphite(queue, since) {
		f.Why, f.Job = droppedWhy, p.queueJob(queue.drafts, since)
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

func (p watchPR) droppedByGraphite(queue queueRuns, since time.Time) bool {
	at, byGraphite, ok := p.TimelineItems.removal(queue.label)
	held := draftHolds(queue.drafts, p.Number) || awaitsDraft(queue.drafts, p.Number, at, queue.now)
	if !ok || !byGraphite || held {
		return false
	}
	if ran := newestRun(queue.drafts, p.Number, at); !ran.IsZero() {
		return ran.After(since) && !queue.landed[p.Number]
	}
	return at.Add(takenFor).After(since)
}

func (env *Env) landedDrops(
	ctx context.Context,
	queue queueRuns,
	prs []watchPR,
	since time.Time,
) (map[int]bool, error) {
	landed := map[int]bool{}
	for _, p := range prs {
		at, _, _ := p.TimelineItems.removal(queue.label)
		if !draftRan(queue.drafts, p.Number, at) || !p.droppedByGraphite(queue, since) {
			continue
		}
		in, err := env.draftLanded(ctx, p.Number, at, queue.drafts)
		if err != nil {
			return nil, err
		}
		landed[p.Number] = in
	}
	return landed, nil
}

func graphiteLogin(login string) bool {
	return strings.Contains(strings.ToLower(login), "graphite")
}

func (p watchPR) queueJob(drafts []queueDraft, since time.Time) gqlContext {
	return queueJob(p.Number, p.Commits, drafts, since)
}

func queueJob(pr int, head lastCommits, drafts []queueDraft, since time.Time) gqlContext {
	var newest queueDraft
	for _, d := range drafts {
		if d.runs(pr, since) && d.ClosedAt.After(newest.ClosedAt) {
			newest = d
		}
	}
	if nodes := newest.Commits.Nodes; len(nodes) > 0 {
		return nodes[len(nodes)-1].Commit.failedJob()
	}
	if nodes := head.Nodes; len(nodes) > 0 {
		return nodes[len(nodes)-1].Commit.failedJob()
	}
	return gqlContext{}
}

func (env *Env) failures(ctx context.Context, rs []Record) ([]failure, watchData, error) {
	since, err := env.lastRun()
	if err != nil {
		return nil, watchData{}, err
	}
	now := env.Now()
	data, err := env.watchData(ctx)
	if err != nil {
		return nil, watchData{}, err
	}
	queue := queueRuns{label: env.Config.QueueLabel, drafts: data.drafts, now: now}
	if queue.landed, err = env.landedDrops(ctx, queue, data.prs, since); err != nil {
		return nil, watchData{}, err
	}
	stamp := now.UTC().Format(time.RFC3339Nano)
	if _, err := env.writeState("watch", lastRunState, []byte(stamp+"\n")); err != nil {
		return nil, watchData{}, err
	}
	return onePerOwnedStack(failures(data.prs, queue, env.Config.FeatureBranch, since), rs), data, nil
}

func onePerOwnedStack(failed []failure, rs []Record) []failure {
	type report struct {
		ticket int
		job    int64
	}
	ownedDrop := func(f failure) (report, bool) {
		_, ticket, owned := ownerStack(rs, f.PR)
		return report{ticket, f.Job.DatabaseID}, owned && f.Why == droppedWhy
	}
	lowest := map[report]int{}
	for _, f := range failed {
		if r, ok := ownedDrop(f); ok && (lowest[r] == 0 || f.PR < lowest[r]) {
			lowest[r] = f.PR
		}
	}
	return slices.DeleteFunc(failed, func(f failure) bool {
		r, ok := ownedDrop(f)
		return ok && f.PR != lowest[r]
	})
}

func stuckOnGraphiteBase(prs []watchPR, rs []Record, label string) []string {
	var out []string
	for _, p := range prs {
		if !strings.HasPrefix(p.BaseRefName, "graphite-base/") {
			continue
		}
		top, ticket, owned := ownerStack(rs, p.Number)
		if !owned && !slices.Contains(p.Labels.Nodes, gqlName{label}) {
			continue
		}
		who := "no ticket"
		switch n, ok := (PR{Body: p.Body}).Ticket(); {
		case owned:
			who = fmt.Sprintf("owner record %d.json", ticket)
		case ok:
			who = fmt.Sprintf("ticket #%d", n)
		}
		if !owned {
			top = stackTop(prs, p)
		}
		out = append(out, fmt.Sprintf("#%d is stuck on %s (a restack that never retargeted); owner: dequeue %d, "+
			"gt sync, gt restack, gt submit --stack --draft, land-stack %d; %s", p.Number, p.BaseRefName, top, top, who))
	}
	return out
}

const (
	conflicting = "CONFLICTING"
	unsettled   = "UNKNOWN"
)

func conflictLine(pr int, base string) string {
	return fmt.Sprintf("#%d conflicts with %s; GitHub runs no CI until it is resolved: restack with gt and resubmit",
		pr, base)
}

type stallScan struct {
	env   *Env
	prs   []watchPR
	rs    []Record
	heads map[string]string
}

func (env *Env) silentStalls(ctx context.Context, prs []watchPR, rs []Record) []string {
	s := stallScan{env: env, prs: prs, rs: rs, heads: map[string]string{}}
	var out []string
	for _, p := range prs {
		if line := s.stall(ctx, p); line != "" {
			out = append(out, line)
		}
	}
	return out
}

func (s stallScan) stall(ctx context.Context, p watchPR) string {
	labeled := slices.Contains(p.Labels.Nodes, gqlName{s.env.Config.QueueLabel})
	_, _, marked := ownerStack(s.rs, p.Number)
	if p.Mergeable == conflicting {
		if _, owned := s.recordOf(ctx, p); labeled || marked || owned {
			return conflictLine(p.Number, p.BaseRefName)
		}
		return ""
	}
	if labeled || marked || !s.landable(p) {
		return ""
	}
	r, ok := s.recordOf(ctx, p)
	if !ok {
		return ""
	}
	if why := restackReason(s.prs, p); why != "" {
		return fmt.Sprintf("#%d is green but its stack needs a restack: %s; owner record %d.json: "+
			"restack onto %s with gt, resubmit, then land-stack %d",
			p.Number, why, r.Ticket, s.env.Config.FeatureBranch, p.Number)
	}
	return fmt.Sprintf("#%d is green but not armed; owner record %d.json: run land-stack %d",
		p.Number, r.Ticket, p.Number)
}

func (s stallScan) landable(top watchPR) bool {
	if top.IsDraft || !top.green() || stackTop(s.prs, top) != top.Number {
		return false
	}
	chain := chainDown(s.prs, top)
	return !slices.ContainsFunc(s.prs, func(q watchPR) bool {
		return q.Mergeable == unsettled && slices.Contains(chain, q.HeadRefName)
	})
}

func restackReason(prs []watchPR, top watchPR) string {
	for _, head := range chainDown(prs, top) {
		i := slices.IndexFunc(prs, func(q watchPR) bool { return q.HeadRefName == head })
		switch {
		case i < 0:
		case strings.HasPrefix(prs[i].BaseRefName, "graphite-base/"):
			return fmt.Sprintf("#%d sits on %s", prs[i].Number, prs[i].BaseRefName)
		case prs[i].Mergeable == conflicting:
			return fmt.Sprintf("#%d conflicts with %s", prs[i].Number, prs[i].BaseRefName)
		}
	}
	return ""
}

func (s stallScan) recordOf(ctx context.Context, p watchPR) (Record, bool) {
	chain := chainDown(s.prs, p)
	for _, r := range s.rs {
		if r.Branch != "" && slices.Contains(chain, r.Branch) {
			return r, true
		}
	}
	for _, r := range s.rs {
		if slices.Contains(chain, s.head(ctx, r.Worktree)) {
			return r, true
		}
	}
	return Record{}, false
}

func (s stallScan) head(ctx context.Context, worktree string) string {
	if h, ok := s.heads[worktree]; ok {
		return h
	}
	out, err := s.env.Run(ctx, worktree, "", "git", "rev-parse", "--abbrev-ref", "HEAD")
	h := strings.TrimSpace(string(out))
	if err != nil || h == "HEAD" {
		h = ""
	}
	s.heads[worktree] = h
	return h
}

func (p watchPR) green() bool {
	n := p.Commits.Nodes
	return len(n) > 0 && n[len(n)-1].Commit.stage1Green()
}

func chainDown(prs []watchPR, p watchPR) []string {
	chain := []string{p.HeadRefName}
	for range prs {
		i := slices.IndexFunc(prs, func(q watchPR) bool { return q.HeadRefName == p.BaseRefName })
		if i < 0 {
			break
		}
		p = prs[i]
		chain = append(chain, p.HeadRefName)
	}
	return chain
}

func ownerStack(rs []Record, pr int) (int, int, bool) {
	for _, r := range rs {
		if r.Queued != nil && slices.Contains(r.Queued.PRs, pr) {
			return r.Queued.Top, r.Ticket, true
		}
		if r.Armed != nil && slices.Contains(r.Armed.PRs, pr) {
			return r.Armed.Top, r.Ticket, true
		}
	}
	return 0, 0, false
}

func stackTop(prs []watchPR, p watchPR) int {
	for range prs {
		i := slices.IndexFunc(prs, func(q watchPR) bool { return q.BaseRefName == p.HeadRefName })
		if i < 0 {
			break
		}
		p = prs[i]
	}
	return p.Number
}

func (env *Env) watchData(ctx context.Context) (watchData, error) {
	var prs []watchPR
	var drafts []queueDraft
	for after := ""; ; {
		var data struct {
			Repository struct {
				PullRequests struct {
					PageInfo gqlPageInfo `json:"pageInfo"`
					Nodes    []watchPR   `json:"nodes"`
				} `json:"pullRequests"`
				Drafts struct {
					Nodes []queueDraft `json:"nodes"`
				} `json:"drafts"`
			} `json:"repository"`
		}
		if err := env.graphQL(ctx, failureQuery(after), &data); err != nil {
			return watchData{}, err
		}
		prs = append(prs, data.Repository.PullRequests.Nodes...)
		drafts = append(drafts, data.Repository.Drafts.Nodes...)
		if after = data.Repository.PullRequests.PageInfo.next(); after == "" {
			break
		}
	}
	var commits []*gqlCommit
	for i := range prs {
		commits = append(commits, prs[i].commits()...)
	}
	for i := range drafts {
		commits = append(commits, drafts[i].commits()...)
	}
	if err := env.readChecks(ctx, commits, env.graphQL); err != nil {
		return watchData{}, err
	}
	return watchData{prs: prs, drafts: drafts}, nil
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
		if r, err := env.localRecord(n); err == nil {
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
