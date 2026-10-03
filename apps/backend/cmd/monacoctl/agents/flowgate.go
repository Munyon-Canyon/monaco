package agents

import (
	"context"
	"fmt"
	"maps"
	"regexp"
	"slices"
	"strings"

	"github.com/monaco/monaco/apps/backend/internal/tools/flows"
)

var landedRef = regexp.MustCompile(`\(#(\d+)\)\s*$`)

type registry struct {
	flows []flows.Flow
	apps  map[string]flows.AppRow
}

func (r registry) affected(changed []string) []string {
	return flows.Affected(changed, r.flows, r.apps)
}

func (env *Env) flowGate(ctx context.Context, _ Record, stack []stackPR) error {
	top := stack[len(stack)-1]
	reg, mine, err := env.stackFlows(ctx, stack, top)
	if err != nil || len(mine) == 0 {
		return err
	}
	if err := env.stagingMoved(ctx, reg, top, mine); err != nil {
		return err
	}
	return env.sharedInQueue(ctx, stack, mine)
}

func (env *Env) stackFlows(ctx context.Context, prs []stackPR, top stackPR) (registry, []string, error) {
	var changed []string
	for _, p := range prs {
		files, err := env.GitHub.Files(ctx, p.Number)
		if err != nil {
			return registry{}, nil, err
		}
		for _, f := range files {
			if f.Filename == flowsFile && f.Patch != "" {
				changed = append(changed, flows.ChangedRows(f.Patch)...)
				continue
			}
			changed = append(changed, f.Filename)
		}
	}
	if len(changed) == 0 {
		return registry{}, nil, nil
	}
	if _, err := env.git(ctx, "fetch", "--no-tags", "origin", top.Head); err != nil {
		return registry{}, nil, err
	}
	reg, err := env.registryAt(ctx, top.HeadOID)
	if err != nil {
		return registry{}, nil, err
	}
	return reg, reg.affected(changed), nil
}

func (env *Env) registryAt(ctx context.Context, rev string) (registry, error) {
	body, err := env.git(ctx, "show", rev+":"+flowsFile)
	if err != nil {
		return registry{}, err
	}
	parsed, _ := flows.Parse(strings.NewReader(body))
	names, err := env.git(ctx, "ls-tree", "--name-only", rev, flows.AppDir+"/")
	if err != nil {
		return registry{}, err
	}
	apps := map[string]flows.AppRow{}
	for _, name := range strings.Fields(names) {
		body, err := env.git(ctx, "show", rev+":"+name)
		if err != nil {
			return registry{}, err
		}
		if row, ok, _ := flows.ParseApp(name, body); ok {
			apps[row.ID] = row
		}
	}
	return registry{flows: parsed, apps: apps}, nil
}

func (env *Env) git(ctx context.Context, args ...string) (string, error) {
	out, err := env.Run(ctx, env.Work, "", "git", args...)
	return string(out), err
}

func (env *Env) stagingMoved(ctx context.Context, reg registry, top stackPR, mine []string) error {
	trunk := "origin/" + env.Config.FeatureBranch
	if _, err := env.git(ctx, "fetch", "--no-tags", "origin", env.Config.FeatureBranch); err != nil {
		return err
	}
	base, err := env.git(ctx, "merge-base", trunk, top.HeadOID)
	if err != nil {
		return err
	}
	span := strings.TrimSpace(base) + ".." + trunk
	paths, err := env.changedOn(ctx, span)
	if err != nil {
		return err
	}
	var moved []string
	for _, id := range reg.affected(slices.Collect(maps.Keys(paths))) {
		if !slices.Contains(mine, id) {
			continue
		}
		by, err := env.lastChange(ctx, span, flowPaths(reg, paths, id))
		if err != nil {
			return err
		}
		moved = append(moved, fmt.Sprintf("flow %s changed on staging since this stack's base (%s)", id, by))
	}
	if len(moved) == 0 {
		return nil
	}
	return landErr(fmt.Sprintf("not landing #%d: %s. Restack with gt and rerun stage 1, then run land-stack again",
		top.Number, strings.Join(moved, "; ")))
}

