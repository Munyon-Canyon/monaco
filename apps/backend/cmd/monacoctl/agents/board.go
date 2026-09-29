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
	verifyContext = "verify"
	prFields      = `number body createdAt state mergedAt mergeQueueEntry{position}
commits(last:1){nodes{commit{committedDate statusCheckRollup{contexts(first:50){nodes{
... on CheckRun{name status conclusion completedAt} ... on StatusContext{context state createdAt}}}}}}}
timelineItems(itemTypes:[ADDED_TO_MERGE_QUEUE_EVENT,REMOVED_FROM_MERGE_QUEUE_EVENT],last:50){nodes{__typename
... on AddedToMergeQueueEvent{createdAt} ... on RemovedFromMergeQueueEvent{createdAt}}}`
)

type gqlPR struct {
	Number          int       `json:"number"`
	Body            string    `json:"body"`
	CreatedAt       time.Time `json:"createdAt"`
	State           string    `json:"state"`
	MergedAt        time.Time `json:"mergedAt"`
	MergeQueueEntry *struct {
		Position int `json:"position"`
	} `json:"mergeQueueEntry"`
	Commits struct {
		Nodes []struct {
			Commit struct {
				CommittedDate     time.Time `json:"committedDate"`
				StatusCheckRollup *struct {
					Contexts struct {
						Nodes []gqlContext `json:"nodes"`
					} `json:"contexts"`
				} `json:"statusCheckRollup"`
			} `json:"commit"`
		} `json:"nodes"`
	} `json:"commits"`
	TimelineItems struct {
		Nodes []struct {
			Typename  string    `json:"__typename"`
			CreatedAt time.Time `json:"createdAt"`
		} `json:"nodes"`
	} `json:"timelineItems"`
}

type gqlContext struct {
	Name        string    `json:"name"`
	Status      string    `json:"status"`
	Conclusion  string    `json:"conclusion"`
	CompletedAt time.Time `json:"completedAt"`
	Context     string    `json:"context"`
	State       string    `json:"state"`
	CreatedAt   time.Time `json:"createdAt"`
}

type ticketPR struct {
	Number   int
	Opened   time.Time
	Merged   time.Time
	Queue    int
	Head     time.Time
	Stage1   string
	Stage1At time.Time
	Verify   string
	VerifyAt time.Time
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
		Repository map[string]struct {
			TimelineItems struct {
				Nodes []struct {
					Source gqlPR `json:"source"`
				} `json:"nodes"`
			} `json:"timelineItems"`
		} `json:"repository"`
	}
	if err := env.GitHub.graphql(ctx, ticketQuery(nums), &data); err != nil {
		return nil, err
	}
	out := make([]ticketView, 0, len(b.Tickets))
	for _, t := range b.Tickets {
		v := ticketView{Ticket: t.Ticket, Dispatched: t.Dispatched}
		for _, node := range data.Repository[fmt.Sprintf("t%d", t.Ticket)].TimelineItems.Nodes {
			src := node.Source
			n, ok := PR{Body: src.Body}.Ticket()
			if !ok || n != t.Ticket || src.State == "CLOSED" ||
				slices.ContainsFunc(v.PRs, func(p ticketPR) bool { return p.Number == src.Number }) {
				continue
			}
			v.PRs = append(v.PRs, src.flat())
		}
		out = append(out, v)
	}
	return out, nil
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

func (p gqlPR) flat() ticketPR {
	t := ticketPR{Number: p.Number, Opened: p.CreatedAt, Merged: p.MergedAt}
	if p.MergeQueueEntry != nil {
		t.Queue = p.MergeQueueEntry.Position
	}
	for _, c := range p.Commits.Nodes {
		t.Head = c.Commit.CommittedDate
		if c.Commit.StatusCheckRollup == nil {
			continue
		}
		for _, x := range c.Commit.StatusCheckRollup.Contexts.Nodes {
			t.note(x)
		}
	}
	for _, e := range p.TimelineItems.Nodes {
		t.Queued = append(t.Queued, queueEvent{Added: e.Typename == "AddedToMergeQueueEvent", At: e.CreatedAt})
	}
	return t
}

func (t *ticketPR) note(c gqlContext) {
	switch {
	case c.Name == stage1Check && c.Status != "COMPLETED":
		t.Stage1 = "pending"
	case c.Name == stage1Check:
		t.Stage1, t.Stage1At = strings.ToLower(c.Conclusion), c.CompletedAt
	case c.Context == verifyContext:
		t.Verify, t.VerifyAt = strings.ToLower(c.State), c.CreatedAt
	}
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
