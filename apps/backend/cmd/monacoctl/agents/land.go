package agents

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
)

const (
	stackFields = `number state closedAt baseRefName headRefName headRefOid body mergeCommit{oid} ` + labelFields + `
commits(last:1){nodes{commit{` + commitChecks + `}}}
timelineItems(itemTypes:[UNLABELED_EVENT],last:20){nodes{__typename ... on UnlabeledEvent{createdAt label{name}}}}`
	settleAfter = time.Minute
	repoQuery   = "query($owner:String!,$name:String!){repository(owner:$owner,name:$name){"

	recomputeEvery = 3 * time.Second
	recomputeFor   = 90 * time.Second
)

type Queue struct {
	Top int   `json:"top"`
	PRs []int `json:"prs"`
}

type stackPR struct {
	gqlPR
	Base        string `json:"baseRefName"`
	Head        string `json:"headRefName"`
	MergeCommit struct {
		OID string `json:"oid"`
	} `json:"mergeCommit"`
}

type gqlOID struct {
	OID string `json:"oid"`
}

type mergeView struct {
	Repository struct {
		PullRequest struct {
			Mergeable string `json:"mergeable"`
			BaseOID   string `json:"baseRefOid"`
			Merge     struct {
				Parents struct {
					Nodes []gqlOID `json:"nodes"`
				} `json:"parents"`
			} `json:"potentialMergeCommit"`
		} `json:"pullRequest"`
	} `json:"repository"`
}

func landStackCmd(ctx context.Context, env *Env, args []string, stdout io.Writer) error {
	n, err := prArg(args, "land-stack <top-pr>")
	if err != nil {
		return err
	}
	tops, err := env.stackPulls(ctx, []int{n})
	if err != nil {
		return err
	}
	ticket, ok := PR{Body: tops[0].Body}.Ticket()
	if !ok {
		return landErr(fmt.Sprintf("#%d links no ticket; its body needs \"Part of #N\" or \"Closes #N\"", n))
	}
	rec, err := env.record(ctx, ticket)
	if err != nil {
		return err
	}
	if rec.Queued != nil {
		if rec.Queued.Top != n {
			return landErr(fmt.Sprintf("#%d already has #%d queued", ticket, rec.Queued.Top))
		}
		if done, err := env.settleQueued(ctx, rec, stdout); done || err != nil {
			return err
		}
	}
	stack, err := env.stackOf(ctx, rec.Worktree, n, stdout)
	if err != nil {
		return err
	}
	if waiting := waitingOn(stack); len(waiting) > 0 {
		_, _ = fmt.Fprintf(stdout, "not landing #%d; waiting on %s\n", n, strings.Join(waiting, ", "))
		return nil
	}
	return env.land(ctx, rec, stack, stdout)
}

func (env *Env) settleQueued(ctx context.Context, rec Record, stdout io.Writer) (bool, error) {
	queued, err := env.stackPulls(ctx, rec.Queued.PRs)
	if err != nil {
		return true, err
	}
	landed, err := env.landedEach(ctx, queued)
	if err != nil {
		return true, err
	}
	drafts, err := env.openQueueDrafts(ctx)
	if err != nil {
		return true, err
	}
	if !env.ejected(queued, landed, drafts) {
		return true, env.settle(ctx, rec, queued, landed, stdout)
	}
	_, _ = fmt.Fprintf(stdout, "#%d left the Graphite merge queue; relanding its stack\n", rec.Queued.Top)
	return false, env.unmark(ctx, rec)
}

func walkStack(open []stackPR, top int, trunk string) ([]stackPR, error) {
	byHead := map[string]stackPR{}
	var cur stackPR
	for _, p := range open {
		byHead[p.Head] = p
		if p.Number == top {
			cur = p
		}
	}
	if cur.Number == 0 {
		return nil, landErr(fmt.Sprintf("#%d is not an open PR", top))
	}
	stack := []stackPR{cur}
	for cur.Base != trunk {
		parent, ok := byHead[cur.Base]
		if !ok || len(stack) > len(open) {
			return nil, landErr(fmt.Sprintf("#%d's base %s is neither %s nor an open PR", cur.Number, cur.Base, trunk))
		}
		stack = append([]stackPR{parent}, stack...)
		cur = parent
	}
	return stack, nil
}

