package agents

import (
	"cmp"
	"context"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
)

func (env *Env) touchesRow(ctx context.Context, self, base string, changed []string) ([]checkRow, error) {
	rec, ok, err := env.ownRecord()
	if err != nil || !ok {
		return nil, err
	}
	outside := slices.DeleteFunc(slices.Clone(changed), func(f string) bool { return globsMatch(rec.Touches, f) })
	row := checkRow{label: "touches"}
	if len(outside) == 0 {
		return []checkRow{row}, nil
	}
	in, err := env.rippleInputOf(ctx, env.Work, rec.Touches)
	if err != nil {
		return nil, err
	}
	accepted := rippleReasons(in)
	rest := slices.DeleteFunc(slices.Clone(outside), func(f string) bool { return accepted[f] != "" })
	wide, err := env.wideReasons(ctx, self, base, rest)
	if err != nil {
		return nil, err
	}
	for _, f := range outside {
		if reason := cmp.Or(accepted[f], wide[f]); reason != "" {
			row.notes = append(row.notes, fmt.Sprintf("accepted by ripple: %s (%s)", f, reason))
		}
	}
	rest = slices.DeleteFunc(rest, func(f string) bool { return wide[f] != "" })
	if len(rest) > 0 {
		row.refuse = "outside Touches: " + strings.Join(rest, ", ")
	}
	return []checkRow{row}, nil
}

func (env *Env) ownRecord() (Record, bool, error) {
	recs, err := env.records()
	if err != nil {
		return Record{}, false, err
	}
	for _, r := range recs {
		if len(r.Touches) > 0 && sameDir(r.Worktree, env.Work) {
			return r, true, nil
		}
	}
	return Record{}, false, nil
}

func (env *Env) wideReasons(
	ctx context.Context, self, base string, rest []string,
) (map[string]string, error) {
	if len(rest) == 0 {
		return map[string]string{}, nil
	}
	backend := filepath.Join(env.Work, "apps", "backend")
	var dirs []string
	for _, flag := range [][]string{nil, {"--excluded"}} {
		out, err := env.Run(ctx, backend, "", self, append([]string{"ci", "affected", "--base", base}, flag...)...)
		if err != nil {
			return nil, fmt.Errorf("find affected packages: %w", err)
		}
		for _, pkg := range strings.Fields(string(out)) {
			dirs = append(dirs, filepath.ToSlash(filepath.Join("apps/backend", pkg)))
		}
	}
	diff, err := env.Run(ctx, env.Work, "", "git", "diff", "-U0", "--no-color", base+"...HEAD", "--", "*.go")
	if err != nil {
		return nil, fmt.Errorf("diff against %s: %w", base, err)
	}
	return wideTier(rest, dirs, changedIdentifiers(string(diff)), func(f string) ([]byte, error) {
		return readIn(env.Work, f)
	}), nil
}
