package agents

import (
	"context"
	"net/http"
	"strings"
	"time"
)

const (
	indicatorWorkflow = "staging.yml"
	indicatorEvery    = 60 * time.Minute
	forwardedTo       = " fast-forwarded to "
)

func (g *GitHub) DispatchWorkflow(ctx context.Context, workflow, ref string) error {
	return g.call(
		ctx, http.MethodPost, g.repo("/actions/workflows/%s/dispatches", workflow),
		"", map[string]string{"ref": ref}, nil,
	)
}

func (s *stream) indicatorLines(ctx context.Context, trunkMsg string) []string {
	env := s.env
	if sha, ok := strings.CutPrefix(trunkMsg, env.Config.FeatureBranch+forwardedTo); ok && s.indicatorIdle() {
		s.indicatorDue = sha
	}
	if s.indicatorDue == "" {
		return nil
	}
	if err := env.GitHub.DispatchWorkflow(ctx, indicatorWorkflow, env.Config.FeatureBranch); err != nil {
		return []string{watchErr("dispatch staging indicator: ", err)}
	}
	sha := s.indicatorDue
	s.indicatorDue, s.indicatorAt = "", env.Now()
	return []string{"dispatched staging indicator for " + sha}
}

func (s *stream) indicatorIdle() bool {
	return s.indicatorAt.IsZero() || s.env.Now().Sub(s.indicatorAt) >= indicatorEvery
}
