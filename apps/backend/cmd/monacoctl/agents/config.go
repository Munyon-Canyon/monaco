package agents

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
)

var (
	errWantKeyValue   = errors.New("want key = value")
	errUnknownSection = errors.New("unknown section")
	errBadBudget      = errors.New("want a positive duration such as \"60s\"")
)

const (
	configPath    = ".monaco/agents.toml"
	defaultLabel  = "merge-queue"
	budgetSection = "[check.budget]"
	budgetPrefix  = "check.budget."
)

func defaultBudget() map[string]time.Duration {
	return map[string]time.Duration{
		"go": 60 * time.Second, "lint": 60 * time.Second, "swift": 150 * time.Second, "xcode": 300 * time.Second,
		"scripts": 30 * time.Second,
		"python":  45 * time.Second, "shell": 10 * time.Second, "ready": 90 * time.Second,
		"migrate": 30 * time.Second, "openapi": 30 * time.Second, "docs": 30 * time.Second,
		"pr": 15 * time.Second, packageKind: 20 * time.Second,
	}
}

type Config struct {
	Repo                 string
	FeatureBranch        string
	Tracking             int
	Lanes                int
	Batch                int
	VerifierApp          string
	VerifierInstallation int
	Milestone            string
	QueueLabel           string
	Budget               map[string]time.Duration
}

func parseConfig(r io.Reader) (Config, error) {
	c := Config{Budget: defaultBudget()}
	section := ""
	seen := map[string]bool{}
	strs := map[string]*string{
		"repo": &c.Repo, "feature_branch": &c.FeatureBranch, "verifier_app": &c.VerifierApp,
		"milestone": &c.Milestone, "queue_label": &c.QueueLabel,
	}
	ints := map[string]*int{
		"tracking": &c.Tracking, "lanes": &c.Lanes, "batch": &c.Batch, "verifier_installation": &c.VerifierInstallation,
	}
	sc := bufio.NewScanner(r)
	for n := 1; sc.Scan(); n++ {
		if err := applyConfigLine(&c, &section, seen, strs, ints, n, sc.Text()); err != nil {
			return Config{}, err
		}
	}
	if err := sc.Err(); err != nil {
		return Config{}, fmt.Errorf("read %s: %w", configPath, err)
	}
	for _, key := range []string{
		"repo", "feature_branch", "tracking", "lanes", "batch", "verifier_app", "verifier_installation",
		"milestone",
	} {
		if !seen[key] {
			return Config{}, detailErr(
				errs.CodeDecodeFailed,
				"monacoctl.agents.config",
				fmt.Sprintf("%s: missing %s", configPath, key),
			)
		}
	}
	if c.QueueLabel == "" {
		c.QueueLabel = defaultLabel
	}
	return c, nil
}

func applyConfigLine(
	c *Config, section *string, seen map[string]bool, strs map[string]*string, ints map[string]*int, n int, text string,
) error {
	line := strings.TrimSpace(text)
	if line == "" || strings.HasPrefix(line, "#") {
		return nil
	}
	key, raw, ok := strings.Cut(line, "=")
	key, raw = *section+strings.TrimSpace(key), strings.TrimSpace(raw)
	var err error
	switch {
	case line == budgetSection:
		*section = budgetPrefix
		return nil
	case strings.HasPrefix(line, "["):
		err = fmt.Errorf("%w %s", errUnknownSection, line)
	case strings.HasPrefix(key, budgetPrefix):
		err = assignBudget(c.Budget, strings.TrimPrefix(key, budgetPrefix), raw, ok)
	default:
		err = assignConfig(strs, ints, key, raw, ok)
	}
	if err != nil {
		return detailErr(
			errs.CodeDecodeFailed,
			"monacoctl.agents.config",
			fmt.Sprintf("%s:%d: %s", configPath, n, err.Error()),
		)
	}
	seen[key] = true
	return nil
}

func assignConfig(strs map[string]*string, ints map[string]*int, key, raw string, ok bool) error {
	switch {
	case !ok:
		return errWantKeyValue
	case strs[key] != nil:
		v, err := strconv.Unquote(raw)
		if err != nil {
			return fmt.Errorf("quote: %w", err)
		}
		*strs[key] = v
		return nil
	case ints[key] != nil:
		v, err := strconv.Atoi(raw)
		if err != nil {
			return fmt.Errorf("int: %w", err)
		}
		*ints[key] = v
		return nil
	default:
		return unknownKeyError{key: key}
	}
}

func assignBudget(budget map[string]time.Duration, kind, raw string, ok bool) error {
	if _, known := budget[kind]; !known || !ok {
		return assignConfig(nil, nil, budgetPrefix+kind, raw, ok)
	}
	v, err := strconv.Unquote(raw)
	if err != nil {
		return fmt.Errorf("quote: %w", err)
	}
	d, err := time.ParseDuration(v)
	if err != nil || d <= 0 {
		return fmt.Errorf("budget %s: %w, got %q", kind, errBadBudget, v)
	}
	budget[kind] = d
	return nil
}

type unknownKeyError struct{ key string }

func (e unknownKeyError) Error() string { return "unknown key " + strconv.Quote(e.key) }

const (
	autoFeatureBranch = "auto"
	featureBranchEnv  = "MONACO_FEATURE_BRANCH"
	featureBranchVar  = "FEATURE_BRANCH"
)

func resolveFeatureBranch(ctx context.Context, run Runner, environ []string, dir string, cfg Config) (string, error) {
	if cfg.FeatureBranch != autoFeatureBranch {
		return cfg.FeatureBranch, nil
	}
	if name := lookup(environ, featureBranchEnv); name != "" {
		return name, nil
	}
	out, err := run(ctx, dir, "", "gh", "variable", "get", featureBranchVar, "--repo", cfg.Repo)
	name := strings.TrimSpace(string(out))
	if err == nil && name != "" {
		return name, nil
	}
	reason := "printed nothing"
	if err != nil {
		reason = err.Error()
	}
	return "", detailErr(
		errs.CodeNotFound,
		"monacoctl.agents.config",
		fmt.Sprintf(
			"%s: feature_branch = %q, but %s is unset and gh variable get %s --repo %s %s",
			configPath, autoFeatureBranch, featureBranchEnv, featureBranchVar, cfg.Repo, reason,
		),
	)
}
