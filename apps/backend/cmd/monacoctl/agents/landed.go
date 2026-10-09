package agents

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"
)

type closedPR struct {
	Number   int
	State    string
	Head     string
	ClosedAt time.Time
}

type trunkLog struct {
	since, fetched time.Time
	commits        []trunkCommit
}

type trunkCommit struct{ sha, subject string }

func (env *Env) landed(ctx context.Context, pr closedPR) (bool, error) {
	switch strings.ToUpper(pr.State) {
	case "MERGED":
		return true, nil
	case "CLOSED":
		_, squashed, err := env.squashed(ctx, pr)
		if err != nil || squashed {
			return squashed, err
		}
		return env.GitHub.inBranch(ctx, env.Config.FeatureBranch, pr.Head)
	default:
		return false, nil
	}
}

func (env *Env) landedCommit(ctx context.Context, pr PR) (string, bool, error) {
	closed := pr.closed()
	switch closed.State {
	case "MERGED":
		return pr.MergeCommitSHA, true, nil
	case "CLOSED":
		sha, ok, err := env.squashed(ctx, closed)
		if err != nil || ok {
			return sha, ok, err
		}
		ok, err = env.GitHub.inBranch(ctx, env.Config.FeatureBranch, closed.Head)
		return closed.Head, ok, err
	default:
		return "", false, nil
	}
}

func (env *Env) squashed(ctx context.Context, pr closedPR) (string, bool, error) {
	if pr.ClosedAt.IsZero() {
		return "", false, nil
	}
	since := pr.ClosedAt.Add(-time.Hour)
	if c := env.trunk; c == nil || since.Before(c.since) || pr.ClosedAt.After(c.fetched) {
		commits, err := env.GitHub.trunkCommits(ctx, env.Config.FeatureBranch, since)
		if err != nil {
			return "", false, err
		}
		env.trunk = &trunkLog{since: since, fetched: env.Now(), commits: commits}
	}
	suffix := fmt.Sprintf(" (#%d)", pr.Number)
	for _, c := range env.trunk.commits {
		if strings.HasSuffix(c.subject, suffix) {
			return c.sha, true, nil
		}
	}
	return "", false, nil
}

func (g *GitHub) trunkCommits(ctx context.Context, branch string, since time.Time) ([]trunkCommit, error) {
	type commit struct {
		SHA    string `json:"sha"`
		Commit struct {
			Message string `json:"message"`
		} `json:"commit"`
	}
	commits, err := pages[commit](ctx, g, g.repo("/commits?sha=%s&since=%s", branch, since.UTC().Format(time.RFC3339)))
	if err != nil {
		return nil, fmt.Errorf("list %s commits: %w", branch, err)
	}
	out := make([]trunkCommit, len(commits))
	for i, c := range commits {
		subject, _, _ := strings.Cut(c.Commit.Message, "\n")
		out[i] = trunkCommit{sha: c.SHA, subject: subject}
	}
	return out, nil
}

func (g *GitHub) inBranch(ctx context.Context, branch, sha string) (bool, error) {
	var cmp struct {
		Status string `json:"status"`
	}
	if err := g.call(ctx, http.MethodGet, g.repo("/compare/%s...%s", branch, sha), "", nil, &cmp); err != nil {
		return false, fmt.Errorf("compare %s with %s: %w", sha, branch, err)
	}
	return cmp.Status == "behind" || cmp.Status == "identical", nil
}

func (pr PR) closed() closedPR {
	c := closedPR{Number: pr.Number, State: pr.graphState(), Head: pr.Head.SHA}
	if pr.ClosedAt != nil {
		c.ClosedAt = *pr.ClosedAt
	}
	return c
}

func (p gqlPR) closed() closedPR {
	return closedPR{Number: p.Number, State: p.State, Head: p.HeadOID, ClosedAt: p.ClosedAt}
}

func (pr PR) graphState() string {
	if pr.MergedAt != nil {
		return "MERGED"
	}
	return strings.ToUpper(pr.State)
}
