package agents

import (
	"context"
	"fmt"
	"slices"
	"strings"
)

const rearmedWhy = "stage 1 went green after a cancelled job disarmed it"

func cancelledStage1(stack []stackPR) bool {
	failing := false
	for _, p := range stack {
		if p.flat("").Stage1 != "failure" {
			continue
		}
		failing = true
		if !p.cancelledJob() {
			return false
		}
	}
	return failing
}

func (p stackPR) cancelledJob() bool {
	cancelled := false
	for _, c := range p.Commits.Nodes {
		for _, x := range c.Commit.latest() {
			switch {
			case !strings.HasPrefix(x.Name, queueCIPrefix) || x.Name == stage1Check:
			case x.Conclusion == "CANCELLED":
				cancelled = true
			case red(x):
				return false
			}
		}
	}
	return cancelled
}

func (env *Env) disarmStage1(ctx context.Context, r Record, top int, stack []stackPR, why string) []string {
	var rearm *Queue
	if cur := r.Armed.find(top); cur != nil && !cur.Rearmed && cancelledStage1(stack) {
		rearm = &Queue{Top: top, PRs: numbers(stack), At: env.Now(), Head: stack[len(stack)-1].HeadOID}
	}
	return env.disarmWith(ctx, r, top, why, rearm)
}

func (env *Env) rearmStacks(ctx context.Context, r Record) []string {
	out := make([]string, 0, len(r.Rearm))
	for _, q := range r.Rearm {
		out = append(out, env.rearmStack(ctx, r.Ticket, q)...)
	}
	return out
}

func (env *Env) rearmStack(ctx context.Context, ticket int, q Queue) []string {
	what := fmt.Sprintf("rearm #%d: ", q.Top)
	prs, err := env.stackPulls(ctx, q.PRs)
	if err != nil {
		return []string{watchErr(what, err)}
	}
	gone := env.rearmGone(prs, q)
	if !gone && !allGreen(prs) {
		return nil
	}
	armed := false
	err = env.updateRecord(ctx, ticket, func(r *Record) { armed = env.rearm(r, q, gone) })
	switch {
	case err != nil:
		return []string{watchErr(what, err)}
	case armed:
		return []string{fmt.Sprintf("armed stack #%d re-armed: %s", q.Top, rearmedWhy)}
	}
	return nil
}

func (env *Env) rearmGone(prs []stackPR, q Queue) bool {
	return slices.ContainsFunc(prs, func(p stackPR) bool {
		return p.State != "OPEN" || p.labeled(env.Config.QueueLabel)
	}) || prs[len(prs)-1].HeadOID != q.Head
}

func (env *Env) rearm(r *Record, q Queue, gone bool) bool {
	cur := r.Rearm.find(q.Top)
	known := cur != nil && cur.At.Equal(q.At)
	if known {
		r.Rearm = r.Rearm.without(q.Top)
	}
	if !known || gone || r.Armed.find(q.Top) != nil || r.Queued.holding(q.Top) != nil {
		return false
	}
	r.Armed = r.Armed.with(Arm{Top: q.Top, PRs: q.PRs, At: env.Now(), Head: q.Head, Rearmed: true})
	r.Changed = env.Now()
	return true
}

func allGreen(prs []stackPR) bool {
	return !slices.ContainsFunc(prs, func(p stackPR) bool {
		return p.Mergeable == conflicting || p.flat("").Stage1 != "success"
	})
}
