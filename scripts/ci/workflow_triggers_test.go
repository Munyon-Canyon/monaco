package ci_test

import (
	"bufio"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
)

var (
	topLevelKey  = regexp.MustCompile(`^[A-Za-z"'][^:]*:`)
	triggerPaths = regexp.MustCompile(`^\s+(paths|paths-ignore):`)
	jobHeader    = regexp.MustCompile(`^  ([A-Za-z0-9_-]+):\s*$`)
	jobName      = regexp.MustCompile(`^    name:\s*(.+?)\s*$`)
)

type workflow struct {
	jobs         map[string]bool
	triggerPaths []string
}

func readWorkflow(t *testing.T, path string) workflow {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	wf := workflow{jobs: map[string]bool{}}
	section := ""
	sc := bufio.NewScanner(f)
	for n := 1; sc.Scan(); n++ {
		line := sc.Text()
		if topLevelKey.MatchString(line) {
			section = strings.Trim(strings.SplitN(line, ":", 2)[0], `"'`)
			continue
		}
		switch section {
		case "on", "true":
			if triggerPaths.MatchString(line) {
				wf.triggerPaths = append(wf.triggerPaths, strings.TrimSpace(line))
			}
		case "jobs":
			if m := jobHeader.FindStringSubmatch(line); m != nil {
				wf.jobs[m[1]] = true
			} else if m := jobName.FindStringSubmatch(line); m != nil {
				wf.jobs[strings.Trim(m[1], `"'`)] = true
			}
		}
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}
	return wf
}

func requiredJobs(t *testing.T, root string) []string {
	t.Helper()
	out, err := exec.Command("bash", filepath.Join(root, "scripts", "feature-branch.sh"), "ruleset", "example-checkpoint-1").Output()
	if err != nil {
		t.Fatalf("feature-branch.sh ruleset: %v", err)
	}
	var jobs []string
	for _, m := range regexp.MustCompile(`"context": "([^"]+)"`).FindAllStringSubmatch(string(out), -1) {
		jobs = append(jobs, strings.SplitN(m[1], " / ", 2)[0])
	}
	if len(jobs) == 0 {
		t.Fatalf("no required checks in the ruleset:\n%s", out)
	}
	return jobs
}

func pathFilteredRequiredWorkflows(t *testing.T, dir string, required []string) (offenders []string, found map[string]bool) {
	t.Helper()
	files, err := filepath.Glob(filepath.Join(dir, "*.yml"))
	if err != nil {
		t.Fatal(err)
	}
	found = map[string]bool{}
	for _, file := range files {
		wf := readWorkflow(t, file)
		for _, job := range required {
			if !wf.jobs[job] {
				continue
			}
			found[job] = true
			if len(wf.triggerPaths) > 0 {
				offenders = append(offenders, filepath.Base(file)+": "+strings.Join(wf.triggerPaths, ", "))
			}
		}
	}
	return offenders, found
}

func repoRoot(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	return filepath.Dir(filepath.Dir(wd))
}

func TestRequiredWorkflows_neverFilterPathsAtTheWorkflowLevel(t *testing.T) {
	root := repoRoot(t)
	required := requiredJobs(t, root)
	offenders, found := pathFilteredRequiredWorkflows(t, filepath.Join(root, ".github", "workflows"), required)
	for _, job := range required {
		if !found[job] {
			t.Fatalf("no workflow defines the required job %q", job)
		}
	}
	if len(offenders) > 0 {
		t.Fatalf("a workflow skipped by paths never reports its required check, so the queue waits out its timeout; filter inside jobs instead:\n%s", strings.Join(offenders, "\n"))
	}
}

func TestRequiredWorkflows_catchAPlantedPathsFilter(t *testing.T) {
	dir := t.TempDir()
	planted := "name: CI\non:\n  pull_request:\n    paths-ignore: ['docs/**']\n  merge_group:\njobs:\n  ci:\n    uses: ./.github/workflows/ci-jobs.yml\n"
	jobLevel := "name: Other\non:\n  pull_request:\njobs:\n  filter:\n    runs-on: ubuntu-latest\n    steps:\n      - uses: dorny/paths-filter@v3\n        with:\n          filters: |\n            paths:\n              - 'x/**'\n"
	for name, body := range map[string]string{"ci.yml": planted, "other.yml": jobLevel} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	offenders, _ := pathFilteredRequiredWorkflows(t, dir, []string{"ci", "filter"})
	if len(offenders) != 1 || !strings.HasPrefix(offenders[0], "ci.yml: paths-ignore") {
		t.Fatalf("offenders %v, want only the planted workflow-level paths-ignore in ci.yml", offenders)
	}
}

