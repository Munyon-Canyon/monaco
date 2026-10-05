package agents

import (
	"context"
	"errors"
	"fmt"
	"io"
	"slices"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
)

var errRequeued = errors.New("the stack was queued again during its release")

const (
	dequeueTries  = 3
	dequeueChecks = 2
	dequeueEvery  = 30 * time.Second
	draftsQuery   = `query($owner:String!,$name:String!){repository(owner:$owner,name:$name){` +
		`drafts: pullRequests(states:OPEN,last:30,orderBy:{field:UPDATED_AT,direction:ASC}){nodes{` +
		`number state title body headRefName}} ` +
		`closed: pullRequests(states:CLOSED,last:30,orderBy:{field:UPDATED_AT,direction:ASC}){nodes{` +
		`number state title body headRefName headRefOid updatedAt}}}}`
)

func dequeueCmd(ctx context.Context, env *Env, args []string, stdout io.Writer) error {
	n, err := prArg(args, "dequeue <top-pr>")
	if err != nil {
		return err
	}
	rec, err := env.heldRecord(ctx, n)
	if err != nil {
		return err
	}
	switch {
	case rec.Queued == nil && rec.Armed == nil:
		return env.dequeueLabeled(ctx, n, stdout)
	case rec.Queued == nil:
		if err := env.unmark(ctx, rec); err != nil {
			return err
		}
		_, _ = fmt.Fprintf(stdout, "disarmed #%d; agents watch will not land it\n", n)
		return nil
	}
	if err := env.releaseQueue(ctx, rec.Queued, nil); err != nil {
		return err
	}
	if err := env.unmark(ctx, rec); err != nil {
		return err
	}
	_, _ = fmt.Fprintf(stdout, "dequeued %s; safe to push\n", prRefs(rec.Queued.PRs))
	return nil
}

func (env *Env) dequeueLabeled(ctx context.Context, top int, stdout io.Writer) error {
	open, err := env.openPulls(ctx)
	if err != nil {
		return err
	}
	prs := labeledChain(open, top, env.Config.QueueLabel)
	if len(prs) == 0 {
		_, _ = fmt.Fprintf(stdout, "no PR of the stack under #%d carries %s; safe to push\n",
			top, env.Config.QueueLabel)
		return nil
	}
	if err := env.releaseQueue(ctx, &Queue{Top: top, PRs: prs}, nil); err != nil {
		return err
	}
	_, _ = fmt.Fprintf(stdout, "dequeued %s; safe to push\n", prRefs(prs))
	return nil
}

func labeledChain(open []stackPR, top int, label string) []int {
	byHead := map[string]stackPR{}
	var cur stackPR
	for _, p := range open {
		byHead[p.Head] = p
		if p.Number == top {
			cur = p
		}
	}
	var out []int
	for seen := 0; cur.Number != 0 && seen <= len(open); seen++ {
		if cur.labeled(label) {
			out = append([]int{cur.Number}, out...)
		}
		cur = byHead[cur.Base]
	}
	return out
}

func (env *Env) releaseQueue(ctx context.Context, q *Queue, stop func(context.Context) (bool, error)) error {
	for range dequeueTries {
		if stop != nil {
			halt, err := stop(ctx)
			if err != nil {
				return err
			}
			if halt {
				return errRequeued
			}
		}
		held, err := env.release(ctx, q.PRs)
		if err != nil || !held {
			return err
		}
	}
	return dequeueErr(errs.CodeVersionConflict, fmt.Sprintf(
		"Graphite still holds #%d; remove it from the queue in the Graphite app, then rerun", q.Top))
}

func (env *Env) heldRecord(ctx context.Context, top int) (Record, error) {
	tops, err := env.stackPulls(ctx, []int{top})
	if err != nil {
		return Record{}, err
	}
	ticket, ok := PR{Body: tops[0].Body}.Ticket()
	if !ok {
		return Record{}, dequeueErr(errs.CodeInvalidInput, fmt.Sprintf("#%d links no ticket", top))
	}
	rec, err := env.record(ctx, ticket)
	if err != nil {
		return Record{}, err
	}
	queued := rec.Queued != nil && rec.Queued.Top == top
	armed := rec.Queued == nil && rec.Armed != nil && rec.Armed.Top == top
	empty := rec.Queued == nil && rec.Armed == nil
	if !queued && !armed && !empty {
		return Record{}, dequeueErr(errs.CodeInvalidInput,
			fmt.Sprintf("#%d has no queued or armed stack with top #%d", ticket, top))
	}
	return rec, nil
}

func (env *Env) release(ctx context.Context, nums []int) (bool, error) {
	for _, p := range nums {
		if err := env.removeLabel(ctx, p); err != nil {
			return false, err
		}
	}
	return env.graphiteHolds(ctx, nums)
}

func (env *Env) graphiteHolds(ctx context.Context, nums []int) (bool, error) {
	for range dequeueChecks {
		select {
		case <-ctx.Done():
			return false, fmt.Errorf("wait for Graphite: %w", context.Cause(ctx))
		case <-env.After(dequeueEvery):
		}
		prs, err := env.stackPulls(ctx, nums)
		if err != nil {
			return false, err
		}
		if slices.ContainsFunc(prs, func(p stackPR) bool { return p.labeled(env.Config.QueueLabel) }) {
			return true, nil
		}
		drafts, err := env.queueDrafts(ctx)
		if err != nil {
			return false, err
		}
		if slices.ContainsFunc(nums, func(n int) bool { return draftHolds(drafts, n) }) {
			return true, nil
		}
	}
	return false, nil
}

func dequeueErr(code errs.Code, detail string) error {
	return detailErr(code, "monacoctl.agents.dequeue", detail)
}

func (env *Env) queueDrafts(ctx context.Context) ([]queueDraft, error) {
	var data struct {
		Repository struct {
			Drafts struct {
				Nodes []queueDraft `json:"nodes"`
			} `json:"drafts"`
			Closed struct {
				Nodes []queueDraft `json:"nodes"`
			} `json:"closed"`
		} `json:"repository"`
	}
	if err := env.graphQL(ctx, draftsQuery, &data); err != nil {
		return nil, err
	}
	return slices.Concat(data.Repository.Drafts.Nodes, data.Repository.Closed.Nodes), nil
}