func (env *Env) stackOf(ctx context.Context, worktree string, top int, stdout io.Writer) ([]stackPR, error) {
	open, err := env.openPulls(ctx)
	if err != nil {
		return nil, err
	}
	walked, err := walkStack(open, top, env.Config.FeatureBranch)
	if err != nil {
		return nil, err
	}
	return env.graphiteStack(ctx, worktree, open, walked, stdout), nil
}

func (env *Env) graphiteStack(
	ctx context.Context,
	worktree string,
	open, walked []stackPR,
	stdout io.Writer,
) []stackPR {
	out, err := env.Run(ctx, worktree, "", "gt", "log", "short", "--stack", "--reverse", "--no-interactive")
	if err != nil {
		_, _ = fmt.Fprintf(stdout, "gt log in %s failed (%v); landing the GitHub base chain\n", worktree, err)
		return walked
	}
	byHead := map[string]stackPR{}
	for _, p := range open {
		byHead[p.Head] = p
	}
	top := walked[len(walked)-1].Number
	trunk := env.Config.FeatureBranch
	prev := trunk
	var stack []stackPR
	for line := range strings.Lines(string(out)) {
		if p, ok := prOn(line, byHead); ok && (p.Base == trunk || p.Base == prev) {
			stack = append(stack, p)
			prev = p.Head
		}
		if len(stack) > 0 && stack[len(stack)-1].Number == top {
			if len(stack) > len(walked) {
				return stack
			}
			break
		}
	}
	return walked
}

func prOn(line string, byHead map[string]stackPR) (stackPR, bool) {
	for _, f := range strings.Fields(line) {
		if p, ok := byHead[f]; ok {
			return p, true
		}
	}
	return stackPR{}, false
}

func waitingOn(stack []stackPR) []string {
	var out []string
	for _, p := range stack {
		t := p.flat("")
		var why []string
		if t.Stage1 != "success" {
			why = append(why, "stage 1 "+orMissing(t.Stage1))
		}
		if t.Verify != "success" {
			why = append(why, "verify "+orMissing(t.Verify))
		}
		if t.Format != "" && t.Format != "success" {
			why = append(why, "PR format "+t.Format)
		}
		if len(why) > 0 {
			out = append(out, fmt.Sprintf("#%d (%s)", p.Number, strings.Join(why, ", ")))
		}
	}
	return out
}

func orMissing(s string) string {
	if s == "" {
		return "missing"
	}
	return s
}

func (env *Env) land(ctx context.Context, rec Record, stack []stackPR, stdout io.Writer) error {
	nums := make([]int, len(stack))
	for i, p := range stack {
		nums[i] = p.Number
	}
	bottom, top := nums[0], nums[len(nums)-1]
	if err := env.cleanRuns(ctx, stack, stdout); err != nil {
		return err
	}
	ready, err := env.awaitMergeable(ctx, bottom, top)
	switch {
	case err != nil:
		return err
	case !ready:
		_, _ = fmt.Fprintf(stdout, "not landing #%d; GitHub has not recomputed #%d's merge commit onto %s in %s. "+
			"Run land-stack %d again\n", top, bottom, env.Config.FeatureBranch, recomputeFor, top)
		return nil
	}
	for _, n := range nums {
		if err := env.addLabel(ctx, n); err != nil {
			return landFailed(err)
		}
	}
	rec.Queued = &Queue{Top: top, PRs: nums}
	rec.Changed = env.Now()
	if err := env.storeRecord(ctx, rec); err != nil {
		return err
	}
	_, _ = fmt.Fprintf(stdout, "queued %s\nfollow it: monacoctl agents watch (under Claude Code's Monitor tool)\n",
		prRefs(nums))
	return env.reportDraft(ctx, stack, stdout)
}

func prRefs(nums []int) string {
	refs := make([]string, len(nums))
	for i, n := range nums {
		refs[i] = "#" + strconv.Itoa(n)
	}
	return strings.Join(refs, " ")
}