var (
	inlineExpr = regexp.MustCompile(`\$\{\{(.*?)\}\}`)
	ifKey      = regexp.MustCompile(`^(\s*)(?:- )?if:\s*(.*)$`)
	literal    = regexp.MustCompile(`^'[^']*'$`)
)

func expressions(text string) []string {
	var out []string
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		for _, m := range inlineExpr.FindAllStringSubmatch(line, -1) {
			out = append(out, m[1])
		}
		m := ifKey.FindStringSubmatch(line)
		if m == nil || strings.Contains(line, "${{") {
			continue
		}
		expr := m[2]
		if expr == ">-" || expr == "|" || expr == ">" {
			expr = ""
			for _, next := range lines[i+1:] {
				if strings.TrimSpace(next) == "" || len(next)-len(strings.TrimLeft(next, " ")) <= len(m[1]) {
					break
				}
				expr += " " + strings.TrimSpace(next)
			}
		}
		out = append(out, expr)
	}
	return out
}

func gatedOnSchedule(term string) bool {
	return strings.Contains(term, "github.event_name == 'schedule' &&")
}

func fallbackOffenders(text string) []string {
	var bad []string
	for _, expr := range expressions(text) {
		terms := strings.Split(expr, "||")
		at := slices.IndexFunc(terms, func(term string) bool { return strings.Contains(term, "vars.FEATURE_BRANCH") })
		if at < 0 || gatedOnSchedule(terms[at]) {
			continue
		}
		last := at > 0
		for _, rest := range terms[at+1:] {
			last = last && literal.MatchString(strings.Trim(rest, " ()"))
		}
		if !last {
			bad = append(bad, strings.TrimSpace(expr))
		}
	}
	return bad
}

func TestWorkflows_readTheFeatureBranchVariableOnlyAsTheFinalFallback(t *testing.T) {
	root := repoRoot(t)
	files, err := filepath.Glob(filepath.Join(root, ".github", "workflows", "*.yml"))
	if err != nil {
		t.Fatal(err)
	}
	actions, err := filepath.Glob(filepath.Join(root, ".github", "actions", "*", "action.yml"))
	if err != nil {
		t.Fatal(err)
	}
	reads := 0
	for _, file := range append(files, actions...) {
		b, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		reads += strings.Count(string(b), "vars.FEATURE_BRANCH")
		for _, expr := range fallbackOffenders(string(b)) {
			t.Errorf("%s reads vars.FEATURE_BRANCH before the event's own branch: %s", filepath.Base(file), expr)
		}
	}
	if reads == 0 {
		t.Fatal("no workflow reads vars.FEATURE_BRANCH; drop this test with the variable")
	}
}

func TestWorkflows_fallbackCheckCatchesTheVariableAsTheSoleGate(t *testing.T) {
	for text, want := range map[string]int{
		"    if: github.ref_name == vars.FEATURE_BRANCH\n":                                                                 1,
		"      base: ${{ vars.FEATURE_BRANCH || github.event.pull_request.base.ref }}\n":                                   1,
		"    if: >-\n      github.base_ref == vars.FEATURE_BRANCH ||\n      github.base_ref == 'main'\n":                   1,
		"      base: ${{ github.event.pull_request.base.ref || vars.FEATURE_BRANCH || 'main' }}\n":                         0,
		"    if: >-\n      !cancelled() && (github.ref_name == 'main' ||\n      github.ref_name == vars.FEATURE_BRANCH)\n": 0,
		"          ref: ${{ github.event_name == 'schedule' && vars.FEATURE_BRANCH || '' }}\n":                             0,
	} {
		if got := fallbackOffenders(text); len(got) != want {
			t.Errorf("%q: offenders %q, want %d", text, got, want)
		}
	}
}
