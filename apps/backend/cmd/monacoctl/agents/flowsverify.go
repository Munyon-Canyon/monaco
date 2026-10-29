package agents

import (
	"context"
	"fmt"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"
)

const (
	flowsVerifyContext   = "flows-verify"
	flowsVerifyWorkflow  = "flows-verify.yml"
	flowsVerifyFor       = 30 * time.Minute
	statusDescriptionMax = 140
	restackLine          = "Restack with gt and rerun stage 1, then run land-stack again"
)

type flowsRun struct {
	pr      int
	head    string
	staging string
	flows   []string
}

func (r flowsRun) key() string {
	return "flows " + strings.Join(r.flows, " ") + " on staging " + r.staging
}

func (r flowsRun) keyPrefix() string {
	return "flows " + strings.Join(r.flows, " ") + " on staging "
}

func (env *Env) latestEquivalent(ctx context.Context, statuses []GHStatus, r flowsRun, paths []string) GHStatus {
	tree := ""
	for _, s := range statuses {
		staging, ok := strings.CutPrefix(s.Description, r.keyPrefix())
		if s.Context != flowsVerifyContext || !ok {
			continue
		}
		if staging == r.staging {
			return s
		}
		if tree == "" {
			tree = env.flowsTree(ctx, r.staging, paths)
		}
		if tree != "" && env.flowsTree(ctx, staging, paths) == tree {
			return s
		}
	}
	return GHStatus{}
}

func (env *Env) flowsTree(ctx context.Context, sha string, paths []string) string {
	out, err := env.git(ctx, append([]string{"ls-tree", sha, "--"}, paths...)...)
	if err != nil {
		return ""
	}
	return out
}

type flowsWait struct {
	waiting string
	started bool
}

func (w flowsWait) entries() []string {
	if w.waiting == "" {
		return nil
	}
	return []string{w.waiting}
}

func (env *Env) verifyMoved(ctx context.Context, top stackPR, moved []movedFlow) (flowsWait, error) {
	if len(moved) == 0 {
		return flowsWait{}, nil
	}
	tip, err := env.git(ctx, "rev-parse", "origin/"+env.Config.FeatureBranch)
	if err != nil {
		return flowsWait{}, err
	}
	run := flowsRun{pr: top.Number, head: top.HeadOID, staging: strings.TrimSpace(tip)}
	why := make([]string, len(moved))
	var paths []string
	for i, m := range moved {
		run.flows = append(run.flows, m.id)
		why[i] = m.String()
		paths = append(paths, m.paths...)
	}
	paths = slices.Sorted(slices.Values(slices.Compact(slices.Sorted(slices.Values(paths)))))
	refusal := fmt.Sprintf("not landing #%d: %s", top.Number, strings.Join(why, "; "))
	if len(run.key()) > statusDescriptionMax {
		return flowsWait{}, landErr(refusal + ". " + restackLine)
	}
	statuses, err := pages[GHStatus](ctx, env.GitHub, env.GitHub.repo("/commits/%s/statuses?", run.head))
	if err != nil {
		return flowsWait{}, err
	}
	waiting := fmt.Sprintf("flows-verify of flows %s on staging %s: %s",
		strings.Join(run.flows, " "), shortSHA(run.staging), strings.Join(why, "; "))
	switch last := env.latestEquivalent(ctx, statuses, run, paths); {
	case last.State == "success":
		return flowsWait{}, nil
	case last.State == "failure" || last.State == "error":
		return flowsWait{}, landErr(fmt.Sprintf("%s, and flows-verify failed on staging %s (%s). %s",
			refusal, shortSHA(run.staging), last.TargetURL, restackLine))
	case last.State == "pending" && env.Now().Sub(last.CreatedAt) < flowsVerifyFor:
		return flowsWait{waiting: waiting}, nil
	}
	if err := env.startFlowsVerify(ctx, run); err != nil {
		return flowsWait{}, err
	}
	return flowsWait{waiting: waiting, started: true}, nil
}

func (env *Env) startFlowsVerify(ctx context.Context, r flowsRun) error {
	dispatch := map[string]any{"ref": env.Config.FeatureBranch, "inputs": map[string]string{
		"pr": strconv.Itoa(r.pr), "head_sha": r.head, "base_sha": r.staging, "flows": strings.Join(r.flows, " "),
	}}
	path := env.GitHub.repo("/actions/workflows/%s/dispatches", flowsVerifyWorkflow)
	if err := env.GitHub.call(ctx, http.MethodPost, path, "", dispatch, nil); err != nil {
		return err
	}
	status := map[string]string{"state": "pending", "context": flowsVerifyContext, "description": r.key()}
	return env.GitHub.call(ctx, http.MethodPost, env.GitHub.repo("/statuses/%s", r.head), "", status, nil)
}
