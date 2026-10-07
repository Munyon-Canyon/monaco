package agents

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
)

const (
	stackFields = `number state mergeable closedAt baseRefName headRefName headRefOid body mergeCommit{oid} ` + labelFields + `
commits(last:1){nodes{commit{` + commitChecks + `}}}
timelineItems(itemTypes:[UNLABELED_EVENT],last:20){nodes{__typename ... on UnlabeledEvent{createdAt label{name} actor{login}}}}`
	settleAfter = time.Minute
	takenFor    = runsFor
	repoQuery   = "query($owner:String!,$name:String!){repository(owner:$owner,name:$name){"
)

type Queue struct {
	Top int       `json:"top"`
	PRs []int     `json:"prs"`
	At  time.Time `json:"at,omitzero"`
}

type Arm = Queue

type stackPR struct {
	gqlPR
	Base        string `json:"baseRefName"`
	Head        string `json:"headRefName"`
	Mergeable   string `json:"mergeable"`
	MergeCommit struct {
		OID string `json:"oid"`
	} `json:"mergeCommit"`
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
	if q := rec.Queued.holding(n); q != nil {
		if q.Top != n {
			return landErr(fmt.Sprintf("#%d already has #%d queued", ticket, q.Top))
		}
		if done, err := env.settleQueued(ctx, rec, *q, stdout); done || err != nil {
			return err
		}
		rec.Queued = rec.Queued.without(n)
	}
	stack, dir, err := env.stackOf(ctx, rec, n, stdout)
	if err != nil {
		return err
	}
	flows, err := env.flowGate(ctx, rec, stack)
	if err != nil {
		return err
	}
	if waiting := append(waitingOn(stack), flows.entries()...); len(waiting) > 0 {
		return env.arm(ctx, rec, stack, waiting, stdout)
	}
	return env.land(ctx, rec, dir, stack, stdout)
}

func (env *Env) arm(ctx context.Context, rec Record, stack []stackPR, waiting []string, stdout io.Writer) error {
	top := stack[len(stack)-1].Number
	if blocker(stack) != "" {
		_, _ = fmt.Fprintf(stdout, "not landing #%d; waiting on %s\n", top, strings.Join(waiting, ", "))
		return nil
	}
	err := env.updateRecord(ctx, rec.Ticket, func(r *Record) {
		r.Armed = r.Armed.with(Arm{Top: top, PRs: numbers(stack), At: env.Now()})
		r.Changed = env.Now()
	})
	if err != nil {
		return err
	}
	_, _ = fmt.Fprintf(stdout, "armed #%d; agents watch lands it once stage 1 passes (waiting on %s)\n",
		top, strings.Join(waiting, ", "))
	return nil
}

func blocker(stack []stackPR) string {
	for _, p := range stack {
		if p.Mergeable == conflicting {
			return conflictLine(p.Number, p.Base)
		}
	}
	for _, p := range stack {
		if p.flat("").Stage1 == "failure" {
			return fmt.Sprintf("#%d stage 1 failed", p.Number)
		}
	}
	return ""
}

func (env *Env) landArmed(ctx context.Context, r Record, a Arm, reran map[int64]int) []string {
	fresh, err := env.localRecord(r.Ticket)
	if err != nil {
		return []string{watchErr(fmt.Sprintf("armed stack #%d: ", a.Top), err)}
	}
	cur := fresh.Armed.find(a.Top)
	if cur == nil || !cur.At.Equal(a.At) {
		return nil
	}
	return env.landFresh(ctx, fresh, a.Top, reran)
}

