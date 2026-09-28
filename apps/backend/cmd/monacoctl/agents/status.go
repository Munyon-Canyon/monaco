package agents

import (
	"context"
	"fmt"
	"io"
	"slices"
	"strings"
)

const statusMarker = "<!-- monacoctl agents status -->"

type Comment struct {
	ID   int64  `json:"id"`
	Body string `json:"body"`
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
	body, err := env.statusBody(ctx)
	if err != nil {
		return err
	}
	c, ok, err := env.statusComment(ctx)
	if err != nil {
		return err
	}
	if ok && c.Body == body {
		_, _ = fmt.Fprintln(stdout, "status comment unchanged")
		return nil
	}
	if err := env.writeStatus(ctx, c.ID, ok, body); err != nil {
		return err
	}
	_, _ = fmt.Fprintln(stdout, "status comment updated")
	return nil
}

func (env *Env) statusComment(ctx context.Context) (Comment, bool, error) {
	all, err := pages[Comment](ctx, env.GitHub, env.GitHub.repo("/issues/%d/comments?", env.Config.Tracking))
	if err != nil {
		return Comment{}, false, err
	}
	for _, c := range all {
		if strings.Contains(c.Body, statusMarker) {
			return c, true, nil
		}
	}
	return Comment{}, false, nil
}

func (env *Env) writeStatus(ctx context.Context, id int64, found bool, body string) error {
	payload := map[string]string{"body": body}
	if !found {
		return env.GitHub.call(
			ctx,
			"POST",
			env.GitHub.repo("/issues/%d/comments", env.Config.Tracking),
			"",
			payload,
			nil,
		)
	}
	path := fmt.Sprintf("/repos/%s/issues/comments/%d", env.GitHub.Repo, id)
	return env.GitHub.call(ctx, "PATCH", path, "", payload, nil)
}

func (env *Env) statusBody(ctx context.Context) (string, error) {
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
	return b.String(), nil
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
		case "ci-ok":
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
