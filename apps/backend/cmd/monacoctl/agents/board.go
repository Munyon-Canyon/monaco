package agents

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"
)

const (
	stage1Check   = "ci / ci-ok"
	formatCheck   = "PR format (title, body and commits)"
	verifyContext = "verify"
	labelFields   = `labels(first:20){nodes{name}}`
	prFields      = `number body createdAt state mergedAt closedAt headRefOid ` + labelFields + `
commits(last:1){nodes{commit{committedDate ` + commitChecks + `}}}
timelineItems(itemTypes:[LABELED_EVENT,UNLABELED_EVENT],last:100){nodes{__typename
... on LabeledEvent{createdAt label{name}} ... on UnlabeledEvent{createdAt label{name}}}}`
)

type gqlName struct {
	Name string `json:"name"`
}

type gqlPR struct {
	Number    int       `json:"number"`
	Body      string    `json:"body"`
	CreatedAt time.Time `json:"createdAt"`
	State     string    `json:"state"`
	MergedAt  time.Time `json:"mergedAt"`
	ClosedAt  time.Time `json:"closedAt"`
	HeadOID   string    `json:"headRefOid"`
	Labels    struct {
		Nodes []gqlName `json:"nodes"`
	} `json:"labels"`
	Commits struct {
		Nodes []struct {
			Commit gqlCommit `json:"commit"`
		} `json:"nodes"`
	} `json:"commits"`
	TimelineItems struct {
		Nodes []struct {
			Typename  string    `json:"__typename"`
			CreatedAt time.Time `json:"createdAt"`
			Label     gqlName   `json:"label"`
		} `json:"nodes"`
	} `json:"timelineItems"`
}

type ticketTimeline struct {
	TimelineItems struct {
		Nodes []struct {
			Source gqlPR `json:"source"`
		} `json:"nodes"`
	} `json:"timelineItems"`
}

type ticketPR struct {
	Number   int
	Opened   time.Time
	Merged   time.Time
	InQueue  bool
	Head     time.Time
	Stage1   string
	Stage1At time.Time
	Verify   string
	VerifyAt time.Time
	Format   string
	Queued   []queueEvent
}

type queueEvent struct {
	Added bool
	At    time.Time
}

type ticketView struct {
	Ticket     int
	Dispatched time.Time
	PRs        []ticketPR
}

func ticketQuery(tickets []int) string {
	var b strings.Builder
	b.WriteString("query($owner:String!,$name:String!){repository(owner:$owner,name:$name){")
	for _, n := range tickets {
		_, _ = fmt.Fprintf(&b, "t%d: issue(number:%d){timelineItems(itemTypes:[CROSS_REFERENCED_EVENT],first:100){"+
			"nodes{... on CrossReferencedEvent{source{... on PullRequest{...pr}}}}}}", n, n)
	}
	b.WriteString("}}\nfragment pr on PullRequest{" + prFields + "}")
	return b.String()
}

func (env *Env) views(ctx context.Context, b Batch) ([]ticketView, error) {
	if len(b.Tickets) == 0 {
		return nil, nil
	}
	nums := make([]int, len(b.Tickets))
	for i, t := range b.Tickets {
		nums[i] = t.Ticket
	}
	var data struct {
		Repository map[string]ticketTimeline `json:"repository"`
	}
	if err := env.graphQL(ctx, ticketQuery(nums), &data); err != nil {
		return nil, err
	}
	if err := env.readChecks(ctx, timelineCommits(data.Repository), env.graphQL); err != nil {
		return nil, err
	}
	out := make([]ticketView, 0, len(b.Tickets))
	for _, t := range b.Tickets {
		prs, err := env.ticketPRs(ctx, t.Ticket, data.Repository[fmt.Sprintf("t%d", t.Ticket)])
		if err != nil {
			return nil, err
		}
		out = append(out, ticketView{Ticket: t.Ticket, Dispatched: t.Dispatched, PRs: prs})
	}
	return out, nil
}

func (env *Env) ticketPRs(ctx context.Context, ticket int, tl ticketTimeline) ([]ticketPR, error) {
	var out []ticketPR
	for _, node := range tl.TimelineItems.Nodes {
		src := node.Source
		n, ok := PR{Body: src.Body}.Ticket()
		if !ok || n != ticket || slices.ContainsFunc(out, func(p ticketPR) bool { return p.Number == src.Number }) {
			continue
		}
		p := src.flat(env.Config.QueueLabel)
		if src.State == "CLOSED" {
			landed, err := env.landed(ctx, src.closed())
			if err != nil {
				return nil, err
			}
			if !landed {
				continue
			}
			p.Merged = src.ClosedAt
		}
		out = append(out, p)
	}
	return out, nil
}

func timelineCommits(tickets map[string]ticketTimeline) []*gqlCommit {
	var out []*gqlCommit
	for _, t := range tickets {
		nodes := t.TimelineItems.Nodes
		for i := range nodes {
			out = append(out, nodes[i].Source.commits()...)
		}
	}
	return out
}