func (env *Env) landFresh(ctx context.Context, r Record, top int, reran map[int64]int) []string {
	var out strings.Builder
	stack, dir, err := env.stackOf(ctx, r, top, &out)
	if err != nil {
		return []string{watchErr(fmt.Sprintf("armed stack #%d: ", top), err)}
	}
	if lines, handled := env.migrationStep(ctx, r, dir, stack); handled {
		return lines
	}
	if failed := blocker(stack); failed != "" {
		return env.disarm(ctx, r, top, failed)
	}
	if len(waitingOn(stack)) > 0 {
		return nil
	}
	flows, err := env.flowGate(ctx, r, stack)
	switch {
	case err != nil:
		return env.disarm(ctx, r, top, cmp.Or(cliText(err), err.Error()))
	case flows.started:
		return []string{fmt.Sprintf("armed stack #%d started %s", top, flows.waiting)}
	case flows.waiting != "":
		return nil
	}
	state, err := env.checkRuns(ctx, stack, reran, &out)
	if err != nil {
		disarmed := env.disarm(ctx, r, top, cmp.Or(cliText(err), err.Error()))
		items := make([]string, 0, len(disarmed)+1)
		items = append(items, fmt.Sprintf("armed stack #%d landing", top))
		return append(items, disarmed...)
	}
	if state == runsPending {
		return nil
	}
	err = env.queue(ctx, r, dir, stack, &out)
	items := []string{fmt.Sprintf("armed stack #%d landing", top)}
	for line := range strings.Lines(out.String()) {
		items = append(items, strings.TrimSuffix(line, "\n"))
	}
	if err != nil {
		return append(items, env.disarm(ctx, r, top, cmp.Or(cliText(err), err.Error()))...)
	}
	return items
}

func (env *Env) disarm(ctx context.Context, r Record, top int, why string) []string {
	err := env.updateRecord(ctx, r.Ticket, func(r *Record) {
		r.Armed = r.Armed.without(top)
		r.Changed = env.Now()
	})
	if err != nil {
		return []string{watchErr(fmt.Sprintf("disarm #%d: ", top), err)}
	}
	return []string{fmt.Sprintf("armed stack #%d disarmed: %s", top, why)}
}