func (env *Env) changedOn(ctx context.Context, span string) (map[string]string, error) {
	names, err := env.git(ctx, "diff", "--name-only", span)
	if err != nil {
		return nil, err
	}
	paths := map[string]string{}
	for _, f := range strings.Fields(names) {
		paths[f] = f
	}
	if _, ok := paths[flowsFile]; !ok {
		return paths, nil
	}
	rows, err := env.git(ctx, "diff", "-U0", span, "--", flowsFile)
	if err != nil {
		return nil, err
	}
	delete(paths, flowsFile)
	for _, row := range flows.ChangedRows(rows) {
		paths[row] = flowsFile
	}
	return paths, nil
}

func flowPaths(reg registry, paths map[string]string, id string) []string {
	var out []string
	for changed, path := range paths {
		if slices.Contains(reg.affected([]string{changed}), id) && !slices.Contains(out, path) {
			out = append(out, path)
		}
	}
	slices.Sort(out)
	return out
}

func (env *Env) lastChange(ctx context.Context, span string, paths []string) (string, error) {
	subject, err := env.git(ctx, append([]string{"log", "-1", "--format=%s", span, "--"}, paths...)...)
	if err != nil {
		return "", err
	}
	if m := landedRef.FindStringSubmatch(subject); m != nil {
		return "#" + m[1], nil
	}
	return strings.TrimSpace(subject), nil
}

func (env *Env) sharedInQueue(ctx context.Context, stack []stackPR, mine []string) error {
	queued, err := env.labeledStacks(ctx, numbers(stack))
	if err != nil {
		return err
	}
	var shared, tops []string
	for _, q := range queued {
		_, theirs, err := env.stackFlows(ctx, q.prs, q.top)
		if err != nil {
			return err
		}
		ref := fmt.Sprintf("#%d", q.top.Number)
		for _, id := range mine {
			if slices.Contains(theirs, id) {
				shared = append(shared, fmt.Sprintf("flow %s is in queued stack %s", id, ref))
				tops = append(tops, ref)
			}
		}
	}
	if len(shared) == 0 {
		return nil
	}
	return landErr(fmt.Sprintf("not landing #%d: %s. Wait for %s to land, then run land-stack again",
		stack[len(stack)-1].Number, strings.Join(shared, "; "), strings.Join(slices.Compact(tops), " and ")))
}

type labeledStack struct {
	prs []stackPR
	top stackPR
}

func (env *Env) labeledStacks(ctx context.Context, ours []int) ([]labeledStack, error) {
	open, err := env.openPulls(ctx)
	if err != nil {
		return nil, err
	}
	byNumber := map[int]stackPR{}
	var queued []PR
	for _, p := range open {
		if p.labeled(env.Config.QueueLabel) && !slices.Contains(ours, p.Number) {
			byNumber[p.Number] = p
			queued = append(queued, PR{Number: p.Number, Head: Ref{Ref: p.Head}, Base: Ref{Ref: p.Base}})
		}
	}
	grouped := stacks(queued, env.Config.FeatureBranch)
	out := make([]labeledStack, 0, len(grouped))
	for _, bottom := range slices.Sorted(maps.Keys(grouped)) {
		prs, top := stackPRs(grouped[bottom], byNumber)
		out = append(out, labeledStack{prs: prs, top: top})
	}
	return out, nil
}

func stackPRs(group []PR, byNumber map[int]stackPR) ([]stackPR, stackPR) {
	bases := map[string]bool{}
	for _, p := range group {
		bases[p.Base.Ref] = true
	}
	prs := make([]stackPR, 0, len(group))
	var top stackPR
	for _, p := range group {
		prs = append(prs, byNumber[p.Number])
		if !bases[p.Head.Ref] {
			top = byNumber[p.Number]
		}
	}
	return prs, top
}
