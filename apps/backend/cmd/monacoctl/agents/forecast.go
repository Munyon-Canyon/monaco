package agents

import (
	"context"
	"fmt"
	"io"
	"maps"
	"slices"
	"strings"
)

const maxLines = 20

type risk struct {
	path    string
	bottoms []int
}

func forecastCmd(ctx context.Context, env *Env, args []string, stdout io.Writer) error {
	if len(args) != 0 {
		return usageError("forecast")
	}
	return printForecast(ctx, env, stdout)
}

func printForecast(ctx context.Context, env *Env, stdout io.Writer) error {
	risks, flowRisks, flowErr, err := forecast(ctx, env)
	if err != nil {
		return err
	}
	trunk := env.Config.FeatureBranch
	if len(risks) == 0 {
		_, _ = fmt.Fprintf(stdout, "no file is touched by more than one open stack into %s\n", trunk)
	} else {
		_, _ = fmt.Fprintf(stdout, "%d files touched by more than one open stack into %s:\n", len(risks), trunk)
	}
	for i, r := range risks {
		if i == maxLines-2 {
			_, _ = fmt.Fprintf(stdout, "  and %d more\n", len(risks)-i)
			break
		}
		_, _ = fmt.Fprintf(stdout, "  %s  %s\n", r.path, prList(r.bottoms))
	}
	if flowErr != nil {
		_, _ = fmt.Fprintf(stdout, "flows: not checked: %s\n", strings.TrimSpace(flowErr.Error()))
	}
	for _, r := range flowRisks {
		_, _ = fmt.Fprintf(stdout, "flows: %s %s\n", r.path, prList(r.bottoms))
	}
	return nil
}

func forecast(ctx context.Context, env *Env) (files, flowRisks []risk, flowErr, err error) {
	open, err := env.GitHub.PRs(ctx, "state=open")
	if err != nil {
		return nil, nil, nil, err
	}
	touched := map[string]map[int]bool{}
	changed := map[int][]string{}
	grouped := stacks(open, env.Config.FeatureBranch)
	for bottom, stack := range grouped {
		for _, pr := range stack {
			if err := touch(ctx, env, pr, bottom, touched, changed); err != nil {
				return nil, nil, nil, err
			}
		}
	}
	if len(grouped) < 2 {
		return overlaps(touched), nil, nil, nil
	}
	reg, err := env.registryAt(ctx, "origin/"+env.Config.FeatureBranch)
	if err != nil {
		return overlaps(touched), nil, err, nil
	}
	flowed := map[string]map[int]bool{}
	for bottom, files := range changed {
		for _, id := range reg.affected(files) {
			if flowed[id] == nil {
				flowed[id] = map[int]bool{}
			}
			flowed[id][bottom] = true
		}
	}
	return overlaps(touched), overlaps(flowed), nil, nil
}

func overlaps(touched map[string]map[int]bool) []risk {
	var risks []risk
	for _, path := range slices.Sorted(maps.Keys(touched)) {
		if len(touched[path]) > 1 {
			risks = append(risks, risk{path, slices.Sorted(maps.Keys(touched[path]))})
		}
	}
	return risks
}

func touch(
	ctx context.Context, env *Env, pr PR, bottom int, touched map[string]map[int]bool, changed map[int][]string,
) error {
	files, err := env.GitHub.Files(ctx, pr.Number)
	if err != nil {
		return err
	}
	for _, f := range files {
		if touched[f.Filename] == nil {
			touched[f.Filename] = map[int]bool{}
		}
		touched[f.Filename][bottom] = true
		changed[bottom] = append(changed[bottom], f.Filename)
	}
	return nil
}

func stacks(open []PR, trunk string) map[int][]PR {
	byHead := map[string]PR{}
	for _, pr := range open {
		byHead[pr.Head.Ref] = pr
	}
	out := map[int][]PR{}
	for _, pr := range open {
		if bottom, ok := stackBottom(pr, byHead, trunk, len(open)); ok {
			out[bottom] = append(out[bottom], pr)
		}
	}
	return out
}

func stackBottom(pr PR, byHead map[string]PR, trunk string, limit int) (int, bool) {
	cur := pr
	for range limit {
		if cur.Base.Ref == trunk {
			return cur.Number, true
		}
		parent, ok := byHead[cur.Base.Ref]
		if !ok {
			return 0, false
		}
		cur = parent
	}
	return 0, false
}

func prList(ns []int) string {
	s := make([]string, len(ns))
	for i, n := range ns {
		s[i] = fmt.Sprintf("#%d", n)
	}
	return strings.Join(s, " ")
}
