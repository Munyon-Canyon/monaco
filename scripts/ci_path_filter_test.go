package scripts_test

import (
	"os"
	"reflect"
	"regexp"
	"strings"
	"testing"
)

func TestCIPathFilter_jobsFollowTheWorkflow(t *testing.T) {
	filters := parsePathFilters(t)
	cases := []struct {
		name  string
		files []string
		jobs  []string
	}{
		{"backend-only", []string{"apps/backend/internal/platform/db/db.go"}, []string{"lint", "ready", "backend"}},
		{"mobile-core-only", []string{"packages/mobile-core/Sources/Foo.swift"}, []string{"mobile-core", "ios"}},
		{"ios-only", []string{"apps/mobile/App.swift"}, []string{"ios"}},
		{"openapi-only", []string{"apps/backend/api/openapi.yaml"}, []string{"lint", "ready", "backend", "mobile-core", "ios"}},
		{"xcode-version-only", []string{".xcode-version"}, []string{"mobile-core", "ios"}},
		{"ci-only", []string{".github/workflows/ci.yml"}, []string{"actionlint"}},
		{"stamp-log-only", []string{"scripts/ci/stamp-log.sh"}, []string{"ios"}},
		{"toolchain-probe-only", []string{"scripts/ci/toolchain-probe.sh"}, []string{"ios"}},
		{"probe-list-only", []string{"scripts/ci/probe-list-resolved.sh"}, []string{"ios"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := jobsFor(filters, tc.files)
			if !reflect.DeepEqual(got, tc.jobs) {
				t.Fatalf("jobs = %v, want %v", got, tc.jobs)
			}
		})
	}
	got := jobsFor(filters, []string{"apps/mobile/App.swift"})
	for _, job := range got {
		if job == "backend" || job == "flake" {
			t.Fatalf("planted ios-only list ran %s", job)
		}
	}
}

func jobsFor(filters map[string][]string, files []string) []string {
	hit := map[string]bool{}
	for name, pats := range filters {
		for _, f := range files {
			for _, pat := range pats {
				if globMatch(pat, f) {
					hit[name] = true
				}
			}
		}
	}
	var jobs []string
	if hit["backend"] {
		jobs = append(jobs, "lint", "ready", "backend")
	}
	if hit["mobile-core"] {
		jobs = append(jobs, "mobile-core")
	}
	if hit["ios"] {
		jobs = append(jobs, "ios")
	}
	if hit["backend-tests"] {
		jobs = append(jobs, "flake")
	}
	if hit["scripts"] {
		jobs = append(jobs, "scripts")
	}
	if hit["web"] {
		jobs = append(jobs, "web")
	}
	if hit["workflows"] {
		jobs = append(jobs, "actionlint")
	}
	return jobs
}

func parsePathFilters(t *testing.T) map[string][]string {
	t.Helper()
	text, err := os.ReadFile(repoRoot(t) + "/.github/workflows/ci-jobs.yml")
	if err != nil {
		t.Fatal(err)
	}
	marker := "filters: |"
	start := strings.Index(string(text), marker)
	if start < 0 {
		t.Fatal("ci-jobs.yml has no filters block")
	}
	rest := strings.Split(string(text)[start+len(marker):], "\n")
	filters := map[string][]string{}
	name := ""
	nameRe := regexp.MustCompile(`^            ([a-z0-9-]+):\s*$`)
	patRe := regexp.MustCompile(`^              - '([^']+)'\s*$`)
	for _, line := range rest[1:] {
		if strings.TrimSpace(line) == "" || !strings.HasPrefix(line, "            ") {
			break
		}
		if m := nameRe.FindStringSubmatch(line); m != nil {
			name = m[1]
			filters[name] = nil
			continue
		}
		if m := patRe.FindStringSubmatch(line); m != nil && name != "" {
			filters[name] = append(filters[name], m[1])
		}
	}
	if len(filters) == 0 {
		t.Fatal("parsed no path filters")
	}
	return filters
}

func globMatch(pattern, path string) bool {
	var b strings.Builder
	b.WriteString("^")
	for i := 0; i < len(pattern); i++ {
		c := pattern[i]
		switch c {
		case '*':
			if i+1 < len(pattern) && pattern[i+1] == '*' {
				if i+2 < len(pattern) && pattern[i+2] == '/' {
					b.WriteString("(?:.*/)?")
					i += 2
					continue
				}
				b.WriteString(".*")
				i++
				continue
			}
			b.WriteString("[^/]*")
		case '?':
			b.WriteString("[^/]")
		case '.', '+', '(', ')', '|', '^', '$', '{', '}', '[', ']', '\\':
			b.WriteByte('\\')
			b.WriteByte(c)
		default:
			b.WriteByte(c)
		}
	}
	b.WriteString("$")
	ok, err := regexp.MatchString(b.String(), path)
	return err == nil && ok
}
