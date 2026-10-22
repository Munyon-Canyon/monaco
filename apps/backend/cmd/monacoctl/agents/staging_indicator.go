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

func (g *GitHub) DispatchWorkflow(ctx context.Context, workflow, ref string, inputs map[string]string) error {
	body := map[string]any{"ref": ref}
	if len(inputs) > 0 {
		body["inputs"] = inputs
	}
	return g.call(ctx, http.MethodPost, g.repo("/actions/workflows/%s/dispatches", workflow), "", body, nil)
}

func (s *stream) indicatorLines(ctx context.Context, trunkMsg string) []string {
	env := s.env
	if sha, ok := strings.CutPrefix(trunkMsg, env.Config.FeatureBranch+forwardedTo); ok && s.indicatorIdle() {
		s.indicatorDue = sha
	}
	if s.indicatorDue == "" {
		return nil
	}
	if err := env.GitHub.DispatchWorkflow(ctx, indicatorWorkflow, env.Config.FeatureBranch, nil); err != nil {
		return []string{watchErr("dispatch staging indicator: ", err)}
	}
	sha := s.indicatorDue
	s.indicatorDue, s.indicatorAt = "", env.Now()
	return []string{"dispatched staging indicator for " + sha}
}

func (s *stream) indicatorIdle() bool {
	now := s.env.Now()
	if !s.indicatorAt.IsZero() && now.Sub(s.indicatorAt) < indicatorEvery {
		return false
	}
	rs, err := s.env.records()
	if err != nil {
		return true
	}
	for _, r := range rs {
		for _, l := range r.Landed {
			if now.Sub(l.At) < indicatorEvery {
				return false
			}
		}
	}
	return true
}
