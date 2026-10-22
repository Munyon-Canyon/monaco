package main

import (
	"fmt"
	"io/fs"
	"os"
	"path"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/monaco/monaco/apps/backend/internal/errs"
)

const (
	agentsOp     = "gen.agents"
	rulesFile    = "docs/agents/rules.yaml"
	enforcedFile = "docs/agents/enforced.txt"
	enforcedHead = "# Rules a hook or CI check already enforces. Briefs must not restate these lines.\n"
)

type surfaceSpec struct {
	file      string
	audiences []string
}

func fixedSurface(surface string) (surfaceSpec, bool) {
	switch surface {
	case "agents-md":
		return surfaceSpec{file: "AGENTS.md"}, true
	case "monaco-milestone":
		return surfaceSpec{file: ".claude/skills/monaco-milestone/SKILL.md"}, true
	case "backend-agents-md":
		return surfaceSpec{file: "apps/backend/AGENTS.md", audiences: []string{"all"}}, true
	case "mobile-agents-md":
		return surfaceSpec{file: "apps/mobile/AGENTS.md", audiences: []string{"all"}}, true
	case "standing-orders":
		return surfaceSpec{file: "docs/agents/standing-orders.md", audiences: []string{"all", "root", "owner"}}, true
	case "owner-brief":
		return surfaceSpec{file: "docs/agents/owner.md", audiences: []string{"all", "owner"}}, true
	case "verifier-brief":
		return surfaceSpec{file: "docs/agents/verifier.md", audiences: []string{"all", "verifier"}}, true
	}
	return surfaceSpec{}, false
}

const cursorPrefix = "cursor-rules/"

func surfaceFor(surface string) (surfaceSpec, bool) {
	if spec, ok := fixedSurface(surface); ok {
		return spec, true
	}
	name, ok := strings.CutPrefix(surface, cursorPrefix)
	if !ok || name == "" || strings.ContainsAny(name, "/.") {
		return surfaceSpec{}, false
	}
	return surfaceSpec{file: ".cursor/rules/" + name + ".mdc", audiences: []string{"all"}}, true
}

func surfaceNames(rules []rule) []string {
	names := []string{"agents-md", "monaco-milestone"}
	for _, r := range rules {
		for _, s := range r.Surface {
			if !slices.Contains(names, s) {
				names = append(names, s)
			}
		}
	}
	slices.Sort(names)
	return names
}

type rule struct {
	ID         string   `yaml:"id"`
	Title      string   `yaml:"title"`
	Text       string   `yaml:"text"`
	Audience   string   `yaml:"audience"`
	Surface    []string `yaml:"surface"`
	EnforcedBy string   `yaml:"enforced_by"`
	Covers     []string `yaml:"covers"`
	Phrase     string   `yaml:"phrase"`
	Why        string   `yaml:"why"`
	Incident   string   `yaml:"incident"`
	Note       string   `yaml:"note"`
}

type ruleSet struct {
	LegacyPhrases []string `yaml:"legacy_phrases"`
	Rules         []rule   `yaml:"rules"`
}

func (r rule) prose() bool { return r.EnforcedBy == "prose" }

func invalid(op, reason string) error {
	return errs.New(errs.CodeInvalidInput, op+": "+reason)
}

func loadRules(repo fs.FS) (ruleSet, error) {
	raw, err := fs.ReadFile(repo, rulesFile)
	if err != nil {
		return ruleSet{}, errs.Wrap(err, errs.CodeInvalidInput, agentsOp)
	}
	var set ruleSet
	dec := yaml.NewDecoder(strings.NewReader(string(raw)))
	dec.KnownFields(true)
	if err := dec.Decode(&set); err != nil {
		return ruleSet{}, errs.Wrap(err, errs.CodeInvalidInput, agentsOp)
	}
	return set, validateRules(repo, set.Rules)
}

func validateRules(repo fs.FS, rules []rule) error {
	seen := map[string]bool{}
	for _, r := range rules {
		if reason := ruleProblem(repo, r, seen[r.ID]); reason != "" {
			return invalid(agentsOp, fmt.Sprintf("rule %q: %s", r.ID, reason))
		}
		seen[r.ID] = true
	}
	return nil
}

