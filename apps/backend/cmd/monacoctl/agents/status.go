package agents

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"slices"
	"strings"
)

const (
	statusMarker = "<!-- monacoctl agents status -->"
	batchMarker  = "<!-- monacoctl agents batch "
	activeMarker = "<!-- monacoctl agents active batch -->"
)

type Comment struct {
	ID                int64  `json:"id"`
	Body              string `json:"body"`
	User              Author `json:"user"`
	AuthorAssociation string `json:"author_association"`
}

type Author struct {
	Login string `json:"login"`
}

const actionsBot = "github-actions[bot]"

func (c Comment) trusted() bool {
	if c.User.Login == actionsBot {
		return true
	}
	switch c.AuthorAssociation {
	case "OWNER", "MEMBER", "COLLABORATOR":
		return true
	}
	return false
}

type CheckRun struct {
	Name       string `json:"name"`
	Status     string `json:"status"`
	Conclusion string `json:"conclusion"`
}

type GHStatus struct {
	State   string `json:"state"`
	Context string `json:"context"`
}

func statusCmd(ctx context.Context, env *Env, args []string, stdout io.Writer) error {
	if len(args) != 1 || args[0] != "--publish" {
		return usageError("status --publish")
	}
	all, err := env.comments(ctx)
	if err != nil {
		return err
	}
	c, ok := newestTrusted(all, statusMarker)
	body, err := env.statusBody(ctx, c.Body)
	if err != nil {
		return err
	}
	if ok && c.Body == body {
		_, _ = fmt.Fprintln(stdout, "status comment unchanged")
		return nil
	}
	if err := env.publishComment(ctx, env.Config.Tracking, all, statusMarker, body); err != nil {
		return err
	}
	_, _ = fmt.Fprintln(stdout, "status comment updated")
	return nil
}

func (env *Env) comments(ctx context.Context) ([]Comment, error) {
	return env.issueComments(ctx, env.Config.Tracking)
}

func (env *Env) issueComments(ctx context.Context, issue int) ([]Comment, error) {
	return pages[Comment](ctx, env.GitHub, env.GitHub.repo("/issues/%d/comments?", issue))
}

func newestTrusted(all []Comment, marker string) (Comment, bool) {
	for _, c := range slices.Backward(all) {
		if c.trusted() && strings.Contains(c.Body, marker) {
			return c, true
		}
	}
	return Comment{}, false
}

func (env *Env) publishComment(ctx context.Context, issue int, all []Comment, marker, body string) error {
	c, ok := newestTrusted(all, marker)
	if ok {
		me, err := env.login(ctx)
		if err != nil {
			return err
		}
		ok = c.User.Login == me
	}
	return env.writeComment(ctx, issue, c.ID, ok, body)
}

func (env *Env) login(ctx context.Context) (string, error) {
	if env.Actions {
		return actionsBot, nil
	}
	me, err := env.Run(ctx, env.Work, "", "gh", "api", "user", "--jq", ".login")
	return strings.TrimSpace(string(me)), err
}

func (env *Env) writeComment(ctx context.Context, issue int, id int64, found bool, body string) error {
	payload := map[string]string{"body": body}
	if !found {
		return env.GitHub.call(
			ctx,
			"POST",
			env.GitHub.repo("/issues/%d/comments", issue),
			"",
			payload,
			nil,
		)
	}
	path := fmt.Sprintf("/repos/%s/issues/comments/%d", env.GitHub.Repo, id)
	return env.GitHub.call(ctx, "PATCH", path, "", payload, nil)
}

func (env *Env) statusBody(ctx context.Context, published string) (string, error) {
	open, err := env.GitHub.PRs(ctx, "state=open")
	if err != nil {
		return "", err
	}
	var rows []PR
	for _, stack := range stacks(open, env.Config.FeatureBranch) {
		rows = append(rows, stack...)
	}
	slices.SortFunc(rows, func(a, b PR) int { return a.Number - b.Number })
	var b strings.Builder
	_, _ = fmt.Fprintf(&b, "%s\n| pr | sha | ci | ci-ok | verify |\n", statusMarker)
	limit := maxLines - 3
	for i, pr := range rows {
		if i == limit {
			_, _ = fmt.Fprintf(&b, "| and %d more |\n", len(rows)-i)
			break
		}
		ci, ciok, verify, err := env.checks(ctx, pr.Head.SHA)
		if err != nil {
			return "", err
		}
		_, _ = fmt.Fprintf(&b, "| #%d | %s | %s | %s | %s |\n", pr.Number, shortSHA(pr.Head.SHA), ci, ciok, verify)
	}
	batch, err := env.batchBoard(ctx, published)
	if err != nil {
		return "", err
	}
	if err := env.remoteOwners(&b); err != nil {
		return "", err
	}
	return b.String() + batch, nil
}

func (env *Env) batchBoard(ctx context.Context, published string) (string, error) {
	b, err := env.currentBatch(published)
	if err != nil || len(b.Tickets) == 0 {
		return "", err
	}
	views, err := env.views(ctx, b)
	if err != nil {
		return "", err
	}
	raw, _ := json.Marshal(b)
	out := "\n" + boardRows(views, env.Now()) + batchMarker + string(raw) + " -->\n"
	if slices.ContainsFunc(views, func(v ticketView) bool { return v.state() != "merged" }) {
		out += activeMarker + "\n"
	}
	return out, nil
}

func (env *Env) currentBatch(published string) (Batch, error) {
	prev := publishedBatch(published)
	b, ok, err := loadBatch(env.batchPath())
	if err != nil || !ok {
		return prev, err
	}
	return env.withDispatch(b, prev)
}

func publishedBatch(body string) Batch {
	_, rest, ok := strings.Cut(body, batchMarker)
	raw, _, _ := strings.Cut(rest, " -->")
	var b Batch
	if !ok || json.Unmarshal([]byte(raw), &b) != nil {
		return Batch{}
	}
	return b
}

func (env *Env) checks(ctx context.Context, sha string) (string, string, string, error) {
	var resp struct {
		CheckRuns []CheckRun `json:"check_runs"`
	}
	if err := env.GitHub.call(
		ctx,
		"GET",
		env.GitHub.repo("/commits/%s/check-runs?per_page=%d", sha, pageSize),
		"",
		nil,
		&resp,
	); err != nil {
		return "", "", "", err
	}
	ci, ciok := "none", "none"
	for _, run := range resp.CheckRuns {
		got := run.Conclusion
		if got == "" {
			got = run.Status
		}
		switch run.Name {
		case "ci":
			ci = got
		case stage1Check:
			ciok = got
		}
	}
	verify, err := env.verifyState(ctx, sha)
	return ci, ciok, verify, err
}

func (env *Env) verifyState(ctx context.Context, sha string) (string, error) {
	all, err := pages[GHStatus](ctx, env.GitHub, env.GitHub.repo("/commits/%s/statuses?", sha))
	if err != nil {
		return "", err
	}
	for _, s := range all {
		if s.Context == "verify" {
			return s.State, nil
		}
	}
	return "none", nil
}

func (env *Env) remoteOwners(b *strings.Builder) error {
	rs, err := env.records()
	if err != nil {
		return err
	}
	header := false
	for _, r := range rs {
		if r.State == Exited || worktreeHere(r) {
			continue
		}
		if !header {
			b.WriteString("\nRemote owners (worktree on another machine, not counted against the lane cap)\n")
			header = true
		}
		_, _ = fmt.Fprintf(b, "- #%d %s %s\n", r.Ticket, r.Model, r.State)
	}
	return nil
}
