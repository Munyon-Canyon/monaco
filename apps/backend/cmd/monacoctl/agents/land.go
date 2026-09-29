package agents

import (
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

	"github.com/monaco/monaco/apps/backend/internal/errs"
)

const (
	landsPrefix = "Lands stack:"
	stackFields = `number state baseRefName headRefName body mergeCommit{oid} autoMergeRequest{enabledAt}
isInMergeQueue mergeQueueEntry{position} commits(last:1){nodes{commit{` + commitChecks + `}}}`
	repoQuery = "query($owner:String!,$name:String!){repository(owner:$owner,name:$name){"
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
	AutoMerge *struct{} `json:"autoMergeRequest"`
	InQueue   bool      `json:"isInMergeQueue"`
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
		if !tops[0].ejected() {
			return env.settle(ctx, rec, tops[0], stdout)
		}
		_, _ = fmt.Fprintf(stdout, "#%d left the queue; relanding its stack\n", n)
		if err := env.unmark(ctx, rec); err != nil {
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

func walkStack(open []stackPR, top int, trunks []string) ([]stackPR, error) {
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
	for !slices.Contains(trunks, cur.Base) {
		parent, ok := byHead[cur.Base]
		if !ok || len(stack) > len(open) {
			return nil, landErr(fmt.Sprintf("#%d's base %s is neither %s nor an open PR",
				cur.Number, cur.Base, strings.Join(trunks, " nor ")))
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
	trunks, err := env.trunks(ctx)
	if err != nil {
		return nil, err
	}
	walked, err := walkStack(open, top, trunks)
	if err != nil {
		return nil, err
	}
	return env.wholeStack(ctx, worktree, open, walked, stdout)
}

func (env *Env) wholeStack(
	ctx context.Context,
	worktree string,
	open, walked []stackPR,
	stdout io.Writer,
) ([]stackPR, error) {
	top := walked[len(walked)-1]
	nums, ok, err := landsNums(top.Body)
	switch {
	case err != nil:
		return nil, err
	case !ok:
		return env.graphiteStack(ctx, worktree, open, walked, stdout), nil
	case len(nums) <= len(walked):
		return walked, nil
	case nums[len(nums)-1] != top.Number:
		return nil, landErr(fmt.Sprintf("#%d's %q line does not end with #%d", top.Number, landsPrefix, top.Number))
	}
	prs, err := env.stackPulls(ctx, nums)
	if err != nil {
		return nil, err
	}
	for _, p := range prs {
		if p.State != "OPEN" {
			return nil, landErr(fmt.Sprintf("#%d from #%d's %q line is %s; only open PRs reland",
				p.Number, top.Number, landsPrefix, strings.ToLower(p.State)))
		}
	}
	return prs, nil
}

func landsNums(body string) ([]int, bool, error) {
	first, _, _ := strings.Cut(body, "\n")
	rest, ok := strings.CutPrefix(first, landsPrefix)
	if !ok {
		return nil, false, nil
	}
	var nums []int
	for _, ref := range strings.Fields(rest) {
		n, err := strconv.Atoi(strings.TrimPrefix(ref, "#"))
		if err != nil {
			return nil, true, landErr(fmt.Sprintf("%q is not a PR in %q", ref, first))
		}
		nums = append(nums, n)
	}
	return nums, true, nil
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
	trunk := walked[0].Base
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
		t := p.flat()
		var why []string
		if t.Stage1 != "success" {
			why = append(why, "stage 1 "+orMissing(t.Stage1))
		}
		if t.Verify != "success" {
			why = append(why, "verify "+orMissing(t.Verify))
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
	fb := stack[0].Base
	for _, p := range stack[1:] {
		if p.Base == fb {
			continue
		}
		if err := env.gh(ctx, "", "pr", "edit", strconv.Itoa(p.Number), "--base", fb); err != nil {
			return landFailed(err)
		}
	}
	top := stack[len(stack)-1]
	line := landsLine(nums)
	if err := env.gh(
		ctx,
		landsBody(line, top.Body),
		"pr",
		"edit",
		strconv.Itoa(top.Number),
		"--body-file",
		"-",
	); err != nil {
		return landFailed(err)
	}
	if err := env.gh(ctx, "", "pr", "merge", strconv.Itoa(top.Number), "--auto"); err != nil {
		return landFailed(err)
	}
	rec.Queued = &Queue{Top: top.Number, PRs: nums}
	rec.Changed = env.Now()
	if err := env.storeRecord(ctx, rec); err != nil {
		return err
	}
	_, _ = fmt.Fprintf(stdout, "queued #%d. %s\n", top.Number, line)
	return nil
}

func (env *Env) settle(ctx context.Context, rec Record, top stackPR, stdout io.Writer) error {
	switch {
	case top.InQueue:
		_, _ = fmt.Fprintf(stdout, "#%d is queued at position %d\n", top.Number, top.position())
		return nil
	case top.State != "MERGED":
		_, _ = fmt.Fprintf(stdout, "#%d waits for its checks, then enters the queue\n", top.Number)
		return nil
	}
	prs, err := env.stackPulls(ctx, rec.Queued.PRs)
	if err != nil {
		return err
	}
	sha := shortSHA(top.MergeCommit.OID)
	for _, p := range prs[:len(prs)-1] {
		if p.State != "OPEN" {
			continue
		}
		note := fmt.Sprintf("Landed in #%d (%s)", top.Number, sha)
		if err := env.gh(ctx, "", "pr", "close", strconv.Itoa(p.Number), "--comment", note); err != nil {
			return err
		}
		_, _ = fmt.Fprintf(stdout, "closed #%d: %s\n", p.Number, note)
	}
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
		return err
	}
	_, _ = fmt.Fprintf(stdout, "#%d merged as %s; gt sync ran in %s\n", top.Number, sha, rec.Worktree)
	return env.unmark(ctx, rec)
}

func (p stackPR) position() int {
	if p.MergeQueueEntry == nil {
		return 0
	}
	return p.MergeQueueEntry.Position
}

func (p stackPR) ejected() bool {
	return p.State != "MERGED" && !p.InQueue && p.AutoMerge == nil
}

func (env *Env) unmark(ctx context.Context, rec Record) error {
	rec.Queued = nil
	rec.Changed = env.Now()
	return env.storeRecord(ctx, rec)
}

func (env *Env) unqueueEjected(ctx context.Context, rs []Record, stdout io.Writer) error {
	for _, r := range rs {
		if r.Queued == nil {
			continue
		}
		tops, err := env.stackPulls(ctx, []int{r.Queued.Top})
		if err != nil {
			return err
		}
		if !tops[0].ejected() {
			continue
		}
		_, _ = fmt.Fprintf(stdout, "unqueued: #%d; #%d left the queue. Fix the stack with gt modify and "+
			"gt submit --stack --draft, then run land-stack %d\n", r.Ticket, r.Queued.Top, r.Queued.Top)
		if err := env.unmark(ctx, r); err != nil {
			return err
		}
	}
	return nil
}

func landsLine(nums []int) string {
	refs := make([]string, len(nums))
	for i, n := range nums {
		refs[i] = "#" + strconv.Itoa(n)
	}
	return landsPrefix + " " + strings.Join(refs, " ")
}

func landsBody(line, body string) string {
	if first, rest, _ := strings.Cut(body, "\n"); strings.HasPrefix(first, landsPrefix) {
		body = rest
	}
	return line + "\n\n" + strings.TrimLeft(body, "\n")
}

func (env *Env) gh(ctx context.Context, stdin string, args ...string) error {
	_, err := env.Run(ctx, env.Work, stdin, "gh", append(args, "-R", env.Config.Repo)...)
	return err
}

func (env *Env) graphqlGH(ctx context.Context, query string, out any) error {
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
	return readAllChecks(ctx, env.graphqlGH, commits)
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
	return fmt.Errorf("%w; the stack is not marked queued: run gt submit --stack --draft in its worktree "+
		"to restore the bases, then land-stack again", err)
}
