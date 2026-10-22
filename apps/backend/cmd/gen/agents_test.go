package main

import (
	"io/fs"
	"os"
	"path/filepath"
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
