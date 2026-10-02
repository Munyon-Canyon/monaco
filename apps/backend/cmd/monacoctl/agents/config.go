package agents

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
)

var (
	errWantKeyValue   = errors.New("want key = value")
	errUnknownSection = errors.New("unknown section")
	errBadBudget      = errors.New("want a positive duration such as \"60s\"")
	errWantList       = errors.New("want a list of quoted strings such as [\"a/**\"]")
)

const (
	configPath    = ".monaco/agents.toml"
	defaultLabel  = "merge-queue"
	budgetSection = "[check.budget]"
	budgetPrefix  = "check.budget."
	checkSection  = "[check]"
	batchSection  = "[batch]"
	checkPrefix   = "check."
	batchPrefix   = "batch."
	defaultSlots  = 2
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
	Slots                int
	Budget               map[string]time.Duration
	Shared               []string
}

func parseConfig(r io.Reader) (Config, error) {
	c := Config{Budget: defaultBudget(), Slots: defaultSlots}
	section := ""
	seen := map[string]bool{}
	strs := map[string]*string{
		"repo": &c.Repo, "feature_branch": &c.FeatureBranch, "verifier_app": &c.VerifierApp,
		"milestone": &c.Milestone, "queue_label": &c.QueueLabel,
	}
	ints := map[string]*int{
		"tracking": &c.Tracking, "lanes": &c.Lanes, "batch.size": &c.Batch,
		"verifier_installation": &c.VerifierInstallation, "check.slots": &c.Slots,
	}
	lists := map[string]*[]string{"batch.shared": &c.Shared}
	lines, err := logicalLines(r)
	if err != nil {
		return Config{}, err
	}
	for _, l := range lines {
		if err := applyConfigLine(&c, &section, seen, strs, ints, lists, l.n, l.text); err != nil {
			return Config{}, err
		}
	}
	for _, key := range []string{
		"repo", "feature_branch", "tracking", "lanes", "batch.size", "verifier_app", "verifier_installation",
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
	if c.Slots < 1 {
		return Config{}, detailErr(errs.CodeDecodeFailed, "monacoctl.agents.config",
			fmt.Sprintf("%s: check.slots: want at least 1, got %d", configPath, c.Slots))
	}
	if c.QueueLabel == "" {
		c.QueueLabel = defaultLabel
	}
	return c, nil
}

type configLine struct {
	n    int
	text string
}

func logicalLines(r io.Reader) ([]configLine, error) {
	var (
		out  []configLine
		open *configLine
	)
	sc := bufio.NewScanner(r)
	for n := 1; sc.Scan(); n++ {
		line := sc.Text()
		switch trimmed := strings.TrimSpace(line); {
		case open == nil && !opensList(line):
			out = append(out, configLine{n, line})
		case open == nil:
			open = &configLine{n, trimmed}
		case !strings.HasPrefix(trimmed, "#"):
			open.text += trimmed
		}
		if open != nil && strings.HasSuffix(open.text, "]") {
			out, open = append(out, *open), nil
		}
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("read %s: %w", configPath, err)
	}
	if open != nil {
		return nil, configLineErr(open.n, errWantList)
	}
	return out, nil
}

func opensList(line string) bool {
	_, raw, ok := strings.Cut(line, "=")
	raw = strings.TrimSpace(raw)
	return ok && strings.HasPrefix(raw, "[") && !strings.HasSuffix(raw, "]")
}

func applyConfigLine(
	c *Config, section *string, seen map[string]bool, strs map[string]*string, ints map[string]*int,
	lists map[string]*[]string, n int, text string,
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
	case line == checkSection:
		*section = checkPrefix
		return nil
	case line == batchSection:
		*section = batchPrefix
		return nil
	case strings.HasPrefix(line, "["):
		err = fmt.Errorf("%w %s", errUnknownSection, line)
	case strings.HasPrefix(key, budgetPrefix):
		err = assignBudget(c.Budget, strings.TrimPrefix(key, budgetPrefix), raw, ok)
	case lists[key] != nil:
		*lists[key], err = parseList(raw)
	default:
		err = assignConfig(strs, ints, key, raw, ok)
	}
	if err != nil {
		return configLineErr(n, err)
	}
	seen[key] = true
	return nil
}

func configLineErr(n int, err error) error {
	return detailErr(
		errs.CodeDecodeFailed,
		"monacoctl.agents.config",
		fmt.Sprintf("%s:%d: %s", configPath, n, err.Error()),
	)
}

func parseList(raw string) ([]string, error) {
	if !strings.HasPrefix(raw, "[") || !strings.HasSuffix(raw, "]") {
		return nil, errWantList
	}
	inner := raw[1 : len(raw)-1]
	var out []string
	for item := range strings.SplitSeq(inner, ",") {
		if item = strings.TrimSpace(item); item == "" {
			continue
		}
		v, err := strconv.Unquote(item)
		if err != nil {
			return nil, fmt.Errorf("%w, got %s", errWantList, item)
		}
		out = append(out, v)
	}
	return out, nil
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

func resolveFeatureBranch(
	ctx context.Context, run Runner, gh *GitHub, environ []string, dir string, cfg Config,
) (string, string, error) {
	if cfg.FeatureBranch != autoFeatureBranch {
		return cfg.FeatureBranch, "", nil
	}
	if name := lookup(environ, featureBranchEnv); name != "" {
		return name, "", nil
	}
	out, err := run(ctx, dir, "", "gh", "variable", "get", featureBranchVar, "--repo", cfg.Repo)
	name := strings.TrimSpace(string(out))
	if err == nil && name != "" {
		return name, "", nil
	}
	reason := "printed nothing"
	if err != nil {
		reason = err.Error()
	}
	branch, repoErr := repoDefaultBranch(ctx, gh, cfg.Repo)
	if repoErr != nil || branch == "" {
		repoReason := "printed nothing"
		if repoErr != nil {
			repoReason = repoErr.Error()
		}
		return "", "", detailErr(
			errs.CodeNotFound,
			"monacoctl.agents.config",
			fmt.Sprintf(
				"%s: feature_branch = %q, but %s is unset, gh variable get %s --repo %s %s, "+
					"and GET /repos/%s default_branch %s",
				configPath, autoFeatureBranch, featureBranchEnv, featureBranchVar, cfg.Repo, reason,
				cfg.Repo, repoReason,
			),
		)
	}
	note := fmt.Sprintf(
		"feature branch %s (repo default branch; gh variable get failed: %s)", branch, reason,
	)
	return branch, note, nil
}

func repoDefaultBranch(ctx context.Context, gh *GitHub, repo string) (string, error) {
	var body struct {
		DefaultBranch string `json:"default_branch"`
	}
	if err := gh.call(ctx, http.MethodGet, "/repos/"+repo, "", nil, &body); err != nil {
		return "", err
	}
	return body.DefaultBranch, nil
}