func (env *Env) settleQueued(ctx context.Context, rec Record, q Queue, stdout io.Writer) (bool, error) {
	queued, err := env.stackPulls(ctx, q.PRs)
	if err != nil {
		return true, err
	}
	drafts, err := env.queueDrafts(ctx)
	if err != nil {
		return true, err
	}
	landed, err := env.landedEach(ctx, queued, drafts)
	if err != nil {
		return true, err
	}
	if !env.ejected(queued, landed, drafts) {
		return true, env.settle(ctx, rec, q, queued, landed, stdout)
	}
	_, _ = fmt.Fprintf(stdout, "#%d left the Graphite merge queue; relanding its stack\n", q.Top)
	return false, env.unmark(ctx, rec.Ticket, q.Top, nil)
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

func (env *Env) stackOf(ctx context.Context, rec Record, top int, stdout io.Writer) ([]stackPR, string, error) {
	open, err := env.openPulls(ctx)
	if err != nil {
		return nil, "", err
	}
	walked, err := walkStack(open, top, env.Config.FeatureBranch)
	if err != nil {
		return nil, "", err
	}
	dir := env.workdir(ctx, rec, walked[len(walked)-1].Head, stdout)
	return env.graphiteStack(ctx, dir, open, walked, stdout), dir, nil
}

func (env *Env) workdir(ctx context.Context, rec Record, branch string, stdout io.Writer) string {
	if worktreeHere(rec) {
		return rec.Worktree
	}
	root := filepath.Dir(env.Common)
	dir := cmp.Or(env.checkout(ctx, root, branch), root)
	_, _ = fmt.Fprintf(stdout, "record %d's worktree %s is not on this machine; using %s\n",
		rec.Ticket, rec.Worktree, dir)
	return dir
}

func (env *Env) checkout(ctx context.Context, root, branch string) string {
	out, _ := env.Run(ctx, root, "", "git", "worktree", "list", "--porcelain")
	var path string
	for line := range strings.Lines(string(out)) {
		line = strings.TrimSuffix(line, "\n")
		if p, ok := strings.CutPrefix(line, "worktree "); ok {
			path = p
		}
		if line == "branch refs/heads/"+branch {
			return path
		}
	}
	return ""
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
		if p.Mergeable == conflicting {
			out = append(out, conflictLine(p.Number, p.Base))
			continue
		}
		t := p.flat("")
		var why []string
		if t.Stage1 != "success" {
			why = append(why, "stage 1 "+orMissing(t.Stage1))
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

func (env *Env) land(ctx context.Context, rec Record, dir string, stack []stackPR, stdout io.Writer) error {
	if err := env.cleanRuns(ctx, stack, stdout); err != nil {
		return err
	}
	if err := env.queue(ctx, rec, dir, stack, stdout); err != nil {
		return err
	}
	return env.reportDraft(ctx, stack, stdout)
}

func (env *Env) queue(ctx context.Context, rec Record, dir string, stack []stackPR, stdout io.Writer) error {
	nums := numbers(stack)
	top := nums[len(nums)-1]
	if err := env.mergeable(ctx, dir, stack[0], top); err != nil {
		return err
	}
	for _, n := range slices.Backward(nums) {
		if err := env.addLabel(ctx, n); err != nil {
			return landFailed(err)
		}
	}
	err := env.updateRecord(ctx, rec.Ticket, func(r *Record) {
		r.Queued = r.Queued.with(Queue{Top: top, PRs: nums, At: env.Now()})
		r.Armed = r.Armed.without(top)
		r.Settled = nil
		r.Changed = env.Now()
	})
	if err != nil {
		return err
	}
	_, _ = fmt.Fprintf(stdout, "queued %s\nfollow it: monacoctl agents watch (under Claude Code's Monitor tool)\n",
		prRefs(nums))
	return nil
}

func prRefs(nums []int) string {
	refs := make([]string, len(nums))
	for i, n := range nums {
		refs[i] = "#" + strconv.Itoa(n)
	}
	return strings.Join(refs, " ")
}

func (env *Env) mergeable(ctx context.Context, worktree string, bottom stackPR, top int) error {
	trunk := env.Config.FeatureBranch
	if _, err := env.Run(ctx, worktree, "", "git", "fetch", "--quiet", "origin", trunk, bottom.HeadOID); err != nil {
		return landFailed(err)
	}
	_, err := env.Run(ctx, worktree, "", "git", "merge-tree", "--write-tree", "origin/"+trunk, bottom.HeadOID)
	var exit *exec.ExitError
	switch {
	case err == nil:
		return nil
	case errors.As(err, &exit) && exit.ExitCode() == 1:
		return landErr(fmt.Sprintf("#%d conflicts with %s. Fix the conflicts with gt modify and "+
			"gt submit --stack --draft, then run land-stack %d", bottom.Number, trunk, top))
	default:
		return landFailed(err)
	}
}

func (env *Env) settle(ctx context.Context, rec Record, q Queue, prs []stackPR, landed []bool, stdout io.Writer) error {
	top := prs[len(prs)-1]
	if slices.Contains(landed, false) {
		_, _ = fmt.Fprintf(stdout, "#%d is queued in the Graphite merge queue\n", top.Number)
		return nil
	}
	sha := shortSHA(cmp.Or(top.MergeCommit.OID, top.HeadOID))
	_, _ = fmt.Fprintf(stdout, "#%d merged as %s\n", top.Number, sha)
	return env.conclude(ctx, rec, q, outcomeLanded, landedLine(q))
}

func landedLine(q Queue) string {
	return fmt.Sprintf("stack #%d landed (%s)", q.Top, prRefs(q.PRs))
}

func (env *Env) landedEach(ctx context.Context, prs []stackPR, drafts []queueDraft) ([]bool, error) {
	out := make([]bool, len(prs))
	for i, p := range prs {
		landed, err := env.landed(ctx, p.closed())
		if err == nil && !landed && p.State == "OPEN" && env.queueState(p, false, drafts) == prEjected {
			landed, err = env.landedBeforeGraphiteClosed(ctx, p, drafts)
		}
		if err != nil {
			return nil, err
		}
		out[i] = landed
	}
	return out, nil
}

func (env *Env) landedBeforeGraphiteClosed(ctx context.Context, p stackPR, drafts []queueDraft) (bool, error) {
	at, _, ok := p.TimelineItems.removal(env.Config.QueueLabel)
	if !ok {
		return false, nil
	}
	return env.draftLanded(ctx, p.Number, at, drafts)
}

func (env *Env) draftLanded(ctx context.Context, pr int, at time.Time, drafts []queueDraft) (bool, error) {
	if squashed, err := env.squashed(ctx, closedPR{Number: pr, ClosedAt: at}); err != nil || squashed {
		return squashed, err
	}
	for _, d := range drafts {
		if !d.runs(pr, at) {
			continue
		}
		if d.State == "MERGED" {
			return true, nil
		}
		if d.HeadRefOID == "" {
			continue
		}
		if in, err := env.GitHub.inBranch(ctx, env.Config.FeatureBranch, d.HeadRefOID); err != nil || in {
			return in, err
		}
	}
	return false, nil
}

const (
	prQueued   = "queued"
	prWaiting  = "labeled, waiting for Graphite"
	prSettling = "label just removed, waiting for Graphite"
	prTaken    = "taken by Graphite, waiting for a draft"
	prLanded   = "landed"
	prEjected  = "ejected"
)

func (env *Env) queueState(p stackPR, landed bool, drafts []queueDraft) string {
	switch {
	case landed:
		return prLanded
	case p.State != "OPEN":
		return prEjected
	case draftHolds(drafts, p.Number):
		return prQueued
	case p.labeled(env.Config.QueueLabel):
		return prWaiting
	case env.waitsForDraft(p.gqlPR, drafts):
		return prTaken
	case env.justUnlabeled(p):
		return prSettling
	default:
		return prEjected
	}
}

func (env *Env) ejected(prs []stackPR, landed []bool, drafts []queueDraft) bool {
	_, ok := env.firstEjected(prs, landed, drafts)
	return ok
}

func (env *Env) firstEjected(prs []stackPR, landed []bool, drafts []queueDraft) (stackPR, bool) {
	for i, p := range prs {
		if env.queueState(p, landed[i], drafts) == prEjected {
			return p, true
		}
	}
	return stackPR{}, false
}

func ejectedWhy(out stackPR) string {
	if out.State == "CLOSED" {
		return "was closed without landing"
	}
	return "left the Graphite merge queue"
}

func (env *Env) ejectStack(
	ctx context.Context,
	rec Record,
	q Queue,
	out stackPR,
	prs []stackPR,
	drafts []queueDraft,
) (string, bool, error) {
	if pr, held := draftTesting(prs, drafts); held {
		if err := env.unlabel(ctx, prs); err != nil {
			return "", false, err
		}
		return leftQueuedLine(q.Top, pr), true, nil
	}
	stop := env.requeued(rec, q)
	err := env.releaseQueue(ctx, &q, stop)
	if errors.Is(err, errRequeued) {
		return requeuedLine(q.Top), true, nil
	}
	if err != nil {
		return "", false, err
	}
	if again, err := stop(ctx); err != nil || again {
		return requeuedLine(q.Top), again, err
	}
	line := ejectedLine(q.Top, out)
	return line, false, env.conclude(ctx, rec, q, outcomeEjected, line)
}

func requeuedLine(top int) string {
	return fmt.Sprintf("stack #%d was re-queued during its release; left it queued", top)
}

func leftQueuedLine(top, pr int) string {
	return fmt.Sprintf("stack #%d left queued: an open Graphite draft still tests #%d", top, pr)
}

func draftTesting(prs []stackPR, drafts []queueDraft) (int, bool) {
	i := slices.IndexFunc(prs, func(p stackPR) bool { return draftHolds(drafts, p.Number) })
	if i < 0 {
		return 0, false
	}
	return prs[i].Number, true
}

func (env *Env) unlabel(ctx context.Context, prs []stackPR) error {
	for _, p := range prs {
		if p.labeled(env.Config.QueueLabel) {
			if err := env.removeLabel(ctx, p.Number); err != nil {
				return err
			}
		}
	}
	return nil
}

func ejectedLine(top int, out stackPR) string {
	return fmt.Sprintf("stack #%d ejected: #%d %s", top, out.Number, ejectedWhy(out))
}

func (env *Env) requeued(rec Record, q Queue) func(context.Context) (bool, error) {
	return func(ctx context.Context) (bool, error) {
		cur, err := env.record(ctx, rec.Ticket)
		if err != nil {
			return false, err
		}
		cur2 := cur.Queued.find(q.Top)
		return cur2 != nil && !cur2.At.Equal(q.At), nil
	}
}

func (env *Env) conclude(ctx context.Context, rec Record, q Queue, outcome Outcome, detail string) error {
	settled := &Settlement{Top: q.Top, PRs: q.PRs, Outcome: outcome, Detail: detail, At: env.Now()}
	return env.unmark(ctx, rec.Ticket, q.Top, settled)
}

func (env *Env) unmark(ctx context.Context, ticket, top int, settled *Settlement) error {
	return env.updateRecord(ctx, ticket, func(r *Record) {
		r.Queued = r.Queued.without(top)
		r.Armed = r.Armed.without(top)
		if settled != nil {
			r.Settled = settled
		}
		r.Changed = env.Now()
	})
}

func (env *Env) justUnlabeled(p stackPR) bool {
	at, _, ok := p.TimelineItems.removal(env.Config.QueueLabel)
	return ok && env.Now().Sub(at) < settleAfter
}

func (env *Env) waitsForDraft(p gqlPR, drafts []queueDraft) bool {
	_, taken := env.takenUntil(p, drafts)
	return taken
}

func (env *Env) takenUntil(p gqlPR, drafts []queueDraft) (time.Time, bool) {
	at, byGraphite, ok := p.TimelineItems.removal(env.Config.QueueLabel)
	return at.Add(takenFor), ok && byGraphite && awaitsDraft(drafts, p.Number, at, env.Now())
}

func awaitsDraft(drafts []queueDraft, pr int, takenAt, now time.Time) bool {
	return now.Sub(takenAt) < takenFor && !draftRan(drafts, pr, takenAt)
}

func draftRan(drafts []queueDraft, pr int, takenAt time.Time) bool {
	return !newestRun(drafts, pr, takenAt).IsZero()
}

func newestRun(drafts []queueDraft, pr int, takenAt time.Time) time.Time {
	var newest time.Time
	for _, d := range drafts {
		if d.runs(pr, takenAt) && d.ClosedAt.After(newest) {
			newest = d.ClosedAt
		}
	}
	return newest
}

func (env *Env) heldByGraphite(p gqlPR, drafts []queueDraft) bool {
	return draftHolds(drafts, p.Number) || env.waitsForDraft(p, drafts)
}

func (env *Env) unqueueEjected(ctx context.Context, rs []Record, stdout io.Writer) error {
	drafts := sync.OnceValues(func() ([]queueDraft, error) { return env.queueDrafts(ctx) })
	for _, r := range rs {
		for _, q := range r.Queued {
			if err := env.unqueueStack(ctx, r, q, drafts, stdout); err != nil {
				return err
			}
		}
	}
	return nil
}

func (env *Env) unqueueStack(
	ctx context.Context,
	r Record,
	q Queue,
	drafts func() ([]queueDraft, error),
	stdout io.Writer,
) error {
	open, err := drafts()
	if err != nil {
		return err
	}
	prs, err := env.stackPulls(ctx, q.PRs)
	if err != nil {
		return err
	}
	landed, err := env.landedEach(ctx, prs, open)
	if err != nil {
		return err
	}
	out, ok := env.firstEjected(prs, landed, open)
	if !ok {
		return nil
	}
	line, requeued, err := env.ejectStack(ctx, r, q, out, prs, open)
	if err != nil {
		return err
	}
	if requeued {
		_, _ = fmt.Fprintln(stdout, line)
		return nil
	}
	_, _ = fmt.Fprintf(stdout, "unqueued: #%d; #%d left the Graphite merge queue. "+
		"Fix the stack with gt modify and gt submit --stack --draft, then run land-stack %d\n",
		r.Ticket, q.Top, q.Top)
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
	var open []stackPR
	for after := ""; ; {
		var data struct {
			Repository struct {
				Open struct {
					PageInfo gqlPageInfo `json:"pageInfo"`
					Nodes    []stackPR   `json:"nodes"`
				} `json:"open"`
			} `json:"repository"`
		}
		q := repoQuery + "open: pullRequests(" + openPage(after) + "){pageInfo{hasNextPage endCursor} " +
			"nodes{" + stackFields + "}}}}"
		if err := env.graphqlGH(ctx, q, &data); err != nil {
			return nil, err
		}
		open = append(open, data.Repository.Open.Nodes...)
		if after = data.Repository.Open.PageInfo.next(); after == "" {
			break
		}
	}
	return open, env.readStackChecks(ctx, open)
}

const openPageSize = 20

type gqlPageInfo struct {
	HasNextPage bool   `json:"hasNextPage"`
	EndCursor   string `json:"endCursor"`
}

func (p gqlPageInfo) next() string {
	if !p.HasNextPage {
		return ""
	}
	return p.EndCursor
}

func openPage(after string) string {
	if after == "" {
		return fmt.Sprintf("states:OPEN,first:%d", openPageSize)
	}
	return fmt.Sprintf("states:OPEN,first:%d,after:%q", openPageSize, after)
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
