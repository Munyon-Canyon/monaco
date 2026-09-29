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
	trunks, err := env.trunks(ctx)
	if err != nil {
		return err
	}
	return printForecast(ctx, env, trunks, stdout)
}

func printForecast(ctx context.Context, env *Env, trunks []string, stdout io.Writer) error {
	open, err := env.GitHub.PRs(ctx, "state=open")
	if err != nil {
		return err
	}
	for _, trunk := range trunks {
		risks, err := forecast(ctx, env, open, trunk)
		if err != nil {
			return err
		}
		if len(risks) == 0 {
			_, _ = fmt.Fprintf(stdout, "no file is touched by more than one open stack into %s\n", trunk)
			continue
		}
		_, _ = fmt.Fprintf(stdout, "%d files touched by more than one open stack into %s:\n", len(risks), trunk)
		for i, r := range risks {
			if i == maxLines-2 {
				_, _ = fmt.Fprintf(stdout, "  and %d more\n", len(risks)-i)
				break
			}
			_, _ = fmt.Fprintf(stdout, "  %s  %s\n", r.path, prList(r.bottoms))
		}
	}
	return nil
}

func forecast(ctx context.Context, env *Env, open []PR, trunk string) ([]risk, error) {
	touched := map[string]map[int]bool{}
	for bottom, stack := range stacks(open, trunk) {
		for _, pr := range stack {
			if err := touch(ctx, env, pr, bottom, touched); err != nil {
				return nil, err
			}
		}
	}
	var risks []risk
	for _, path := range slices.Sorted(maps.Keys(touched)) {
		if len(touched[path]) > 1 {
			risks = append(risks, risk{path, slices.Sorted(maps.Keys(touched[path]))})
		}
	}
	return risks, nil
}

func touch(ctx context.Context, env *Env, pr PR, bottom int, touched map[string]map[int]bool) error {
	files, err := env.GitHub.Files(ctx, pr.Number)
	if err != nil {
		return err
	}
	for _, f := range files {
		if touched[f.Filename] == nil {
			touched[f.Filename] = map[int]bool{}
		}
		touched[f.Filename][bottom] = true
	}
	return nil
}

func stacks(open []PR, trunks ...string) map[int][]PR {
	byHead := map[string]PR{}
	for _, pr := range open {
		byHead[pr.Head.Ref] = pr
	}
	landsWith := map[int]int{}
	for _, pr := range open {
		if nums, ok, err := landsNums(pr.Body); ok && err == nil && len(nums) > 0 {
			for _, n := range nums {
				landsWith[n] = nums[0]
			}
		}
	}
	out := map[int][]PR{}
	for _, pr := range open {
		if bottom, ok := stackBottom(pr, byHead, trunks, len(open)); ok {
			if first, landed := landsWith[bottom]; landed {
				bottom = first
			}
			out[bottom] = append(out[bottom], pr)
		}
	}
	return out
}

func stackBottom(pr PR, byHead map[string]PR, trunks []string, limit int) (int, bool) {
	cur := pr
	for range limit {
		if slices.Contains(trunks, cur.Base.Ref) {
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