func ruleProblem(repo fs.FS, r rule, duplicate bool) string {
	switch {
	case r.ID == "":
		return "id is empty"
	case duplicate:
		return "id is duplicated"
	case r.Text == "" || r.Title == "":
		return "title and text are required"
	case !slices.Contains([]string{"all", "root", "owner", "verifier"}, r.Audience):
		return "audience must be all, root, owner or verifier"
	}
	for _, s := range r.Surface {
		spec, ok := surfaceFor(s)
		switch {
		case !ok:
			return "unknown surface " + s
		case spec.audiences != nil && !slices.Contains(spec.audiences, r.Audience):
			return fmt.Sprintf("audience %s does not render on surface %s", r.Audience, s)
		}
	}
	return enforcerProblem(repo, r.EnforcedBy)
}

func enforcerProblem(repo fs.FS, enforcedBy string) string {
	if enforcedBy == "prose" {
		return ""
	}
	kind, target, ok := strings.Cut(enforcedBy, ":")
	if !ok || target == "" {
		return fmt.Sprintf("enforced_by %q is not prose or kind:target", enforcedBy)
	}
	exists := func(rel string) bool {
		_, err := fs.Stat(repo, rel)
		return err == nil
	}
	switch kind {
	case "hook":
		return hookProblem(repo, target)
	case "ci":
		if !exists(".github/workflows/" + target + ".yml") {
			return fmt.Sprintf("no workflow .github/workflows/%s.yml", target)
		}
	case "lint", "test", "gen", "runtime":
		if !exists(target) {
			return fmt.Sprintf("%s target %q does not exist", kind, target)
		}
	default:
		return fmt.Sprintf("unknown enforcer kind %q", kind)
	}
	return ""
}

func hookProblem(repo fs.FS, target string) string {
	guard, err := fs.ReadFile(repo, "scripts/agent-guard.py")
	if err != nil {
		return "cannot read scripts/agent-guard.py"
	}
	if _, err := fs.Stat(repo, "scripts/"+target); err == nil || strings.Contains(string(guard), "def "+target+"(") {
		return ""
	}
	return fmt.Sprintf("hook %q is neither a rule in scripts/agent-guard.py nor a file in scripts/", target)
}

func renderRules(rules []rule, surface string) string {
	var b strings.Builder
	for _, r := range rules {
		switch {
		case !slices.Contains(r.Surface, surface):
		case r.prose():
			fmt.Fprintf(&b, "- %s\n", proseLine(r))
		default:
			fmt.Fprintf(&b, "- %s: enforced by `%s`.\n", r.Title, r.EnforcedBy)
		}
	}
	return b.String()
}

func proseLine(r rule) string {
	line := r.Text
	if r.Why != "" {
		line += " Why: " + r.Why
	}
	if r.Incident != "" {
		line += " Incident: " + r.Incident
	}
	return line
}

func renderEnforced(set ruleSet) string {
	phrases := slices.Clone(set.LegacyPhrases)
	for _, r := range set.Rules {
		if !r.prose() && r.Phrase != "" {
			phrases = append(phrases, r.Phrase)
		}
	}
	return enforcedHead + strings.Join(phrases, "\n") + "\n"
}

func spliceRegion(doc, surface, body string) (string, error) {
	begin := "<!-- BEGIN GENERATED rules:" + surface + " "
	end := "<!-- END GENERATED rules:" + surface + " -->"
	i := strings.Index(doc, begin)
	j := strings.Index(doc, end)
	if i < 0 || j < i {
		return "", invalid(agentsOp, fmt.Sprintf("no %q ... %q region", begin, end))
	}
	eol := strings.Index(doc[i:], "\n")
	return doc[:i+eol+1] + body + doc[j:], nil
}

func renderAgents(repo fs.FS) (map[string]string, error) {
	set, err := loadRules(repo)
	if err != nil {
		return nil, err
	}
	out := map[string]string{enforcedFile: renderEnforced(set)}
	for _, surface := range surfaceNames(set.Rules) {
		spec, _ := surfaceFor(surface)
		rel := spec.file
		doc, err := fs.ReadFile(repo, rel)
		if err != nil {
			return nil, errs.Wrap(err, errs.CodeInternal, agentsOp)
		}
		spliced, err := spliceRegion(string(doc), surface, renderRules(set.Rules, surface))
		if err != nil {
			return nil, errs.Wrap(err, errs.CodeInternal, agentsOp+": "+rel)
		}
		out[rel] = spliced
	}
	return out, nil
}

func genAgents(root string) error {
	repo := os.DirFS(root)
	files, err := renderAgents(repo)
	if err != nil {
		return err
	}
	for rel, want := range files {
		if have, err := fs.ReadFile(repo, rel); err == nil && string(have) == want {
			continue
		}
		if err := os.WriteFile(path.Join(root, rel), []byte(want), 0o600); err != nil {
			return errs.Wrap(err, errs.CodeInternal, agentsOp)
		}
	}
	return nil
}
