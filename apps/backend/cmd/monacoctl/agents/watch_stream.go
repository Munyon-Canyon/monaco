package agents

import (
	"cmp"
	"context"
	"fmt"
	"io"
	"slices"
	"strings"
	"time"
)

const (
	watchUse     = "watch [--once] [--every <duration of 10s or more>]"
	defaultEvery = 30 * time.Second
	minEvery     = 10 * time.Second
	droppedWhy   = "dropped from the Graphite merge queue"
)

func watchArgs(args []string) (bool, time.Duration, error) {
	once, every := false, defaultEvery
	for i := 0; i < len(args); i++ {
		switch {
		case args[i] == "--once":
			once = true
		case args[i] == "--every" && i+1 < len(args):
			d, err := time.ParseDuration(args[i+1])
			if err != nil || d < minEvery {
				return false, 0, usageError(watchUse)
			}
			every = d
			i++
		default:
			return false, 0, usageError(watchUse)
		}
	}
	return once, every, nil
}

func streams(args []string) bool {
	return args[0] == "watch" && !slices.Contains(args[1:], "--once")
}

type stream struct {
	env     *Env
	prev    map[string]bool
	ejected map[int]bool
	blocks  map[string]string
}

func (env *Env) watchStream(ctx context.Context, every time.Duration, out io.Writer) error {
	s := &stream{env: env, prev: map[string]bool{}, ejected: map[int]bool{}, blocks: map[string]string{}}
	for {
		next := map[string]bool{}
		for _, item := range s.round(ctx) {
			if !s.prev[item] && !next[item] {
				_, _ = io.WriteString(out, item+"\n")
			}
			next[item] = true
		}
		s.prev = next
		select {
		case <-ctx.Done():
			return nil
		case <-env.After(every):
		}
	}
}

func (s *stream) round(ctx context.Context) []string {
	env := s.env
	rs, err := env.records()
	if err != nil {
		return []string{watchErr("", err)}
	}
	items, _, err := env.ownerLines(ctx, rs)
	if err != nil {
		items = append(items, watchErr("", err))
	}
	data, err := env.watchData(ctx)
	if err != nil {
		items = append(items, watchErr("", err))
	}
	var queued []int
	for _, r := range rs {
		if r.Queued != nil {
			queued = append(queued, r.Queued.PRs...)
			items = append(items, s.stack(ctx, r, data.drafts)...)
		}
	}
	items = append(items, draftLines(data.drafts, queued)...)
	return append(items, s.failures(ctx, data)...)
}

func (s *stream) stack(ctx context.Context, r Record, drafts []queueDraft) []string {
	env, top := s.env, r.Queued.Top
	prs, err := env.stackPulls(ctx, r.Queued.PRs)
	if err != nil {
		return []string{watchErr(fmt.Sprintf("stack #%d: ", top), err)}
	}
	each, err := env.landedEach(ctx, prs)
	if err != nil {
		return []string{watchErr(fmt.Sprintf("stack #%d: ", top), err)}
	}
	var items []string
	landed, out := 0, stackPR{}
	for i, p := range prs {
		state := env.queueState(p, each[i])
		items = append(items, fmt.Sprintf("#%d %s", p.Number, state))
		switch state {
		case prLanded:
			landed++
		case prEjected:
			if out.Number == 0 {
				out = p
			}
		}
	}
	wasOut := s.ejected[r.Ticket]
	delete(s.ejected, r.Ticket)
	switch {
	case landed == len(prs):
		if err := env.settle(ctx, r, prs, each, io.Discard); err != nil {
			return append(items, watchErr(fmt.Sprintf("settle #%d: ", top), err))
		}
		return append(items, fmt.Sprintf("stack #%d landed (%s)", top, prRefs(r.Queued.PRs)))
	case out.Number == 0:
		return items
	case !wasOut:
		s.ejected[r.Ticket] = true
		return items
	}
	return append(items, s.eject(ctx, r, out, drafts)...)
}

func (s *stream) eject(ctx context.Context, r Record, out stackPR, drafts []queueDraft) []string {
	top := r.Queued.Top
	if err := s.env.unmark(ctx, r); err != nil {
		return []string{watchErr(fmt.Sprintf("unmark #%d: ", top), err)}
	}
	why := "left the Graphite merge queue"
	if out.State == "CLOSED" {
		why = "was closed without landing"
	}
	f := failure{
		PR: out.Number, Head: out.HeadOID, Body: out.Body, Why: why,
		Job: queueJob(out.Number, out.Commits, drafts, time.Time{}),
	}
	return []string{fmt.Sprintf("stack #%d ejected: #%d %s", top, out.Number, why), s.block(ctx, f)}
}

func draftLines(drafts []queueDraft, queued []int) []string {
	var items []string
	for _, d := range drafts {
		if !slices.ContainsFunc(queued, d.tests) {
			continue
		}
		items = append(items, fmt.Sprintf("draft #%d %s", d.Number, strings.ToLower(d.State)))
		for _, c := range d.Commits.Nodes {
			for _, x := range c.Commit.latest() {
				if verdict, done := finished(x); done {
					items = append(items, fmt.Sprintf("draft #%d %s: %s", d.Number, cmp.Or(x.Name, x.Context), verdict))
				}
			}
		}
	}
	return items
}

func finished(x gqlContext) (string, bool) {
	switch {
	case x.Context != "" && x.State == "SUCCESS":
		return "pass", true
	case x.Context != "" && (x.State == "FAILURE" || x.State == "ERROR"):
		return "fail", true
	case x.Context != "" || x.Conclusion == "":
		return "", false
	case x.Conclusion == "SUCCESS" || x.Conclusion == "NEUTRAL" || x.Conclusion == "SKIPPED":
		return "pass", true
	default:
		return "fail", true
	}
}

func (s *stream) failures(ctx context.Context, data watchData) []string {
	env := s.env
	labeled := map[int]bool{}
	for _, p := range data.prs {
		labeled[p.Number] = slices.Contains(p.Labels.Nodes, gqlName{env.Config.QueueLabel})
	}
	queue := queueRuns{label: env.Config.QueueLabel, drafts: data.drafts}
	var items []string
	for _, f := range failures(data.prs, queue, env.Config.FeatureBranch, time.Time{}) {
		if f.Why == droppedWhy && labeled[f.PR] {
			continue
		}
		items = append(items, s.block(ctx, f))
	}
	return items
}

func (s *stream) block(ctx context.Context, f failure) string {
	key := fmt.Sprint(f.PR, f.Why, f.Head, f.Job.DatabaseID)
	if b, ok := s.blocks[key]; ok {
		return b
	}
	b := strings.TrimSuffix(s.env.freshOwnerFor(ctx, f), "\n")
	s.blocks[key] = b
	return b
}

func watchErr(what string, err error) string {
	return "watch error: " + what + cmp.Or(cliText(err), err.Error())
}
