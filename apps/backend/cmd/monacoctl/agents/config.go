package agents

import (
	"bufio"
	"fmt"
	"io"
	"strconv"
	"strings"
)

const configPath = ".monaco/agents.toml"

type Config struct {
	Repo          string
	FeatureBranch string
	Tracking      int
	Lanes         int
}

func parseConfig(r io.Reader) (Config, error) {
	var c Config
	seen := map[string]bool{}
	strs := map[string]*string{"repo": &c.Repo, "feature_branch": &c.FeatureBranch}
	ints := map[string]*int{"tracking": &c.Tracking, "lanes": &c.Lanes}
	sc := bufio.NewScanner(r)
	for n := 1; sc.Scan(); n++ {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, raw, ok := strings.Cut(line, "=")
		key, raw = strings.TrimSpace(key), strings.TrimSpace(raw)
		var err error
		switch {
		case !ok:
			err = failure("want key = value")
		case strs[key] != nil:
			*strs[key], err = strconv.Unquote(raw)
		case ints[key] != nil:
			*ints[key], err = strconv.Atoi(raw)
		default:
			err = failf("unknown key %q", key)
		}
		if err != nil {
			return Config{}, fmt.Errorf("%s:%d: %w", configPath, n, err)
		}
		seen[key] = true
	}
	if err := sc.Err(); err != nil {
		return Config{}, fmt.Errorf("read %s: %w", configPath, err)
	}
	for _, key := range []string{"repo", "feature_branch", "tracking", "lanes"} {
		if !seen[key] {
			return Config{}, failf("%s: missing %s", configPath, key)
		}
	}
	return c, nil
}