func (env *Env) awaitMergeable(ctx context.Context, pr, top int) (bool, error) {
	deadline := env.Now().Add(recomputeFor)
	for {
		var view mergeView
		err := env.graphqlGH(ctx, mergeQuery(pr), &view)
		switch {
		case err != nil:
			return false, landFailed(err)
		case view.conflicting():
			return false, landErr(fmt.Sprintf("#%d conflicts with %s. Fix the conflicts with gt modify and "+
				"gt submit --stack --draft, then run land-stack %d", pr, env.Config.FeatureBranch, top))
		case view.ready():
			return true, nil
		case !env.Now().Before(deadline):
			return false, nil
		}
		select {
		case <-ctx.Done():
			return false, landFailed(context.Cause(ctx))
		case <-env.After(recomputeEvery):
		}
	}
}

func mergeQuery(top int) string {
	return fmt.Sprintf(
		"%spullRequest(number:%d){mergeable baseRefOid potentialMergeCommit{parents(first:2){nodes{oid}}}}}}",
		repoQuery, top,
	)
}

func (v mergeView) conflicting() bool {
	return v.Repository.PullRequest.Mergeable == "CONFLICTING"
}

func (v mergeView) ready() bool {
	pr := v.Repository.PullRequest
	return pr.Mergeable == "MERGEABLE" && slices.Contains(pr.Merge.Parents.Nodes, gqlOID{pr.BaseOID})
}

func (env *Env) settle(ctx context.Context, rec Record, prs []stackPR, landed []bool, stdout io.Writer) error {
	top := prs[len(prs)-1]
	if slices.Contains(landed, false) {
		_, _ = fmt.Fprintf(stdout, "#%d is queued in the Graphite merge queue\n", top.Number)
		return nil
	}
	sha := shortSHA(cmp.Or(top.MergeCommit.OID, top.HeadOID))
	if _, err := os.Stat(rec.Worktree); errors.Is(err, fs.ErrNotExist) {
		_, _ = fmt.Fprintf(
			stdout, "#%d merged as %s; no worktree at %s, skipped gt sync\n", top.Number, sha, rec.Worktree,
		)
		return env.unmark(ctx, rec)
	}
	if _, err := env.Run(
		ctx,
		rec.Worktree,
		"",
		"gt",
		"sync",
		"--no-interactive",
		"--delete-all",
		"--no-restack",
	); err != nil {
		first, _, _ := strings.Cut(strings.TrimSpace(err.Error()), "\n")
		_, _ = fmt.Fprintf(stdout, "#%d merged as %s; gt sync failed in %s: %s\n", top.Number, sha, rec.Worktree, first)
		return env.unmark(ctx, rec)
	}
	_, _ = fmt.Fprintf(stdout, "#%d merged as %s; gt sync ran in %s\n", top.Number, sha, rec.Worktree)
	return env.unmark(ctx, rec)
}

func (env *Env) landedEach(ctx context.Context, prs []stackPR) ([]bool, error) {
	out := make([]bool, len(prs))
	for i, p := range prs {
		landed, err := env.landed(ctx, p.closed())
		if err != nil {
			return nil, err
		}
		out[i] = landed
	}
	return out, nil
}

const (
	prQueued  = "queued"
	prLanded  = "landed"
	prEjected = "ejected"
)

func (env *Env) queueState(p stackPR, landed bool, drafts []queueDraft) string {
	switch {
	case landed:
		return prLanded
	case p.State != "OPEN":
		return prEjected
	case p.labeled(env.Config.QueueLabel),
		slices.ContainsFunc(drafts, func(d queueDraft) bool { return d.State == "OPEN" && d.tests(p.Number) }),
		env.justUnlabeled(p):
		return prQueued
	default:
		return prEjected
	}
}

func (env *Env) ejected(prs []stackPR, landed []bool, drafts []queueDraft) bool {
	for i, p := range prs {
		if env.queueState(p, landed[i], drafts) == prEjected {
			return true
		}
	}
	return false
}

func (env *Env) unmark(ctx context.Context, rec Record) error {
	rec.Queued = nil
	rec.Changed = env.Now()
	return env.storeRecord(ctx, rec)
}

