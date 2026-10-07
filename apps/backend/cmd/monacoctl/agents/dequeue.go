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
		`number state title body headRefName headRefOid updatedAt closedAt}}}}`
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
	q := rec.Queued.find(n)
	switch {
	case len(rec.Queued) == 0 && len(rec.Armed) == 0:
		return env.dequeueLabeled(ctx, n, stdout)
	case q == nil:
		if err := env.unmark(ctx, rec.Ticket, n, nil); err != nil {
			return err
		}
		_, _ = fmt.Fprintf(stdout, "disarmed #%d; agents watch will not land it\n", n)
		return nil
	}
	if err := env.releaseQueue(ctx, q, nil); err != nil {
		return err
	}
	if err := env.clearOfGraphite(ctx, n, q.PRs); err != nil {
		return err
	}
	if err := env.unmark(ctx, rec.Ticket, n, nil); err != nil {
		return err
	}
	_, _ = fmt.Fprintf(stdout, "dequeued %s; safe to push\n", prRefs(q.PRs))
	return nil
}

func (env *Env) dequeueLabeled(ctx context.Context, top int, stdout io.Writer) error {
	open, err := env.openPulls(ctx)
	if err != nil {
		return err
	}
	chain := stackChain(open, top)
	var prs []int
	for _, p := range chain {
		if p.labeled(env.Config.QueueLabel) {
			prs = append(prs, p.Number)
		}
	}
	if len(prs) > 0 {
		if err := env.releaseQueue(ctx, &Queue{Top: top, PRs: prs}, nil); err != nil {
			return err
		}
	}
	if err := env.clearOfGraphite(ctx, top, numbers(chain)); err != nil {
		return err
	}
	if len(prs) == 0 {
		_, _ = fmt.Fprintf(stdout, "no PR of the stack under #%d carries %s; safe to push\n",
			top, env.Config.QueueLabel)
		return nil
	}
	_, _ = fmt.Fprintf(stdout, "dequeued %s; safe to push\n", prRefs(prs))
	return nil
}

func stackChain(open []stackPR, top int) []stackPR {
	byHead := map[string]stackPR{}
	var cur stackPR
	for _, p := range open {
		byHead[p.Head] = p
		if p.Number == top {
			cur = p
		}
	}
	var out []stackPR
	for seen := 0; cur.Number != 0 && seen <= len(open); seen++ {
		out = append([]stackPR{cur}, out...)
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
	return stillHeld(q.Top)
}

func stillHeld(top int) error {
	return dequeueErr(errs.CodeVersionConflict, fmt.Sprintf(
		"Graphite still holds #%d; remove it from the queue in the Graphite app, then rerun", top))
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
	queued := rec.Queued.find(top) != nil
	armed := rec.Armed.find(top) != nil
	empty := len(rec.Queued) == 0 && len(rec.Armed) == 0
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
		h, err := env.readHold(ctx, nums)
		if err != nil || h.queued {
			return h.queued, err
		}
	}
	return false, nil
}

type stackHold struct {
	queued bool
	taken  []int
	clears time.Time
}

func (env *Env) readHold(ctx context.Context, nums []int) (stackHold, error) {
	var h stackHold
	if len(nums) == 0 {
		return h, nil
	}
	prs, err := env.stackPulls(ctx, nums)
	if err != nil {
		return h, err
	}
	drafts, err := env.queueDrafts(ctx)
	if err != nil {
		return h, err
	}
	for _, p := range prs {
		if p.State != "OPEN" {
			continue
		}
		until, taken := env.takenUntil(p.gqlPR, drafts)
		switch {
		case p.labeled(env.Config.QueueLabel) || draftHolds(drafts, p.Number):
			h.queued = true
		case taken:
			landed, err := env.landedBeforeGraphiteClosed(ctx, p, drafts)
			if err != nil {
				return h, err
			}
			if !landed {
				h.take(p.Number, until)
			}
		}
	}
	return h, nil
}

func (h *stackHold) take(pr int, until time.Time) {
	h.taken = append(h.taken, pr)
	if until.After(h.clears) {
		h.clears = until
	}
}

func (env *Env) clearOfGraphite(ctx context.Context, top int, nums []int) error {
	h, err := env.readHold(ctx, nums)
	switch {
	case err != nil:
		return err
	case h.queued:
		return stillHeld(top)
	case len(h.taken) > 0:
		return dequeueErr(errs.CodeVersionConflict, fmt.Sprintf(
			"Graphite took %s and has not opened its draft; the hold clears at %s, then rerun",
			prRefs(h.taken), h.clears.Format(time.RFC3339)))
	}
	return nil
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
