package scripts_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"reflect"
	"regexp"
	"slices"
	"strconv"
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
		{"app-only", []string{"apps/mobile/App.swift"}, []string{"mobile-core", "ios"}},
		{"systemping-only", []string{"apps/mobile/Monaco/Features/SystemPing/SystemPingView.swift"}, []string{"mobile-core", "ios"}},
		{"pbxproj-only", []string{"apps/mobile/Monaco.xcodeproj/project.pbxproj"}, []string{"ios"}},
		{"ios-workflow-only", []string{".github/workflows/ci-ios.yml"}, []string{"ios", "scripts", "actionlint"}},
		{"generator-only", []string{"scripts/gen-mobile-feature.sh"}, []string{"mobile-core"}},
		{"mobile-integration-script-only", []string{"scripts/ci/mobile-integration.sh"}, []string{"mobile-core", "scripts"}},
		{"prune-caches-script-only", []string{"scripts/ci/prune-caches.sh"}, []string{"scripts"}},
		{"xcode-lock-script-only", []string{"scripts/qa/xcode-lock.sh"}, []string{"scripts"}},
		{"record-clip-script-only", []string{"scripts/demo/record-clip.sh"}, []string{"scripts"}},
		{"openapi-only", []string{"apps/backend/api/openapi.yaml"}, []string{"lint", "ready", "backend", "mobile-core"}},
		{"xcode-version-only", []string{".xcode-version"}, []string{"mobile-core", "ios"}},
		{"mobile-core-script-only", []string{"scripts/mobile-core-test.sh"}, []string{"mobile-core"}},
		{"flows-only", []string{"packages/flows/app/00.tsv"}, []string{"lint", "ready", "backend", "mobile-core", "ios"}},
		{"ci-only", []string{".github/workflows/ci.yml"}, []string{"scripts", "actionlint"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := jobsFor(filters, modified(tc.files...))
			if !reflect.DeepEqual(got, tc.jobs) {
				t.Fatalf("jobs = %v, want %v", got, tc.jobs)
			}
		})
	}
	got := jobsFor(filters, modified("apps/mobile/App.swift"))
	for _, job := range got {
		if job == "backend" || job == "flake" {
			t.Fatalf("apps/mobile change ran %s", job)
		}
	}
}

func TestCIPathFilter_scriptsRunWhenAFileTheyReadChanges(t *testing.T) {
	filters := parsePathFilters(t)
	for _, p := range outsideScriptsFilter(filters, readRepoPaths(t)) {
		t.Errorf("%s is read by a scripts test but is not in the scripts filter of ci-jobs.yml", p)
	}
	opensWithoutReadRepo := []struct{ path, reader string }{
		{".claude/skills/any/SKILL.md", "skill_paths_test.go"},
		{".cursor/skills/any/SKILL.md", "skill_paths_test.go"},
		{".github/pull_request_template.md", "test_check_pr_format.py"},
		{".github/workflows/mutation.yml", "tool_manifest_test.go"},
		{"packages/mobile-core/Tests/MonacoCoreTests/LegacyFreezeTests.swift", "test_check_legacy_growth.py"},
	}
	for _, in := range opensWithoutReadRepo {
		if len(outsideScriptsFilter(filters, []string{in.path})) > 0 {
			t.Errorf("%s is opened by %s but is not in the scripts filter of ci-jobs.yml", in.path, in.reader)
		}
	}
	planted := []string{"docs/not-read.md"}
	if got := outsideScriptsFilter(filters, planted); !reflect.DeepEqual(got, planted) {
		t.Fatalf("planted path outside the filter: got %v, want %v", got, planted)
	}
}

func TestCIPathFilter_scriptsRunWhenAPRDeletesOrRenamesAFile(t *testing.T) {
	filters := parsePathFilters(t)
	cases := []struct {
		name    string
		changes []change
	}{
		{"delete", []change{{"deleted", "docs/removed.md"}}},
		{"rename", []change{{"deleted", "docs/before.md"}, {"added", "docs/after.md"}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := jobsFor(filters, tc.changes); !reflect.DeepEqual(got, []string{"scripts"}) {
				t.Fatalf("jobs = %v, want [scripts]", got)
			}
		})
	}
}

type filterRule struct{ status, pattern string }

type change struct{ status, path string }

func (r filterRule) matches(c change) bool {
	return (r.status == "" || slices.Contains(strings.Split(r.status, "|"), c.status)) && globMatch(r.pattern, c.path)
}

func modified(paths ...string) []change {
	changes := make([]change, len(paths))
	for i, p := range paths {
		changes[i] = change{"modified", p}
	}
	return changes
}

func jobsFor(filters map[string][]filterRule, changes []change) []string {
	hit := map[string]bool{}
	for name, rules := range filters {
		for _, c := range changes {
			for _, r := range rules {
				if r.matches(c) {
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

func outsideScriptsFilter(filters map[string][]filterRule, paths []string) []string {
	var out []string
	for _, p := range paths {
		if !slices.Contains(jobsFor(filters, modified(p)), "scripts") {
			out = append(out, p)
		}
	}
	return out
}

func readRepoPaths(t *testing.T) []string {
	t.Helper()
	fset := token.NewFileSet()
	var paths []string
	err := filepath.WalkDir(filepath.Join(repoRoot(t), "scripts"), func(file string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if d.Name() == "testdata" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(file, "_test.go") {
			return nil
		}
		parsed, err := parser.ParseFile(fset, file, nil, parser.SkipObjectResolution)
		if err != nil {
			return err
		}
		ast.Inspect(parsed, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok || len(call.Args) != 3 {
				return true
			}
			fn, _ := call.Fun.(*ast.Ident)
			lit, _ := call.Args[2].(*ast.BasicLit)
			if fn != nil && fn.Name == "readRepo" && lit != nil && lit.Kind == token.STRING {
				p, _ := strconv.Unquote(lit.Value)
				paths = append(paths, p)
			}
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) == 0 {
		t.Fatal("found no readRepo call with a literal path in the scripts tests")
	}
	slices.Sort(paths)
	return slices.Compact(paths)
}

func parsePathFilters(t *testing.T) map[string][]filterRule {
	t.Helper()
	text := readRepo(t, repoRoot(t), ".github/workflows/ci-jobs.yml")
	marker := "filters: |"
	start := strings.Index(text, marker)
	if start < 0 {
		t.Fatal("ci-jobs.yml has no filters block")
	}
	rest := strings.Split(text[start+len(marker):], "\n")
	filters := map[string][]filterRule{}
	name := ""
	nameRe := regexp.MustCompile(`^            ([a-z0-9-]+):\s*$`)
	patRe := regexp.MustCompile(`^              - (?:([a-z|]+): )?'([^']+)'\s*$`)
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
			filters[name] = append(filters[name], filterRule{status: m[1], pattern: m[2]})
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
