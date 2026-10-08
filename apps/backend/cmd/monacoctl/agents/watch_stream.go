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
	return args[0] == "check" || args[0] == "watch" && !slices.Contains(args[1:], "--once")
}

type stream struct {
	env      *Env
	since    time.Time
	prev     map[string]bool
	ejected  map[int]bool
	blocks   map[string]string
	seenOpen map[int]bool
	reported map[int]bool
	told     map[dropKey]bool
	reran    map[int64]int
	waits    map[int]*graphiteWait
}

type dropKey struct {
	pr   int
	head string
}

type graphiteWait struct {
	since time.Time
	said  bool
}

func newStream(env *Env) *stream {
	return &stream{
		env: env, since: env.Now(), prev: map[string]bool{}, ejected: map[int]bool{}, blocks: map[string]string{},
		seenOpen: map[int]bool{}, reported: map[int]bool{}, told: map[dropKey]bool{},
		reran: map[int64]int{}, waits: map[int]*graphiteWait{},
	}
}

func (env *Env) watchStream(ctx context.Context, every time.Duration, out io.Writer) error {
	s := newStream(env)
	for {
		for _, item := range s.next(ctx) {
			_, _ = io.WriteString(out, item+"\n")
		}
		select {
		case <-ctx.Done():
			return nil
		case <-env.After(every):
		}
	}
}

func (s *stream) next(ctx context.Context) []string {
	s.env.trunk = nil
	seen := map[string]bool{}
	var fresh []string
	items, every := s.round(ctx)
	for _, item := range items {
		if !s.prev[item] && !seen[item] {
			fresh = append(fresh, item)
		}
		seen[item] = true
	}
	s.prev = seen
	return append(fresh, every...)
}

func (s *stream) round(ctx context.Context) ([]string, []string) {
	env := s.env
	rs, err := env.records()
	if err != nil {
		return []string{watchErr("", err)}, nil
	}
	armedLines := env.landEachArmed(ctx, rs, s.reran)
	data, dataErr := env.watchData(ctx)
	if rs, err = env.records(); err != nil {
		return []string{watchErr("", err)}, nil
	}
	items, _, err := env.ownerLines(ctx, rs)
	if err != nil {
		items = append(items, watchErr("", err))
	}
	if dataErr != nil {
		items = append(items, watchErr("", dataErr))
	}
	var queued, armed []int
	for i, r := range rs {
		if line, stale := r.staleLine(); stale {
			items = append(items, line)
		}
		for _, q := range r.Queued {
			queued = append(queued, q.PRs...)
			items = append(items, s.stack(ctx, r, q, data.drafts)...)
		}
		for _, a := range r.Armed {
			armed = append(armed, a.PRs...)
		}
		items = append(items, armedLines[i]...)
		if r.Settled != nil && r.Settled.At.After(s.since) {
			items = append(items, r.Settled.Detail)
		}
	}
	items = append(items, s.draftLines(data.drafts, queued)...)
	every := append(stuckOnGraphiteBase(data.prs, rs, env.Config.QueueLabel), env.silentStalls(ctx, data.prs, rs)...)
	return append(items, s.failures(ctx, data, slices.Concat(queued, armed))...), every
}

func (s *stream) stack(ctx context.Context, r Record, q Queue, drafts []queueDraft) []string {
	env, top := s.env, q.Top
	prs, err := env.stackPulls(ctx, q.PRs)
	if err != nil {
		return []string{watchErr(fmt.Sprintf("stack #%d: ", top), err)}
	}
	each, err := env.landedEach(ctx, prs, drafts)
	if err != nil {
		return []string{watchErr(fmt.Sprintf("stack #%d: ", top), err)}
	}
	var items []string
	landed, out, states := 0, stackPR{}, make([]string, 0, len(prs))
	for i, p := range prs {
		state := env.queueState(p, each[i], drafts)
		states = append(states, state)
		line := fmt.Sprintf("#%d %s", p.Number, state)
		if state == prWaiting {
			line += fmt.Sprintf(" (stage 1 %s)", orMissing(p.flat("").Stage1))
		}
		items = append(items, line)
		switch state {
		case prLanded:
			landed++
		case prEjected:
			if out.Number == 0 {
				out = p
			}
		}
	}
	items = append(items, s.stuck(top, slices.Contains(states, prWaiting), drafts)...)
	wasOut := s.ejected[top]
	delete(s.ejected, top)
	switch {
	case landed == len(prs):
		if err := env.settle(ctx, r, q, prs, each, io.Discard); err != nil {
			return append(items, watchErr(fmt.Sprintf("settle #%d: ", top), err))
		}
		return append(items, landedLine(q))
	case out.Number == 0:
		return items
	case !wasOut:
		s.ejected[top] = true
		return items
	}
	return append(items, s.eject(ctx, r, q, prs, out, drafts)...)
}

