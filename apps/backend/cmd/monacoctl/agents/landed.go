package agents

import (
	"context"
	"fmt"
	"net/http"
	"slices"
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
	subjects       []string
}

func (env *Env) landed(ctx context.Context, pr closedPR) (bool, error) {
	switch strings.ToUpper(pr.State) {
	case "MERGED":
		return true, nil
	case "CLOSED":
		squashed, err := env.squashed(ctx, pr)
		if err != nil || squashed {
			return squashed, err
		}
		return env.GitHub.inBranch(ctx, env.Config.FeatureBranch, pr.Head)
	default:
		return false, nil
	}
}

func (env *Env) squashed(ctx context.Context, pr closedPR) (bool, error) {
	if pr.ClosedAt.IsZero() {
		return false, nil
	}
	since := pr.ClosedAt.Add(-time.Hour)
	if c := env.trunk; c == nil || since.Before(c.since) || pr.ClosedAt.After(c.fetched) {
		subjects, err := env.GitHub.subjects(ctx, env.Config.FeatureBranch, since)
		if err != nil {
			return false, err
		}
		env.trunk = &trunkLog{since: since, fetched: env.Now(), subjects: subjects}
	}
	suffix := fmt.Sprintf(" (#%d)", pr.Number)
	return slices.ContainsFunc(env.trunk.subjects, func(s string) bool { return strings.HasSuffix(s, suffix) }), nil
}

func (g *GitHub) subjects(ctx context.Context, branch string, since time.Time) ([]string, error) {
	type commit struct {
		Commit struct {
			Message string `json:"message"`
		} `json:"commit"`
	}
	commits, err := pages[commit](ctx, g, g.repo("/commits?sha=%s&since=%s", branch, since.UTC().Format(time.RFC3339)))
	if err != nil {
		return nil, fmt.Errorf("list %s commits: %w", branch, err)
	}
	out := make([]string, len(commits))
	for i, c := range commits {
		out[i], _, _ = strings.Cut(c.Commit.Message, "\n")
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

func (pr PR) landedSHA() string {
	if pr.MergedAt != nil {
		return pr.MergeCommitSHA
	}
	return pr.Head.SHA
}
