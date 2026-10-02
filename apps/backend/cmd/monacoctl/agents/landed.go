package agents

import (
	"context"
	"fmt"
	"net/http"
	"strings"
)

func (env *Env) landed(ctx context.Context, state, head string) (bool, error) {
	switch strings.ToUpper(state) {
	case "MERGED":
		return true, nil
	case "CLOSED":
		return env.GitHub.inBranch(ctx, env.Config.FeatureBranch, head)
	default:
		return false, nil
	}
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
