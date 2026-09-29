package agents

import (
	"context"
	"fmt"
	"io"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/monaco/monaco/apps/backend/internal/errs"
)

var (
	featureBranchRE = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*-checkpoint-[0-9]+$`)
	legacyBranchRE  = regexp.MustCompile(`^([a-z0-9]+(?:-[a-z0-9]+)*)-[0-9]+$`)
	baseHeaderRE    = regexp.MustCompile("\\*\\*Base branch:\\*\\*\\s*`([^`]+)`")
	trackingRE      = regexp.MustCompile(`\*\*Tracking:\*\*\s*#([0-9]+)`)
)

const (
	autoFeatureBranch = "auto"
	featureBranchEnv  = "MONACO_FEATURE_BRANCH"
	featureBranchVar  = "FEATURE_BRANCH"
	checkpointInfix   = "-checkpoint-"
)

func featureOf(branch string) (string, bool) {
	if !featureBranchRE.MatchString(branch) {
		return "", false
	}
	return branch[:strings.LastIndex(branch, checkpointInfix)], true
}

func checkpointOf(branch string) int {
	n, _ := strconv.Atoi(branch[strings.LastIndex(branch, "-")+1:])
	return n
}

func notFeatureBranch(source, name string) error {
	return detailErr(errs.CodeInvalidInput, "monacoctl.agents.config",
		fmt.Sprintf("%s %q is not a feature branch <feature>-checkpoint-<N>", source, name))
}

func overrideBranch(flag string, environ []string, cfg Config) (string, error) {
	pinned := cfg.FeatureBranch
	if pinned == autoFeatureBranch {
		pinned = ""
	}
	for _, src := range []struct{ name, value string }{
		{"--branch", flag},
		{featureBranchEnv, lookup(environ, featureBranchEnv)},
		{configPath + " feature_branch", pinned},
	} {
		if src.value == "" {
			continue
		}
		if !featureBranchRE.MatchString(src.value) {
			return "", notFeatureBranch(src.name, src.value)
		}
		return src.value, nil
	}
	return "", nil
}

func (env *Env) liveBranches(ctx context.Context) ([]string, error) {
	out, err := env.Run(ctx, env.Work, "", "git", "ls-remote", "--heads", "origin", "*"+checkpointInfix+"*")
	if err != nil {
		return nil, fmt.Errorf("list the feature branches: %w", err)
	}
	var live []string
	for line := range strings.Lines(string(out)) {
		_, ref, _ := strings.Cut(strings.TrimSpace(line), "\t")
		if name := strings.TrimPrefix(ref, "refs/heads/"); featureBranchRE.MatchString(name) {
			live = append(live, name)
		}
	}
	slices.Sort(live)
	return live, nil
}

func (env *Env) fallbackBranch(ctx context.Context) (string, error) {
	out, err := env.Run(ctx, env.Work, "", "gh", "variable", "get", featureBranchVar, "--repo", env.Config.Repo)
	name := strings.TrimSpace(string(out))
	switch {
	case err != nil:
		return "", fmt.Errorf("read the %s repo variable: %w", featureBranchVar, err)
	case !featureBranchRE.MatchString(name):
		return "", notFeatureBranch("the "+featureBranchVar+" repo variable", name)
	}
	return name, nil
}

func (env *Env) trunks(ctx context.Context) ([]string, error) {
	if env.Branch != "" {
		return []string{env.Branch}, nil
	}
	live, err := env.liveBranches(ctx)
	if err != nil || len(live) > 0 {
		return live, err
	}
	name, err := env.fallbackBranch(ctx)
	if err != nil {
		return nil, detailErr(errs.CodeNotFound, "monacoctl.agents.config",
			"no live feature branch <feature>-checkpoint-<N> on origin, and "+cliText(err))
	}
	return []string{name}, nil
}

func (env *Env) ticketBranch(ctx context.Context, ticket int, body string, notice io.Writer) (string, error) {
	if env.Branch != "" {
		return env.Branch, nil
	}
	m := baseHeaderRE.FindStringSubmatch(body)
	if m == nil {
		name, err := env.fallbackBranch(ctx)
		if err != nil {
			return "", detailErr(errs.CodeNotFound, "monacoctl.agents.config", fmt.Sprintf(
				"#%d has no **Base branch:** header, and %s; pass --branch <feature>-checkpoint-<N>",
				ticket,
				cliText(err),
			))
		}
		return name, nil
	}
	live, err := env.liveBranches(ctx)
	if err != nil {
		return "", err
	}
	return resolveHeader(ticket, m[1], live, notice)
}

func resolveHeader(ticket int, named string, live []string, notice io.Writer) (string, error) {
	if slices.Contains(live, named) {
		return named, nil
	}
	feature, ok := featureOf(named)
	if !ok {
		legacy := legacyBranchRE.FindStringSubmatch(named)
		if legacy == nil {
			return "", notFeatureBranch(fmt.Sprintf("#%d's Base branch", ticket), named)
		}
		feature = legacy[1]
	}
	var newest string
	for _, name := range live {
		if f, _ := featureOf(name); f == feature && (newest == "" || checkpointOf(name) > checkpointOf(newest)) {
			newest = name
		}
	}
	if newest == "" {
		return "", detailErr(errs.CodeNotFound, "monacoctl.agents.config", fmt.Sprintf(
			"#%d's Base branch %s is gone, and origin has no %s-checkpoint-<N>", ticket, named, feature))
	}
	_, _ = fmt.Fprintf(notice, "#%d: Base branch %s is gone; using %s\n", ticket, named, newest)
	return newest, nil
}

func (env *Env) worktreeBranch(ctx context.Context, notice io.Writer) (string, error) {
	if env.Branch != "" {
		return env.Branch, nil
	}
	rs, err := env.records()
	if err != nil {
		return "", err
	}
	ticket, _ := strconv.Atoi(filepath.Base(env.Work))
	if i := slices.IndexFunc(rs, func(r Record) bool { return r.Worktree == env.Work }); i >= 0 {
		ticket = rs[i].Ticket
	}
	if ticket <= 0 {
		name, err := env.fallbackBranch(ctx)
		if err != nil {
			return "", detailErr(errs.CodeNotFound, "monacoctl.agents.config", fmt.Sprintf(
				"no owner record names this worktree, and %s; pass --branch <feature>-checkpoint-<N>", cliText(err)))
		}
		return name, nil
	}
	is, err := env.GitHub.Issue(ctx, ticket)
	if err != nil {
		return "", err
	}
	return env.ticketBranch(ctx, ticket, is.Body, notice)
}

func (env *Env) trackingIssue(ctx context.Context, branch string, tickets ...int) (int, error) {
	feature, _ := featureOf(branch)
	if n := env.Config.Features[feature]; n > 0 {
		return n, nil
	}
	for _, ticket := range tickets {
		is, err := env.GitHub.Issue(ctx, ticket)
		if err != nil {
			return 0, err
		}
		if m := trackingRE.FindStringSubmatch(is.Body); m != nil {
			n, _ := strconv.Atoi(m[1])
			return n, nil
		}
	}
	if env.Config.Tracking > 0 {
		return env.Config.Tracking, nil
	}
	return 0, detailErr(errs.CodeNotFound, "monacoctl.agents.config", fmt.Sprintf(
		"%s has no tracking issue; add [features.%s] with tracking = <issue> to %s", branch, feature, configPath))
}
