package scripts_test

import (
	"os"
	"path/filepath"
	"testing"
)

type spawnCase struct {
	name, cwd, kind, model, prompt, want string
}

func agentInput(kind, model, prompt string) map[string]any {
	in := map[string]any{"description": "d", "prompt": prompt}
	if kind != "" {
		in["subagent_type"] = kind
	}
	if model != "" {
		in["model"] = model
	}
	return in
}

func dispatchRepo(t *testing.T) (repo, linked string) {
	t.Helper()
	repo = t.TempDir()
	git(t, repo, "init", "-q")
	git(t, repo, "-c", "user.email=a@example.com", "-c", "user.name=a", "-c", "commit.gpgsign=false",
		"commit", "-q", "--allow-empty", "-m", "chore: root")
	linked = filepath.Join(t.TempDir(), "lane")
	git(t, repo, "worktree", "add", "-q", "--detach", linked)
	records := filepath.Join(repo, ".git", ".monaco", "agents")
	if err := os.MkdirAll(records, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(records, "12.json"), []byte(`{"ticket":12}`), 0o600); err != nil {
		t.Fatal(err)
	}
	return repo, linked
}

func TestAgentGuardDispatch_ownersAndVerifiersRunAsPotetoAgentsOnAnAllowedModel(t *testing.T) {
	t.Parallel()
	repo, linked := dispatchRepo(t)
	outside := t.TempDir()
	owner := "ticket: 12\nworktree: /w/.worktrees/12\nparent: 84e512cf\nbrief: docs/agents/owner.md\n"
	verifier := "pr: 40\nticket: 12\nbrief: /Users/x/monaco/docs/agents/verifier.md\n"
	poteto := "pstack:poteto-agent"
	notPoteto := "pstack:poteto-agent, not general-purpose"
	cases := []spawnCase{
		{"owner as general-purpose", repo, "general-purpose", "opus", owner, notPoteto},
		{"owner with no type", repo, "", "opus", owner, notPoteto},
		{"verifier as general-purpose", repo, "general-purpose", "sonnet", verifier, notPoteto},
		{"owner on fable", repo, poteto, "fable", owner, "one of opus, sonnet; got fable"},
		{"owner inheriting the root model", repo, poteto, "", owner, "got none (inherits yours)"},
		{"verifier on haiku", repo, poteto, "haiku", verifier, "got haiku"},
		{"watch fresh owner as general-purpose", repo, "general-purpose", "opus",
			"#40 ejected\n  fresh owner\n  ticket: 12\n  brief: docs/agents/owner.md\n", notPoteto},
		{"owner before dispatch", repo, poteto, "opus", "ticket: 13\nbrief: docs/agents/owner.md\n",
			"#13 has no dispatch record"},
		{"owner outside a repo", outside, poteto, "opus", owner, "could not find the git common dir"},
		{"capitalised brief", repo, "general-purpose", "opus", "Brief: docs/agents/owner.md\n", notPoteto},
		{"bulleted brief", repo, "general-purpose", "opus", "- brief: docs/agents/owner.md\n", notPoteto},
		{"bold brief", repo, "general-purpose", "opus", "**brief:** docs/agents/owner.md\n", notPoteto},
		{"backticked brief", repo, "general-purpose", "opus", "brief: `docs/agents/verifier.md`\n", notPoteto},
		{"spaced brief", repo, "general-purpose", "opus", "  * BRIEF : /abs/docs/agents/owner.md\n", notPoteto},
		{"dispatched owner", repo, poteto, "opus", owner, ""},
		{"dispatched owner from a linked worktree", linked, poteto, "opus", owner, ""},
		{"verifier", repo, poteto, "sonnet", verifier, ""},
		{"fresh owner for a PR with no ticket", repo, poteto, "opus",
			"#41 ejected\n  fresh owner\n  ticket: unknown\n  brief: docs/agents/owner.md\n", ""},
		{"swarm worker", repo, "general-purpose", "", "Audit scripts/ for dead code.", ""},
		{"how explorer", repo, "Explore", "haiku", "Map how docs/agents/owner.md is loaded by dispatch.", ""},
		{"poteto helper on the root model", repo, poteto, "", "Fix the flaky test in scripts/.", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			r := runHook(t, "agent-guard-dispatch.py", map[string]any{
				"hook_event_name": "PreToolUse",
				"tool_name":       "Agent",
				"cwd":             c.cwd,
				"tool_input":      agentInput(c.kind, c.model, c.prompt),
			}, "PYENV_VERSION=system")
			if c.want == "" {
				assertAllowed(t, r, c.name)
			} else {
				assertBlocked(t, r, c.name, c.want)
			}
		})
	}
	t.Run("a Bash call", func(t *testing.T) {
		t.Parallel()
		r := runHook(t, "agent-guard-dispatch.py", map[string]any{
			"hook_event_name": "PreToolUse",
			"tool_name":       "Bash",
			"cwd":             repo,
			"tool_input":      map[string]any{"command": "echo " + owner},
		}, "PYENV_VERSION=system")
		assertAllowed(t, r, "a Bash call")
	})
}