func (s *stream) stuck(top int, waiting bool, drafts []queueDraft) []string {
	if !waiting {
		delete(s.waits, top)
		return nil
	}
	w := s.waits[top]
	if w == nil {
		w = &graphiteWait{since: s.env.Now()}
		s.waits[top] = w
	}
	age := s.env.Now().Sub(w.since)
	if w.said || age <= s.env.Config.StuckAfter || draftHolds(drafts, top) || s.queueFull(drafts) {
		return nil
	}
	w.said = true
	return []string{fmt.Sprintf("stack #%d stuck in the queue %s: no live queue draft; "+
		"run monacoctl agents dequeue %d then land-stack %d", top, span(age), top, top)}
}

func (s *stream) queueFull(drafts []queueDraft) bool {
	n := 0
	for _, d := range drafts {
		if d.State == "OPEN" && strings.HasPrefix(d.HeadRefName, draftPrefix) {
			n++
		}
	}
	return s.env.Config.QueueConcurrency > 0 && n >= s.env.Config.QueueConcurrency
}

func (s *stream) eject(
	ctx context.Context,
	r Record,
	q Queue,
	prs []stackPR,
	out stackPR,
	drafts []queueDraft,
) []string {
	env, top := s.env, q.Top
	if err := env.unlabel(ctx, prs); err != nil {
		return []string{watchErr(fmt.Sprintf("eject #%d: ", top), err)}
	}
	if _, held := draftTesting(prs, drafts); held {
		s.ejected[top] = true
		return nil
	}
	again, err := env.requeued(r, q)(ctx)
	if err != nil {
		return []string{watchErr(fmt.Sprintf("eject #%d: ", top), err)}
	}
	if again {
		return []string{requeuedLine(top)}
	}
	line := ejectedLine(top, out)
	if err := env.conclude(ctx, r, q, outcomeEjected, line); err != nil {
		return []string{watchErr(fmt.Sprintf("eject #%d: ", top), err)}
	}
	for _, p := range prs {
		s.reported[p.Number] = true
	}
	f := failure{
		PR: out.Number, Head: out.HeadOID, Body: out.Body, Why: ejectedWhy(out),
		Job: queueJob(out.Number, out.Commits, drafts, time.Time{}),
	}
	return []string{line, s.block(ctx, f)}
}

func (s *stream) draftLines(drafts []queueDraft, queued []int) []string {
	var items []string
	for _, d := range drafts {
		switch {
		case !slices.ContainsFunc(queued, d.tests):
			continue
		case d.State != "OPEN":
			if s.seenOpen[d.Number] {
				items = append(items, fmt.Sprintf("draft #%d closed", d.Number))
			}
			continue
		}
		s.seenOpen[d.Number] = true
		items = append(items, fmt.Sprintf("draft #%d open", d.Number))
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
	case x.Conclusion == "NEUTRAL" || x.Conclusion == "SKIPPED":
		return "", false
	case x.Conclusion == "SUCCESS":
		return "pass", true
	default:
		return "fail", true
	}
}

func (s *stream) failures(ctx context.Context, data watchData, held []int) []string {
	env := s.env
	labeled := map[int]bool{}
	for _, p := range data.prs {
		labeled[p.Number] = slices.Contains(p.Labels.Nodes, gqlName{env.Config.QueueLabel})
	}
	queue := queueRuns{label: env.Config.QueueLabel, drafts: data.drafts, now: env.Now()}
	var items []string
	landed, err := env.landedDrops(ctx, queue, data.prs, s.since)
	trunkUnread := err != nil
	if trunkUnread {
		items = append(items, watchErr("", err))
	}
	queue.landed = landed
	for _, f := range failures(data.prs, queue, env.Config.FeatureBranch, s.since) {
		if f.Why == droppedWhy {
			key := dropKey{f.PR, f.Head}
			if trunkUnread || labeled[f.PR] || s.reported[f.PR] || s.told[key] || slices.Contains(held, f.PR) {
				continue
			}
			s.told[key] = true
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
