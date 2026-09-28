package scripts_test

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func TestBriefs_stayShortAndDoNotRestateEnforcedRules(t *testing.T) {
	t.Parallel()
	repo := repoRoot(t)
	rules := enforcedRules(t, repo)
	for _, name := range []string{"docs/agents/owner.md", "docs/agents/verifier.md"} {
		text := readRepo(t, repo, name)
		if problems := briefProblems(text, rules); len(problems) > 0 {
			t.Errorf("%s: %s", name, strings.Join(problems, "; "))
		}
	}
	planted := strings.Repeat("line\n", 61) + "do not git push\n"
	if problems := briefProblems(planted, rules); len(problems) < 2 {
		t.Fatalf("planted brief problems = %v", problems)
	}
}

func TestBriefs_dispatchPromptStaysUnder400Tokens(t *testing.T) {
	t.Parallel()
	repo := repoRoot(t)
	brief := readRepo(t, repo, "docs/agents/owner.md")
	prompt := "to: pstack:poteto-agent\n" +
		"ticket: 789\nworktree: .worktrees/789\nparent: 84e512cf\nbrief: docs/agents/owner.md\n\n" + brief
	n := (len(prompt) + 3) / 4
	t.Logf("owner dispatch prompt: %d tokens", n)
	if n >= 400 {
		t.Fatalf("dispatch prompt is %d tokens", n)
	}
}

func TestMilestoneSkill_namesRealCommandsAndStaysShort(t *testing.T) {
	t.Parallel()
	repo := repoRoot(t)
	text := readRepo(t, repo, ".claude/skills/monaco-milestone/SKILL.md")
	if problems := skillProblems(t, repo, text); len(problems) > 0 {
		t.Fatalf("%s", strings.Join(problems, "\n"))
	}
	long := strings.Repeat("x\n", 121)
	if problems := skillProblems(t, repo, long); len(problems) == 0 || !strings.Contains(problems[0], "121") {
		t.Fatalf("planted length: %v", problems)
	}
	bad := "`monacoctl agents not-a-command`\n`just not-a-recipe`\n`scripts/not-a-script.sh`\n`pstack:not-a-skill`\nmerge queue\n"
	problems := skillProblems(t, repo, bad)
	for _, want := range []string{"not-a-command", "not-a-recipe", "not-a-script.sh", "pstack:not-a-skill", "merge queue"} {
		if !strings.Contains(strings.Join(problems, "\n"), want) {
			t.Errorf("planted skill missing %q in %v", want, problems)
		}
	}
	agents, err := os.ReadDir(filepath.Join(repo, ".claude", "agents"))
	if err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	if len(agents) > 0 {
		t.Fatalf("project agent definitions: %v", agents)
	}
}

func briefProblems(text string, rules []string) []string {
	var out []string
	if n := lineCount(text); n > 60 {
		out = append(out, fmt.Sprintf("brief is %d lines", n))
	}
	low := strings.ToLower(text)
	for _, rule := range rules {
		if strings.Contains(low, strings.ToLower(rule)) {
			out = append(out, "restates "+rule)
		}
	}
	return out
}

func skillProblems(t *testing.T, repo, text string) []string {
	t.Helper()
	var out []string
	if n := lineCount(text); n > 120 {
		out = append(out, fmt.Sprintf("skill is %d lines", n))
	}
	if strings.Contains(strings.ToLower(text), "merge queue") {
		out = append(out, "skill mentions a merge queue")
	}
	cmds := agentSubcommands(t, repo)
	recipes := justRecipes(t, repo)
	documented := documentedJust(t, repo)
	for _, raw := range backticks(text) {
		if err := commandProblem(repo, raw, cmds, recipes, documented); err != nil {
			out = append(out, err.Error())
		}
	}
	if strings.Contains(text, "pstack:") {
		out = append(out, "skill names a pstack skill")
	}
	return out
}

func commandProblem(repo, raw string, cmds, recipes, documented map[string]bool) error {
	if strings.Contains(raw, "pstack:") {
		return fmt.Errorf("unknown pstack skill %q", raw)
	}
	fields := strings.Fields(raw)
	if len(fields) == 0 {
		return nil
	}
	switch fields[0] {
	case "monacoctl":
		if len(fields) < 3 || fields[1] != "agents" || !cmds[fields[2]] {
			return fmt.Errorf("unknown command %q", raw)
		}
	case "just":
		if len(fields) < 2 || (!recipes[fields[1]] && !documented[fields[1]]) {
			return fmt.Errorf("unknown just recipe %q", fields[1])
		}
	case "gh":
	default:
		if strings.Contains(fields[0], "/") {
			if _, err := os.Stat(filepath.Join(repo, fields[0])); err != nil {
				return fmt.Errorf("unknown script %q", fields[0])
			}
		}
	}
	return nil
}

func backticks(text string) []string {
	var out []string
	for _, m := range regexp.MustCompile("`([^`\n]+)`").FindAllStringSubmatch(text, -1) {
		out = append(out, m[1])
	}
	return out
}

func agentSubcommands(t *testing.T, repo string) map[string]bool {
	t.Helper()
	text := readRepo(t, repo, "apps/backend/cmd/monacoctl/agents/agents.go")
	i := strings.Index(text, "func commands()")
	if i < 0 {
		t.Fatal("commands func")
	}
	text = text[i:]
	j := strings.Index(text, "\n}\n")
	if j < 0 {
		t.Fatal("commands end")
	}
	out := map[string]bool{}
	for _, m := range regexp.MustCompile(`"([a-z0-9-]+)":`).FindAllStringSubmatch(text[:j], -1) {
		out[m[1]] = true
	}
	if len(out) == 0 {
		t.Fatal("no subcommands")
	}
	return out
}

func justRecipes(t *testing.T, repo string) map[string]bool {
	t.Helper()
	out := map[string]bool{}
	for _, line := range strings.Split(readRepo(t, repo, "Justfile"), "\n") {
		if line == "" || strings.HasPrefix(line, " ") || strings.HasPrefix(line, "\t") ||
			strings.HasPrefix(line, "#") || strings.Contains(line, ":=") {
			continue
		}
		name, _, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		fields := strings.Fields(name)
		if len(fields) == 0 {
			continue
		}
		out[fields[0]] = true
	}
	return out
}

func documentedJust(t *testing.T, repo string) map[string]bool {
	t.Helper()
	text := readRepo(t, repo, "docs/architecture/backend-platform.md")
	out := map[string]bool{}
	for _, m := range regexp.MustCompile("`just ([a-z0-9-]+)").FindAllStringSubmatch(text, -1) {
		out[m[1]] = true
	}
	if !out["verify"] {
		t.Fatal("docs no longer name just verify")
	}
	return out
}

func enforcedRules(t *testing.T, repo string) []string {
	t.Helper()
	var rules []string
	for _, line := range strings.Split(readRepo(t, repo, "docs/agents/enforced.txt"), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		rules = append(rules, line)
	}
	if len(rules) == 0 {
		t.Fatal("no enforced rules")
	}
	return rules
}

func lineCount(text string) int {
	if text == "" {
		return 0
	}
	n := strings.Count(text, "\n")
	if !strings.HasSuffix(text, "\n") {
		n++
	}
	return n
}

func readRepo(t *testing.T, repo, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(repo, name))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