func (env *Env) withDispatch(b, prev Batch) (Batch, error) {
	rs, err := env.records()
	if err != nil {
		return Batch{}, err
	}
	at := map[int]time.Time{}
	for _, t := range append(prev.Tickets, b.Tickets...) {
		if !t.Dispatched.IsZero() {
			at[t.Ticket] = t.Dispatched
		}
	}
	for _, r := range rs {
		at[r.Ticket] = r.Started
	}
	for i := range b.Tickets {
		b.Tickets[i].Dispatched = at[b.Tickets[i].Ticket]
	}
	return b, nil
}

func (p *gqlPR) commits() []*gqlCommit {
	out := make([]*gqlCommit, len(p.Commits.Nodes))
	for i := range p.Commits.Nodes {
		out[i] = &p.Commits.Nodes[i].Commit
	}
	return out
}

func (p gqlPR) labeled(label string) bool {
	return slices.Contains(p.Labels.Nodes, gqlName{label})
}

func (p gqlPR) flat(label string) ticketPR {
	t := ticketPR{Number: p.Number, Opened: p.CreatedAt, Merged: p.MergedAt, InQueue: p.labeled(label)}
	for _, c := range p.Commits.Nodes {
		t.Head = c.Commit.CommittedDate
		for _, x := range c.Commit.latest() {
			t.note(x)
		}
		if (t.Stage1 == "" || t.Stage1 == "failure") && c.Commit.newerRunPending() {
			t.Stage1 = "pending"
		}
	}
	for _, e := range p.TimelineItems.Nodes {
		if e.Label.Name == label {
			t.Queued = append(t.Queued, queueEvent{Added: e.Typename == "LabeledEvent", At: e.CreatedAt})
		}
	}
	return t
}

func (t *ticketPR) note(c gqlContext) {
	switch {
	case c.Name == stage1Check && c.Status != "COMPLETED":
		t.Stage1 = "pending"
	case c.Name == stage1Check:
		t.Stage1, t.Stage1At = strings.ToLower(c.Conclusion), c.CompletedAt
	case c.Name == formatCheck && c.Conclusion == "":
		t.Format = "pending"
	case c.Name == formatCheck:
		t.Format = strings.ToLower(c.Conclusion)
	case c.Context == verifyContext:
		t.Verify, t.VerifyAt = strings.ToLower(c.State), c.CreatedAt
	}
}

func (t ticketPR) ejected() bool {
	if len(t.Queued) == 0 {
		return false
	}
	last := t.Queued[len(t.Queued)-1]
	return !last.Added && last.At.After(t.Head)
}

func (v ticketView) state() string {
	var open []ticketPR
	for _, p := range v.PRs {
		if p.Merged.IsZero() {
			open = append(open, p)
		}
	}
	switch {
	case len(v.PRs) == 0:
		return "building"
	case len(open) == 0:
		return "merged"
	}
	if slices.ContainsFunc(open, func(p ticketPR) bool { return p.InQueue }) {
		return "queued"
	}
	if slices.ContainsFunc(open, ticketPR.ejected) {
		return "ejected"
	}
	if slices.ContainsFunc(open, func(p ticketPR) bool { return p.Stage1 != "success" }) {
		return "stage 1"
	}
	return "verifying"
}

func (v ticketView) lastOfAll(at func(ticketPR) (time.Time, bool)) time.Time {
	var last time.Time
	for _, p := range v.PRs {
		t, ok := at(p)
		if !ok {
			return time.Time{}
		}
		last = latest(last, t)
	}
	return last
}

func (v ticketView) first(at func(ticketPR) []time.Time) time.Time {
	var first time.Time
	for _, p := range v.PRs {
		for _, t := range at(p) {
			first = earliest(first, t)
		}
	}
	return first
}

func (v ticketView) merged() time.Time {
	return v.lastOfAll(func(p ticketPR) (time.Time, bool) { return p.Merged, !p.Merged.IsZero() })
}

func (v ticketView) elapsed(now time.Time) string {
	if v.Dispatched.IsZero() {
		return "-"
	}
	end := now
	if m := v.merged(); !m.IsZero() {
		end = m
	}
	return span(end.Sub(v.Dispatched))
}

func latest(a, b time.Time) time.Time {
	if b.After(a) {
		return b
	}
	return a
}

func earliest(a, b time.Time) time.Time {
	if a.IsZero() || (!b.IsZero() && b.Before(a)) {
		return b
	}
	return a
}

func span(d time.Duration) string {
	d = d.Round(time.Minute)
	h, m := int(d.Hours()), int(d.Minutes())%60
	if h == 0 {
		return fmt.Sprintf("%dm", m)
	}
	return fmt.Sprintf("%dh%02dm", h, m)
}

func boardRows(views []ticketView, now time.Time) string {
	var b strings.Builder
	b.WriteString("| ticket | state | since dispatch |\n| --- | --- | --- |\n")
	for _, v := range views {
		_, _ = fmt.Fprintf(&b, "| #%d | %s | %s |\n", v.Ticket, v.state(), v.elapsed(now))
	}
	return b.String()
}
