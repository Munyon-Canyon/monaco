package scripts_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBackendAgentsMD_staysUnder60LinesAndCitesOnlyRealPaths(t *testing.T) {
	root := repoRoot(t)
	body, err := os.ReadFile(filepath.Join(root, "apps", "backend", "AGENTS.md"))
	if err != nil {
		t.Fatal(err)
	}
	if lines := strings.Count(string(body), "\n"); lines >= 60 {
		t.Errorf("apps/backend/AGENTS.md has %d lines, want under 60; move the detail into a skill or a lint", lines)
	}
	for _, p := range missingPaths(t, root, string(body)) {
		t.Errorf("apps/backend/AGENTS.md cites `%s`, which exists neither from the repo root nor from apps/backend", p)
	}
	for _, skill := range backendSkills {
		if !strings.Contains(string(body), "`.claude/skills/"+skill+"/SKILL.md`") {
			t.Errorf("apps/backend/AGENTS.md does not point at the %s skill", skill)
		}
	}
	for _, doc := range []string{"`docs/agents/owner.md`", "`docs/architecture/backend-platform.md`"} {
		if !strings.Contains(string(body), doc) {
			t.Errorf("apps/backend/AGENTS.md does not point at %s", doc)
		}
	}
}
