package agents

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/monaco/monaco/apps/backend/internal/errs"
)

const (
	landsPrefix = "Lands stack:"
	stackFields = `number state baseRefName headRefName body mergeCommit{oid} autoMergeRequest{enabledAt}
mergeQueueEntry{position} commits(last:1){nodes{commit{` + commitChecks + `}}}`
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
	rec, err := env.record(ticket)
	if err != nil {
		return err
	}
	if rec.Queued != nil {
		if rec.Queued.Top != n {
			return landErr(fmt.Sprintf("#%d already has #%d queued", ticket, rec.Queued.Top))
		}
		return env.settle(ctx, rec, stdout)
	}
	open, err := env.openPulls(ctx)
	if err != nil {
		return err
	}
	stack, err := walkStack(open, n, env.Config.FeatureBranch)
	if err != nil {
		return err
	}
	if waiting := waitingOn(stack); len(waiting) > 0 {
		_, _ = fmt.Fprintf(stdout, "not landing #%d; waiting on %s\n", n, strings.Join(waiting, ", "))
		return nil
	}
	return env.land(ctx, rec, stack, stdout)
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
	fb := env.Config.FeatureBranch
	for _, p := range stack[1:] {
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
	if err := env.saveRecord(rec); err != nil {
		return err
	}
	_, _ = fmt.Fprintf(stdout, "queued #%d. %s\n", top.Number, line)
	return nil
}

func (env *Env) settle(ctx context.Context, rec Record, stdout io.Writer) error {
	prs, err := env.stackPulls(ctx, rec.Queued.PRs)
	if err != nil {
		return err
	}
	top := prs[len(prs)-1]
	switch {
	case top.State == "MERGED":
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
	case top.State == "OPEN" && top.MergeQueueEntry != nil:
		_, _ = fmt.Fprintf(stdout, "#%d is queued at position %d\n", top.Number, top.MergeQueueEntry.Position)
		return nil
	case top.State == "OPEN" && top.AutoMerge != nil:
		_, _ = fmt.Fprintf(stdout, "#%d waits for its checks, then enters the queue\n", top.Number)
		return nil
	default:
		_, _ = fmt.Fprintf(stdout, "#%d left the queue; the stack is unmarked. "+
			"Fix it with gt modify and gt submit --stack, then run land-stack again\n", top.Number)
	}
	rec.Queued = nil
	rec.Changed = env.Now()
	return env.saveRecord(rec)
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
	return fmt.Errorf("%w; the stack is not marked queued: run gt submit --stack in its worktree "+
		"to restore the bases, then land-stack again", err)
}
