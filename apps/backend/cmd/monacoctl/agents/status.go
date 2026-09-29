package agents

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"slices"
	"strings"

	"github.com/monaco/monaco/apps/backend/internal/errs"
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
	publish := len(args) == 1 && args[0] == "--publish"
	if len(args) != 0 && !publish {
		return usageError("status [--publish]")
	}
	open, err := env.GitHub.PRs(ctx, "state=open")
	if err != nil {
		return err
	}
	trunks, err := env.trunks(ctx)
	if err != nil {
		return err
	}
	if !publish {
		body, err := env.statusBody(ctx, open, trunks, "")
		_, _ = io.WriteString(stdout, body)
		return err
	}
	boards, err := env.statusBoards(ctx, open, trunks, stdout)
	if err != nil {
		return err
	}
	for _, board := range boards {
		if err := env.publishStatus(ctx, open, board, stdout); err != nil {
			return err
		}
	}
	return nil
}

type statusBoard struct {
	issue  int
	trunks []string
}

func (env *Env) statusBoards(ctx context.Context, open []PR, trunks []string, notice io.Writer) ([]statusBoard, error) {
	var boards []statusBoard
	for _, trunk := range trunks {
		issue, err := env.trackingIssue(ctx, trunk, stackTickets(open, trunk)...)
		if errs.CodeOf(err) == errs.CodeNotFound {
			_, _ = fmt.Fprintf(notice, "skipped %s: %s\n", trunk, cliText(err))
			continue
		}
		if err != nil {
			return nil, err
		}
		if i := slices.IndexFunc(boards, func(b statusBoard) bool { return b.issue == issue }); i >= 0 {
			boards[i].trunks = append(boards[i].trunks, trunk)
			continue
		}
		boards = append(boards, statusBoard{issue, []string{trunk}})
	}
	return boards, nil
}

func stackTickets(open []PR, trunk string) []int {
	var tickets []int
	for _, stack := range stacks(open, trunk) {
		for _, pr := range stack {
			if n, ok := pr.Ticket(); ok && !slices.Contains(tickets, n) {
				tickets = append(tickets, n)
			}
		}
	}
	return tickets
}

func (env *Env) publishStatus(ctx context.Context, open []PR, board statusBoard, stdout io.Writer) error {
	all, err := env.issueComments(ctx, board.issue)
	if err != nil {
		return err
	}
	c, ok := newestTrusted(all, statusMarker)
	body, err := env.statusBody(ctx, open, board.trunks, c.Body)
	if err != nil {
		return err
	}
	if ok && c.Body == body {
		_, _ = fmt.Fprintf(stdout, "status comment unchanged on #%d\n", board.issue)
		return nil
	}
	if err := env.publishComment(ctx, board.issue, all, statusMarker, body); err != nil {
		return err
	}
	_, _ = fmt.Fprintf(stdout, "status comment updated on #%d\n", board.issue)
	return nil
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

func (env *Env) statusBody(ctx context.Context, open []PR, trunks []string, published string) (string, error) {
	type row struct {
		pr    PR
		trunk string
	}
	rows := make([]row, 0, len(open))
	for _, trunk := range trunks {
		var group []PR
		for _, stack := range stacks(open, trunk) {
			group = append(group, stack...)
		}
		slices.SortFunc(group, func(a, b PR) int { return a.Number - b.Number })
		for _, pr := range group {
			rows = append(rows, row{pr, trunk})
		}
	}
	var b strings.Builder
	_, _ = fmt.Fprintf(&b, "%s\n| pr | feature branch | sha | ci | ci-ok | verify |\n", statusMarker)
	limit := maxLines - 3
	for i, r := range rows {
		if i == limit {
			_, _ = fmt.Fprintf(&b, "| and %d more |\n", len(rows)-i)
			break
		}
		ci, ciok, verify, err := env.checks(ctx, r.pr.Head.SHA)
		if err != nil {
			return "", err
		}
		_, _ = fmt.Fprintf(&b, "| #%d | %s | %s | %s | %s | %s |\n",
			r.pr.Number, r.trunk, shortSHA(r.pr.Head.SHA), ci, ciok, verify)
	}
	batch, err := env.batchBoard(ctx, published, trunks)
	return b.String() + batch, err
}

func (env *Env) batchBoard(ctx context.Context, published string, trunks []string) (string, error) {
	b, err := env.currentBatch(published)
	if err != nil || len(b.Tickets) == 0 || (b.Branch != "" && !slices.Contains(trunks, b.Branch)) {
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