func (env *Env) justUnlabeled(p stackPR) bool {
	events := p.TimelineItems.Nodes
	for j := len(events) - 1; j >= 0; j-- {
		if events[j].Label.Name == env.Config.QueueLabel {
			return env.Now().Sub(events[j].CreatedAt) < settleAfter
		}
	}
	return false
}

func (env *Env) unqueueEjected(ctx context.Context, rs []Record, stdout io.Writer) error {
	drafts, err := env.openQueueDrafts(ctx)
	if err != nil {
		return err
	}
	for _, r := range rs {
		if r.Queued == nil {
			continue
		}
		prs, err := env.stackPulls(ctx, r.Queued.PRs)
		if err != nil {
			return err
		}
		landed, err := env.landedEach(ctx, prs)
		if err != nil {
			return err
		}
		if !env.ejected(prs, landed, drafts) {
			continue
		}
		_, _ = fmt.Fprintf(stdout, "unqueued: #%d; #%d left the Graphite merge queue. "+
			"Fix the stack with gt modify and gt submit --stack --draft, then run land-stack %d\n",
			r.Ticket, r.Queued.Top, r.Queued.Top)
		if err := env.unmark(ctx, r); err != nil {
			return err
		}
	}
	return nil
}

func (env *Env) graphqlGH(ctx context.Context, query string, out any) error {
	if env.useREST() && !checkPageQuery(query) {
		return env.restQuery(ctx, query, out)
	}
	err := env.graphqlCLI(ctx, query, out)
	if !graphqlCLIDenied(err) {
		return err
	}
	env.markREST()
	if checkPageQuery(query) {
		return err
	}
	return env.restQuery(ctx, query, out)
}

func (env *Env) graphqlCLI(ctx context.Context, query string, out any) error {
	owner, name, _ := strings.Cut(env.Config.Repo, "/")
	raw, err := env.Run(ctx, env.Work, "", "gh", "api", "graphql",
		"-f", "query="+query, "-f", "owner="+owner, "-f", "name="+name)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(raw, &struct {
		Data any `json:"data"`
	}{out}); err != nil {
		return fmt.Errorf("decode gh api graphql: %w", err)
	}
	return nil
}

func (env *Env) openPulls(ctx context.Context) ([]stackPR, error) {
	var data struct {
		Repository struct {
			Open struct {
				Nodes []stackPR `json:"nodes"`
			} `json:"open"`
		} `json:"repository"`
	}
	q := repoQuery + "open: pullRequests(states:OPEN,first:100){nodes{" + stackFields + "}}}}"
	if err := env.graphqlGH(ctx, q, &data); err != nil {
		return nil, err
	}
	open := data.Repository.Open.Nodes
	return open, env.readStackChecks(ctx, open)
}

func (env *Env) readStackChecks(ctx context.Context, prs []stackPR) error {
	var commits []*gqlCommit
	for i := range prs {
		commits = append(commits, prs[i].commits()...)
	}
	return env.readChecks(ctx, commits, env.graphqlGH)
}

func (env *Env) stackPulls(ctx context.Context, nums []int) ([]stackPR, error) {
	var b strings.Builder
	b.WriteString(repoQuery)
	for _, n := range nums {
		_, _ = fmt.Fprintf(&b, "p%d: pullRequest(number:%d){...pr} ", n, n)
	}
	b.WriteString("}}\nfragment pr on PullRequest{" + stackFields + "}")
	var data struct {
		Repository map[string]*stackPR `json:"repository"`
	}
	if err := env.graphqlGH(ctx, b.String(), &data); err != nil {
		return nil, err
	}
	out := make([]stackPR, len(nums))
	for i, n := range nums {
		p := data.Repository["p"+strconv.Itoa(n)]
		if p == nil {
			return nil, detailErr(errs.CodeNotFound, "monacoctl.agents.land-stack", fmt.Sprintf("#%d is not a PR", n))
		}
		out[i] = *p
	}
	return out, env.readStackChecks(ctx, out)
}

func landErr(detail string) error {
	return detailErr(errs.CodeInvalidInput, "monacoctl.agents.land-stack", detail)
}

func landFailed(err error) error {
	return fmt.Errorf("%w; the stack is not marked queued: run land-stack again, which relabels every PR", err)
}
