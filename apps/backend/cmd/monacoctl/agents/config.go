package agents

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/monaco/monaco/apps/backend/internal/errs"
)

var errWantKeyValue = errors.New("want key = value")

const configPath = ".monaco/agents.toml"

type Config struct {
	Repo                 string
	FeatureBranch        string
	Tracking             int
	Lanes                int
	VerifierApp          string
	VerifierInstallation int
	Milestone            string
}

func parseConfig(r io.Reader) (Config, error) {
	var c Config
	seen := map[string]bool{}
	strs := map[string]*string{
		"repo": &c.Repo, "feature_branch": &c.FeatureBranch, "verifier_app": &c.VerifierApp,
		"milestone": &c.Milestone,
	}
	ints := map[string]*int{
		"tracking": &c.Tracking, "lanes": &c.Lanes, "verifier_installation": &c.VerifierInstallation,
	}
	sc := bufio.NewScanner(r)
	for n := 1; sc.Scan(); n++ {
		if err := applyConfigLine(seen, strs, ints, n, sc.Text()); err != nil {
			return Config{}, err
		}
	}
	if err := sc.Err(); err != nil {
		return Config{}, fmt.Errorf("read %s: %w", configPath, err)
	}
	for _, key := range []string{
		"repo", "feature_branch", "tracking", "lanes", "verifier_app", "verifier_installation",
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
	return c, nil
}

func applyConfigLine(seen map[string]bool, strs map[string]*string, ints map[string]*int, n int, text string) error {
	line := strings.TrimSpace(text)
	if line == "" || strings.HasPrefix(line, "#") {
		return nil
	}
	key, raw, ok := strings.Cut(line, "=")
	key, raw = strings.TrimSpace(key), strings.TrimSpace(raw)
	err := assignConfig(strs, ints, key, raw, ok)
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

type unknownKeyError struct{ key string }

func (e unknownKeyError) Error() string { return "unknown key " + strconv.Quote(e.key) }
