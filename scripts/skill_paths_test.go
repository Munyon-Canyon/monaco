package scripts_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
)

var backendSkills = []string{"go-backend-module", "go-concurrency", "money-change", "nats-consumer", "verify-backend"}

var codeSpan = regexp.MustCompile("`([^`\n]+)`")

func pathSpans(markdown string) []string {
	var paths []string
	for _, m := range codeSpan.FindAllStringSubmatch(markdown, -1) {
		span := m[1]
		if !strings.Contains(span, "/") || strings.ContainsAny(span, " <>*{}$|") ||
			strings.HasPrefix(span, "/") || strings.Contains(span, "://") {
			continue
		}
		first, _, _ := strings.Cut(strings.TrimPrefix(span, "./"), "/")
		if strings.Contains(strings.TrimPrefix(first, "."), ".") {
			continue
		}
		span, _, _ = strings.Cut(span, "#")
		paths = append(paths, span)
	}
	return paths
}

func goStdlib(t *testing.T) string {
	t.Helper()
	out, err := exec.Command("go", "env", "GOROOT").Output()
	if err != nil {
		t.Fatalf("go env GOROOT: %v", err)
	}
	return filepath.Join(strings.TrimSpace(string(out)), "src")
}

func missingPaths(t *testing.T, root, markdown string) []string {
	t.Helper()
	stdlib := goStdlib(t)
	var missing []string
	for _, p := range pathSpans(markdown) {
		if info, err := os.Stat(filepath.Join(stdlib, p)); err == nil && info.IsDir() {
			continue
		}
		found := false
		for _, base := range []string{root, filepath.Join(root, "apps", "backend")} {
			if _, err := os.Stat(filepath.Join(base, p)); err == nil {
				found = true
				break
			}
		}
		if !found {
			missing = append(missing, p)
		}
	}
	return missing
}

func TestSkillPaths_everyBacktickedPathInABackendSkillExists(t *testing.T) {
	root := repoRoot(t)
	for _, skill := range backendSkills {
		files, err := filepath.Glob(filepath.Join(root, ".claude", "skills", skill, "*.md"))
		if err != nil || !slices.Contains(files, filepath.Join(root, ".claude", "skills", skill, "SKILL.md")) {
			t.Fatalf("%s: no SKILL.md (files %v, err %v)", skill, files, err)
		}
		for _, f := range files {
			body, err := os.ReadFile(f)
			if err != nil {
				t.Fatal(err)
			}
			rel, _ := filepath.Rel(root, f)
			if len(pathSpans(string(body))) == 0 {
				t.Errorf("%s cites no repo paths", rel)
			}
			for _, p := range missingPaths(t, root, string(body)) {
				t.Errorf("%s cites `%s`, which exists neither from the repo root nor from apps/backend", rel, p)
			}
		}
	}
}

func TestSkillPaths_eachBackendSkillIsUnder150LinesAndMirroredForCursor(t *testing.T) {
	root := repoRoot(t)
	for _, skill := range backendSkills {
		claude := filepath.Join(root, ".claude", "skills", skill, "SKILL.md")
		body, err := os.ReadFile(claude)
		if err != nil {
			t.Fatal(err)
		}
		if lines := strings.Count(string(body), "\n"); lines >= 150 {
			t.Errorf("%s/SKILL.md has %d lines, want under 150", skill, lines)
		}
		if !strings.HasPrefix(string(body), "---\nname: "+skill+"\ndescription: ") {
			t.Errorf("%s/SKILL.md does not open with name and description front matter", skill)
		}
		cursor, err := os.ReadFile(filepath.Join(root, ".cursor", "skills", skill, "SKILL.md"))
		if err != nil || string(cursor) != string(body) {
			t.Errorf(".cursor/skills/%s/SKILL.md does not mirror .claude/skills/%s/SKILL.md (err %v)", skill, skill, err)
		}
	}
}

func TestSkillPaths_flagsAPlantedMissingPathAndSkipsNonPaths(t *testing.T) {
	root := repoRoot(t)
	markdown := "See `apps/backend/flows.tsv`, `internal/platform/db/uow.go#L1`, `./cmd/monacoctl`, " +
		"`apps/backend/internal/nope.go` and `math/big`. " +
		"Not paths: `go run ./cmd/monacoctl verify`, `queries/<module>/x.sql`, `POST /v1/system/pings`, `/v1/system/pings`, `go.uber.org/goleak`, `system.pinged`, `https://x.dev/a`."
	wantSpans := []string{
		"apps/backend/flows.tsv", "internal/platform/db/uow.go", "./cmd/monacoctl", "apps/backend/internal/nope.go",
		"math/big",
	}
	if got := pathSpans(markdown); !slices.Equal(got, wantSpans) {
		t.Fatalf("pathSpans = %q, want %q", got, wantSpans)
	}
	if got := missingPaths(t, root, markdown); !slices.Equal(got, []string{"apps/backend/internal/nope.go"}) {
		t.Fatalf("missingPaths = %q, want only the planted one", got)
	}
}
