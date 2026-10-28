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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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

func TestCIPathFilter_scriptsSkipADeletionNoScriptsTestCanCite(t *testing.T) {
	t.Parallel()
	filters := parsePathFilters(t)
	cases := []struct {
		name    string
		changes []change
		jobs    []string
	}{
		{"mobile-delete", []change{{"deleted", "apps/mobile/Monaco/Features/Withdraw/WithdrawView.swift"}}, []string{"mobile-core", "ios"}},
		{"mobile-rename", []change{{"deleted", "apps/mobile/Monaco/A.swift"}, {"added", "apps/mobile/Monaco/B.swift"}}, []string{"mobile-core", "ios"}},
		{"mobile-core-delete", []change{{"deleted", "packages/mobile-core/Sources/MonacoCore/Old.swift"}}, []string{"mobile-core", "ios"}},
		{"mobile-and-mobile-core-delete", []change{
			{"deleted", "apps/mobile/MonacoTests/OldTests.swift"},
			{"deleted", "packages/mobile-core/Tests/MonacoCoreTests/OldTests.swift"},
			{"modified", "apps/mobile/Monaco/RepoRulesAllowlist.txt"},
		}, []string{"mobile-core", "ios"}},
		{"web-delete", []change{{"deleted", "apps/web/src/old.ts"}}, []string{"web"}},
		{"backend-delete", []change{{"deleted", "apps/backend/internal/platform/db/uow.go"}}, []string{"lint", "ready", "backend", "scripts"}},
		{"flows-delete", []change{{"deleted", "packages/flows/backend/00.tsv"}}, []string{"lint", "ready", "backend", "mobile-core", "ios", "scripts"}},
		{"skill-delete", []change{{"deleted", ".claude/skills/commit/SKILL.md"}}, []string{"scripts"}},
		{"mobile-delete-with-docs-delete", []change{
			{"deleted", "apps/mobile/Monaco/A.swift"},
			{"deleted", "docs/removed.md"},
		}, []string{"mobile-core", "ios", "scripts"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := jobsFor(filters, tc.changes)
			slices.Sort(got)
			want := slices.Clone(tc.jobs)
			slices.Sort(want)
			if !slices.Equal(got, want) {
				t.Fatalf("jobs = %v, want %v", got, want)
			}
		})
	}
}

func TestCIPathFilter_scriptsDeletionOutputFeedsThePlanScriptsOutput(t *testing.T) {
	t.Parallel()
	text := readRepo(t, repoRoot(t), ".github/workflows/ci-jobs.yml")
	if !strings.Contains(text, "steps.every.outputs.scripts-deleted == 'true'") {
		t.Fatal("the plan scripts output does not read steps.every.outputs.scripts-deleted")
	}
}

type filterRule struct{ status, pattern string }

type change struct{ status, path string }

func (r filterRule) matches(c change) bool {
	return (r.status == "" || slices.Contains(strings.Split(r.status, "|"), c.status)) && globMatch(r.pattern, c.path)
}

const everyPrefix = "every/"

func everyMatches(rules []filterRule, c change) bool {
	if len(rules) == 0 {
		return false
	}
	for _, r := range rules {
		if negated, ok := strings.CutPrefix(r.pattern, "!"); ok {
			if (r.status == "" || slices.Contains(strings.Split(r.status, "|"), c.status)) && globMatch(negated, c.path) {
				return false
			}
			continue
		}
		if !r.matches(c) {
			return false
		}
	}
	return true
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
		if strings.HasPrefix(name, everyPrefix) {
			continue
		}
		for _, c := range changes {
			for _, r := range rules {
				if r.matches(c) {
					hit[name] = true
				}
			}
		}
	}
	for _, c := range changes {
		if everyMatches(filters[everyPrefix+"scripts-deleted"], c) {
			hit["scripts"] = true
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
	filters := parseFilterBlock(t, text[start+len(marker):], "")
	everyMarker := "predicate-quantifier: every\n          filters: |"
	everyStart := strings.Index(text, everyMarker)
	if everyStart < 0 {
		t.Fatal("ci-jobs.yml has no filters block under predicate-quantifier: every")
	}
	for name, rules := range parseFilterBlock(t, text[everyStart+len(everyMarker):], everyPrefix) {
		filters[name] = rules
	}
	return filters
}

func parseFilterBlock(t *testing.T, block, prefix string) map[string][]filterRule {
	t.Helper()
	rest := strings.Split(block, "\n")
	filters := map[string][]filterRule{}
	name := ""
	nameRe := regexp.MustCompile(`^            ([a-z0-9-]+):\s*$`)
	patRe := regexp.MustCompile(`^              - (?:([a-z|]+): )?'([^']+)'\s*$`)
	for _, line := range rest[1:] {
		if strings.TrimSpace(line) == "" || !strings.HasPrefix(line, "            ") {
			break
		}
		if m := nameRe.FindStringSubmatch(line); m != nil {
			name = prefix + m[1]
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
