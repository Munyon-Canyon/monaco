package main

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRenderRulesKeepsProseAndPointsAtTheEnforcer(t *testing.T) {
	t.Parallel()
	rules := []rule{
		{ID: "P1", Title: "p", Text: "Do the thing.", Surface: []string{"agents-md"}, EnforcedBy: "prose"},
		{ID: "H1", Title: "No merge", Text: "Never merge.", Surface: []string{"agents-md"}, EnforcedBy: "hook:rule_x"},
		{ID: "O1", Title: "o", Text: "Other surface.", Surface: []string{"monaco-milestone"}, EnforcedBy: "prose"},
	}
	got := renderRules(rules, "agents-md")
	want := "- Do the thing.\n- No merge: enforced by `hook:rule_x`.\n"
	if got != want {
		t.Fatalf("renderRules =\n%q\nwant\n%q", got, want)
	}
}

func TestRenderEnforcedListsOnlyEnforcedRulePhrases(t *testing.T) {
	t.Parallel()
	set := ruleSet{
		LegacyPhrases: []string{"old phrase"},
		Rules: []rule{
			{ID: "A", Phrase: "enforced phrase", EnforcedBy: "hook:x"},
			{ID: "B", Phrase: "prose phrase", EnforcedBy: "prose"},
		},
	}
	want := enforcedHead + "old phrase\nenforced phrase\n"
	if got := renderEnforced(set); got != want {
		t.Fatalf("renderEnforced = %q, want %q", got, want)
	}
}

func TestSpliceRegionReplacesOnlyTheMarkedBody(t *testing.T) {
	t.Parallel()
	doc := "head\n<!-- BEGIN GENERATED rules:agents-md (x) -->\nstale\n<!-- END GENERATED rules:agents-md -->\ntail\n"
	got, err := spliceRegion(doc, "agents-md", "- fresh\n")
	if err != nil {
		t.Fatal(err)
	}
	want := "head\n<!-- BEGIN GENERATED rules:agents-md (x) -->\n- fresh\n<!-- END GENERATED rules:agents-md -->\ntail\n"
	if got != want {
		t.Fatalf("spliceRegion =\n%q\nwant\n%q", got, want)
	}
	if _, err := spliceRegion("no markers", "agents-md", ""); err == nil {
		t.Fatal("want an error when the markers are missing")
	}
}

func TestValidateRulesRejectsBadRules(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "scripts"), 0o750); err != nil {
		t.Fatal(err)
	}
	guard := "def rule_present(inv):\n    return None\n"
	if err := os.WriteFile(filepath.Join(dir, "scripts/agent-guard.py"), []byte(guard), 0o600); err != nil {
		t.Fatal(err)
	}
	root := os.DirFS(dir)
	ok := rule{
		ID:         "X",
		Title:      "t",
		Text:       "x",
		Audience:   "all",
		Surface:    []string{"agents-md"},
		EnforcedBy: "hook:rule_present",
	}
	with := func(edit func(*rule)) rule {
		r := ok
		edit(&r)
		return r
	}
	cases := map[string][]rule{
		"missing workflow":    {with(func(r *rule) { r.EnforcedBy = "ci:no-such-workflow" })},
		"missing hook":        {with(func(r *rule) { r.EnforcedBy = "hook:rule_absent" })},
		"missing path":        {with(func(r *rule) { r.EnforcedBy = "test:no/such/file" })},
		"unknown kind":        {with(func(r *rule) { r.EnforcedBy = "magic:x" })},
		"empty id":            {with(func(r *rule) { r.ID = "" })},
		"duplicate id":        {ok, ok},
		"unknown surface":     {with(func(r *rule) { r.Surface = []string{"nowhere"} })},
		"unknown audience":    {with(func(r *rule) { r.Audience = "everyone" })},
		"missing rule text":   {with(func(r *rule) { r.Text = "" })},
		"enforcer not a pair": {with(func(r *rule) { r.EnforcedBy = "hook" })},
	}
	for name, rules := range cases {
		if err := validateRules(root, rules); err == nil {
			t.Errorf("%s: want an error", name)
		}
	}
	if err := validateRules(os.DirFS(t.TempDir()), []rule{ok}); err == nil {
		t.Error("a hook rule without scripts/agent-guard.py: want an error")
	}
	if err := validateRules(root, []rule{ok}); err != nil {
		t.Fatalf("a valid rule failed: %v", err)
	}
}

