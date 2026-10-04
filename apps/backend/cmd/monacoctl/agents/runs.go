package agents

import (
	"context"
	"fmt"
	"io"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
)

const (
	runsEvery = 20 * time.Second
	runsFor   = 30 * time.Minute

	draftEvery = 10 * time.Second
	draftFor   = 3 * time.Minute
	draftHead  = "gtmq_"
)

var draftPRs = regexp.MustCompile(`\(PRs? ([\d, ]+)\)`)

type runsState uint8

const (
	runsClean runsState = iota
	runsPending
	runsBroken
)

func (env *Env) cleanRuns(ctx context.Context, stack []stackPR, stdout io.Writer) error {
	reran := map[int64]int{}
	deadline := env.Now().Add(runsFor)
	for {
		state, err := env.checkRuns(ctx, stack, reran, stdout)
		if err != nil {
			return err
		}
		switch {
		case state == runsClean:
			return nil
		case !env.Now().Before(deadline):
			return landErr(fmt.Sprintf(
				"runs on the stack's heads are still not clean after %s; nothing is labeled", runsFor))
		}
		select {
		case <-ctx.Done():
			return landFailed(context.Cause(ctx))
		case <-env.After(runsEvery):
		}
	}
}

func (env *Env) checkRuns(
	ctx context.Context,
	stack []stackPR,
	reran map[int64]int,
	stdout io.Writer,
) (runsState, error) {
	pending := false
	for _, p := range stack {
		busy, err := env.cleanHeadRuns(ctx, p, reran, stdout)
		if err != nil {
			return runsBroken, err
		}
		pending = pending || busy
	}
	if pending {
		return runsPending, nil
	}
	return runsClean, nil
}

func (env *Env) cleanHeadRuns(ctx context.Context, p stackPR, reran map[int64]int, stdout io.Writer) (bool, error) {
	runs, err := env.GitHub.Runs(ctx, p.HeadOID)
	if err != nil {
		return false, err
	}
	busy := false
	for _, r := range newestPerWorkflow(runs) {
		switch {
		case !r.done():
			busy = true
		case !r.broken():
		case reran[r.ID] == 0:
			ci, err := env.GitHub.runsCI(ctx, r.ID)
			if err != nil {
				return false, err
			}
			if !ci {
				continue
			}
			if err := env.GitHub.Rerun(ctx, r.ID); err != nil {
				return false, landFailed(err)
			}
			reran[r.ID] = max(r.Attempt, 1)
			busy = true
			_, _ = fmt.Fprintf(stdout, "#%d %s: reran %q (run %d, was %s)\n",
				p.Number, shortSHA(p.HeadOID), r.Name, r.ID, r.Conclusion)
		case r.Attempt > reran[r.ID]:
			return false, landErr(fmt.Sprintf("#%d: run %q ended %s again after a rerun: %s; nothing is labeled",
				p.Number, r.Name, r.Conclusion, r.URL))
		default:
			busy = true
		}
	}
	return busy, nil
}

func (env *Env) reportDraft(ctx context.Context, stack []stackPR, stdout io.Writer) error {
	deadline := env.Now().Add(draftFor)
	for {
		drafts, err := env.GitHub.PRs(ctx, "state=open")
		if err != nil {
			return err
		}
		if in, ok := draftOf(drafts, stack); ok {
			return env.describeDraft(ctx, stack, in, stdout)
		}
		if !env.Now().Before(deadline) {
			_, _ = io.WriteString(stdout, env.noDraftLine(stack, drafts))
			return nil
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("wait for the Graphite draft: %w", context.Cause(ctx))
		case <-env.After(draftEvery):
		}
	}
}

func (env *Env) noDraftLine(stack []stackPR, open []PR) string {
	n := 0
	for _, d := range open {
		if strings.HasPrefix(d.Head.Ref, draftHead) {
			n++
		}
	}
	if n == 0 || n < env.Config.QueueConcurrency {
		return fmt.Sprintf("no Graphite draft holds %s after %s; run land-stack again if it stays that way\n",
			prRefs(numbers(stack)), draftFor)
	}
	return fmt.Sprintf("waiting for a Graphite queue slot (%d drafts open)\n", n)
}

func draftOf(open []PR, stack []stackPR) ([]int, bool) {
	for _, d := range open {
		if !strings.HasPrefix(d.Head.Ref, draftHead) {
			continue
		}
		m := draftPRs.FindStringSubmatch(d.Title)
		if m == nil {
			continue
		}
		var in []int
		for f := range strings.FieldsFuncSeq(m[1], func(r rune) bool { return r == ',' || r == ' ' }) {
			n, err := strconv.Atoi(f)
			if err == nil && slices.Contains(numbers(stack), n) {
				in = append(in, n)
			}
		}
		if len(in) > 0 {
			return in, true
		}
	}
	return nil, false
}

func (env *Env) describeDraft(ctx context.Context, stack []stackPR, in []int, stdout io.Writer) error {
	var held []string
	for _, p := range stack {
		if slices.Contains(in, p.Number) {
			continue
		}
		reason, err := env.heldReason(ctx, p)
		if err != nil {
			return err
		}
		held = append(held, fmt.Sprintf("#%d (%s)", p.Number, reason))
	}
	if len(held) == 0 {
		_, _ = fmt.Fprintf(stdout, "queued together: %s\n", prRefs(in))
		return nil
	}
	_, _ = fmt.Fprintf(stdout, "Graphite queued only %s; held back: %s\n",
		prRefs(in), strings.Join(held, ", "))
	return nil
}

func (env *Env) heldReason(ctx context.Context, p stackPR) (string, error) {
	runs, err := env.GitHub.Runs(ctx, p.HeadOID)
	if err != nil {
		return "", err
	}
	n := 0
	for _, r := range newestPerWorkflow(runs) {
		if r.broken() {
			n++
		}
	}
	if n > 0 {
		return fmt.Sprintf("%d cancelled or failed runs on its head", n), nil
	}
	return "no cancelled or failed run on its head; Graphite gave no reason", nil
}

func numbers(stack []stackPR) []int {
	out := make([]int, len(stack))
	for i, p := range stack {
		out[i] = p.Number
	}
	return out
}
