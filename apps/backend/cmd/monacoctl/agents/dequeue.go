package agents

import (
	"context"
	"fmt"
	"io"
	"slices"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
)

const (
	dequeueTries  = 3
	dequeueChecks = 2
	dequeueEvery  = 30 * time.Second
	openDrafts    = `query($owner:String!,$name:String!){repository(owner:$owner,name:$name){` +
		`drafts: pullRequests(states:OPEN,last:30,orderBy:{field:UPDATED_AT,direction:ASC}){nodes{` +
		`number state title body headRefName}}}}`
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
	if rec.Queued == nil {
		if err := env.unmark(ctx, rec); err != nil {
			return err
		}
		_, _ = fmt.Fprintf(stdout, "disarmed #%d; agents watch will not land it\n", n)
		return nil
	}
	if err := env.releaseQueue(ctx, rec.Queued); err != nil {
		return err
	}
	if err := env.unmark(ctx, rec); err != nil {
		return err
	}
	_, _ = fmt.Fprintf(stdout, "dequeued %s; safe to push\n", prRefs(rec.Queued.PRs))
	return nil
}

func (env *Env) releaseQueue(ctx context.Context, q *Queue) error {
	for range dequeueTries {
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
	if !queued && !armed {
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
		drafts, err := env.openQueueDrafts(ctx)
		if err != nil {
			return false, err
		}
		for _, d := range drafts {
			if slices.ContainsFunc(nums, d.tests) {
				return true, nil
			}
		}
	}
	return false, nil
}

func dequeueErr(code errs.Code, detail string) error {
	return detailErr(code, "monacoctl.agents.dequeue", detail)
}

func (env *Env) openQueueDrafts(ctx context.Context) ([]queueDraft, error) {
	var data struct {
		Repository struct {
			Drafts struct {
				Nodes []queueDraft `json:"nodes"`
			} `json:"drafts"`
		} `json:"repository"`
	}
	if err := env.graphQL(ctx, openDrafts, &data); err != nil {
		return nil, err
	}
	var open []queueDraft
	for _, d := range data.Repository.Drafts.Nodes {
		if d.State == "OPEN" {
			open = append(open, d)
		}
	}
	return open, nil
}