func TestCheckedInAgentFilesMatchTheRuleSource(t *testing.T) {
	t.Parallel()
	root := "../../../.."
	want, err := renderAgents(os.DirFS(root))
	if err != nil {
		t.Fatal(err)
	}
	for rel, content := range want {
		have, err := fs.ReadFile(os.DirFS(root), rel)
		if err != nil {
			t.Fatal(err)
		}
		if string(have) != content {
			t.Errorf("%s is stale against %s; run `go generate ./...` in apps/backend", rel, rulesFile)
		}
	}
}

func agentsFixture(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	files := map[string]string{
		"docs/agents/rules.yaml": "legacy_phrases: [old]\nrules:\n" +
			"  - {id: P, title: p, text: Prose rule., audience: all, surface: [agents-md, monaco-milestone], enforced_by: prose}\n",
		"AGENTS.md": "<!-- BEGIN GENERATED rules:agents-md (x) -->\nstale\n<!-- END GENERATED rules:agents-md -->\n",
		".claude/skills/monaco-milestone/SKILL.md": "<!-- BEGIN GENERATED rules:monaco-milestone (x) -->\n<!-- END GENERATED rules:monaco-milestone -->\n",
	}
	for rel, body := range files {
		path := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func TestGenAgentsWritesEveryFileAndIsIdempotent(t *testing.T) {
	t.Parallel()
	dir := agentsFixture(t)
	for range 2 {
		if err := genAgents(dir); err != nil {
			t.Fatal(err)
		}
	}
	got, err := fs.ReadFile(os.DirFS(dir), "AGENTS.md")
	if err != nil || !strings.Contains(string(got), "- Prose rule.\n") || strings.Contains(string(got), "stale") {
		t.Fatalf("AGENTS.md = %q, %v", got, err)
	}
	enforced, err := fs.ReadFile(os.DirFS(dir), enforcedFile)
	if err != nil || string(enforced) != enforcedHead+"old\n" {
		t.Fatalf("enforced.txt = %q, %v", enforced, err)
	}
}

func TestGenAgentsFailsWhenASurfaceFileIsMissingOrUnmarkedOrUnwritable(t *testing.T) {
	t.Parallel()
	skill := ".claude/skills/monaco-milestone/SKILL.md"
	missing := agentsFixture(t)
	if err := os.Remove(filepath.Join(missing, skill)); err != nil {
		t.Fatal(err)
	}
	unmarked := agentsFixture(t)
	if err := os.WriteFile(filepath.Join(unmarked, skill), []byte("no markers"), 0o600); err != nil {
		t.Fatal(err)
	}
	unwritable := agentsFixture(t)
	if err := os.MkdirAll(filepath.Join(unwritable, enforcedFile), 0o750); err != nil {
		t.Fatal(err)
	}
	for name, dir := range map[string]string{"missing": missing, "unmarked": unmarked, "unwritable": unwritable} {
		if err := genAgents(dir); err == nil {
			t.Errorf("%s: want an error", name)
		}
	}
	badYAML := agentsFixture(t)
	if err := os.WriteFile(filepath.Join(badYAML, rulesFile), []byte("surprise: 1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := genAgents(badYAML); err == nil {
		t.Error("unknown yaml field: want an error")
	}
	if err := genAgents(t.TempDir()); err == nil {
		t.Error("no rules.yaml: want an error")
	}
}
