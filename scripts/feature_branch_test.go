package scripts_test

import (
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

type ruleset struct {
	Name         string `json:"name"`
	Enforcement  string `json:"enforcement"`
	BypassActors []struct {
		ActorID    int    `json:"actor_id"`
		ActorType  string `json:"actor_type"`
		BypassMode string `json:"bypass_mode"`
	} `json:"bypass_actors"`
	Conditions struct {
		RefName struct {
			Include []string `json:"include"`
		} `json:"ref_name"`
	} `json:"conditions"`
	Rules []struct {
		Type       string         `json:"type"`
		Parameters map[string]any `json:"parameters"`
	} `json:"rules"`
}

func (rs ruleset) rule(t *testing.T, typ string) map[string]any {
	t.Helper()
	for _, r := range rs.Rules {
		if r.Type == typ {
			return r.Parameters
		}
	}
	t.Fatalf("ruleset %q has no %q rule", rs.Name, typ)
	return nil
}

func (rs ruleset) requiredChecks(t *testing.T) map[string]float64 {
	t.Helper()
	got := map[string]float64{}
	for _, c := range rs.rule(t, "required_status_checks")["required_status_checks"].([]any) {
		check := c.(map[string]any)
		got[check["context"].(string)] = check["integration_id"].(float64)
	}
	return got
}

func printRuleset(t *testing.T, args ...string) ruleset {
	t.Helper()
	out, err := exec.Command("bash", append([]string{filepath.Join(repoRoot(t), "scripts", "feature-branch.sh")}, args...)...).Output()
	if err != nil {
		t.Fatalf("feature-branch.sh %v: %v", args, err)
	}
	var rs ruleset
	if err := json.Unmarshal(out, &rs); err != nil {
		t.Fatalf("ruleset is not JSON: %v\n%s", err, out)
	}
	if rs.Enforcement != "active" {
		t.Fatalf("ruleset %q enforcement %q, want active", rs.Name, rs.Enforcement)
	}
	for _, typ := range []string{"deletion", "non_fast_forward"} {
		rs.rule(t, typ)
	}
	return rs
}

func TestFeatureBranchRuleset_landsEveryPRThroughTheMergeQueue(t *testing.T) {
	rs := printRuleset(t, "ruleset", "following-checkpoint-12")

	if want := []string{"refs/heads/following-checkpoint-12"}; !reflect.DeepEqual(rs.Conditions.RefName.Include, want) {
		t.Fatalf("targets %v, want %v, because a merge_queue rule takes exact ref names only", rs.Conditions.RefName.Include, want)
	}
	if len(rs.BypassActors) != 1 || rs.BypassActors[0].ActorID != 1 || rs.BypassActors[0].ActorType != "OrganizationAdmin" || rs.BypassActors[0].BypassMode != "always" {
		t.Fatalf("bypass actors %+v, want only org admins, whose token cuts the next feature branch at a checkpoint", rs.BypassActors)
	}
	if got := rs.rule(t, "pull_request")["allowed_merge_methods"]; !reflect.DeepEqual(got, []any{"merge"}) {
		t.Fatalf("merge methods %v, want merge only, so a stack's lower PRs show merged", got)
	}
	want := map[string]any{
		"merge_method":                      "MERGE",
		"grouping_strategy":                 "ALLGREEN",
		"max_entries_to_build":              float64(5),
		"min_entries_to_merge":              float64(1),
		"max_entries_to_merge":              float64(5),
		"min_entries_to_merge_wait_minutes": float64(0),
		"check_response_timeout_minutes":    float64(30),
	}
	if got := rs.rule(t, "merge_queue"); !reflect.DeepEqual(got, want) {
		t.Fatalf("merge queue %v, want %v", got, want)
	}
	if rs.rule(t, "required_status_checks")["strict_required_status_checks_policy"] != false {
		t.Fatal("the queue tests each PR against the tip, so up to date must stay off")
	}
	if got, want := rs.requiredChecks(t), map[string]float64{"ci / ci-ok": 15368, "PR format (title, body and commits)": 15368}; !reflect.DeepEqual(got, want) {
		t.Fatalf("required checks %v, want %v", got, want)
	}
}

func TestMainRuleset_staysSquashOnlyWithNoQueue(t *testing.T) {
	rs := printRuleset(t, "main-ruleset")

	if rs.Name != "main" || !reflect.DeepEqual(rs.Conditions.RefName.Include, []string{"~DEFAULT_BRANCH"}) {
		t.Fatalf("ruleset %q targets %v, want main on ~DEFAULT_BRANCH", rs.Name, rs.Conditions.RefName.Include)
	}
	if len(rs.BypassActors) != 0 {
		t.Fatalf("bypass actors %+v, want none", rs.BypassActors)
	}
	if got := rs.rule(t, "pull_request")["allowed_merge_methods"]; !reflect.DeepEqual(got, []any{"squash"}) {
		t.Fatalf("merge methods %v, want squash only, so main gets one commit per checkpoint", got)
	}
	for _, r := range rs.Rules {
		if r.Type == "merge_queue" {
			t.Fatal("main keeps manual merges and has no queue")
		}
	}
	if rs.rule(t, "required_status_checks")["strict_required_status_checks_policy"] != true {
		t.Fatal("main requires the checkpoint PR to be up to date")
	}
	if got, want := rs.requiredChecks(t), map[string]float64{"ci / ci-ok": 15368, "Changelog (checkpoint into main)": 15368}; !reflect.DeepEqual(got, want) {
		t.Fatalf("required checks %v, want %v", got, want)
	}
}

func TestFeatureBranch_rejectsAMissingNameOrOneOffTheConvention(t *testing.T) {
	for _, args := range [][]string{
		{"apply"}, {"init", ""}, {"ruleset"}, {"main-ruleset", "x-1"},
		{"apply", "leaderboards-checkpoint"}, {"apply", "982-workflow-docs"}, {"init", "main"}, {"ruleset", "Leaderboards-Checkpoint-2"},
	} {
		err := exec.Command("bash", append([]string{filepath.Join(repoRoot(t), "scripts", "feature-branch.sh")}, args...)...).Run()
		var exit *exec.ExitError
		if !errors.As(err, &exit) || exit.ExitCode() != 2 {
			t.Errorf("%v: %v, want a usage error", args, err)
		}
	}
}

// The stub answers "gh api repos/o/r/rulesets[/<id>] --jq <filter>" from <id>.json in $STUB_DIR and logs
// every call, plus the name and include list of each ruleset body it receives.
const applyStub = `#!/usr/bin/env bash
echo "$(basename "$0") $*" >>"$STUB_DIR/log"
if [[ "$1" == api && "$2" == repos/o/r/rulesets* ]]; then
  id="${2#repos/o/r/rulesets}"
  jq -c "${@: -1}" "$STUB_DIR/${id#/}.json"
fi
if [[ "$*" == *"--input -"* ]]; then
  jq -c '{name, include: .conditions.ref_name.include}' >>"$STUB_DIR/log"
  echo 1
fi
`

func TestFeatureBranchApply_addsTheExactBranchToTheOneRuleset(t *testing.T) {
	const (
		perBranch = `{"id": 24089171, "name": "feature branch leaderboards-checkpoint-1", "conditions": {"ref_name": {"include": ["refs/heads/leaderboards-checkpoint-1"]}}}`
		renamed   = `{"id": 24089171, "name": "feature branches", "conditions": {"ref_name": {"include": ["refs/heads/leaderboards-checkpoint-1"]}}}`
		others    = `{"id": 5, "name": "main"}, {"id": 6, "name": "maintenance"}`
	)
	for _, tc := range []struct {
		name, branch, existing, want string
	}{
		{
			"renames the per-branch ruleset in place", "leaderboards-checkpoint-1", perBranch,
			"PUT repos/o/r/rulesets/24089171 --input - --jq .id\n" +
				`{"name":"feature branches","include":["refs/heads/leaderboards-checkpoint-1"]}`,
		},
		{
			"adds the next branch and keeps the old refs", "leaderboards-checkpoint-2", renamed,
			"PUT repos/o/r/rulesets/24089171 --input - --jq .id\n" +
				`{"name":"feature branches","include":["refs/heads/leaderboards-checkpoint-1","refs/heads/leaderboards-checkpoint-2"]}`,
		},
		{
			"adds a new feature", "following-checkpoint-1", renamed,
			"PUT repos/o/r/rulesets/24089171 --input - --jq .id\n" +
				`{"name":"feature branches","include":["refs/heads/leaderboards-checkpoint-1","refs/heads/following-checkpoint-1"]}`,
		},
		{
			"creates the ruleset when there is none", "following-checkpoint-1", "",
			"POST repos/o/r/rulesets --input - --jq .id\n" + `{"name":"feature branches","include":["refs/heads/following-checkpoint-1"]}`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			bin, dir := t.TempDir(), t.TempDir()
			for _, tool := range []string{"gh", "gt"} {
				if err := os.WriteFile(filepath.Join(bin, tool), []byte(applyStub), 0o755); err != nil {
					t.Fatal(err)
				}
			}
			list := others
			if tc.existing != "" {
				list = tc.existing + ", " + others
				if err := os.WriteFile(filepath.Join(dir, "24089171.json"), []byte(tc.existing), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			if err := os.WriteFile(filepath.Join(dir, ".json"), []byte("["+list+"]"), 0o644); err != nil {
				t.Fatal(err)
			}
			cmd := exec.Command("bash", filepath.Join(repoRoot(t), "scripts", "feature-branch.sh"), "apply", tc.branch)
			cmd.Env = append(os.Environ(), "PATH="+bin+":"+os.Getenv("PATH"), "MONACO_REPO=o/r", "STUB_DIR="+dir)
			if out, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("apply: %v\n%s", err, out)
			}
			b, err := os.ReadFile(filepath.Join(dir, "log"))
			if err != nil {
				t.Fatal(err)
			}
			got := string(b)
			for _, want := range []string{
				"gt trunk --add " + tc.branch + " --no-interactive",
				"gh variable set FEATURE_BRANCH --repo o/r --body " + tc.branch,
				tc.want,
				"PUT repos/o/r/rulesets/5 --input - --jq .id\n" + `{"name":"main","include":["~DEFAULT_BRANCH"]}`,
			} {
				if !strings.Contains(got, want) {
					t.Errorf("apply did not run %q:\n%s", want, got)
				}
			}
			if strings.Count(got, "--input -") != 2 || strings.Contains(got, "rulesets/6") {
				t.Errorf("apply wrote an extra ruleset or touched an unrelated one:\n%s", got)
			}
		})
	}
}

const featureBranchPattern = `^[a-z0-9]+(-[a-z0-9]+)*-[0-9]+$`

func TestFeatureBranchName_parsesOnlyNameDashNumber(t *testing.T) {
	script := filepath.Join(repoRoot(t), "scripts", "ci", "feature-branch-name.sh")
	for ref, want := range map[string]string{
		"leaderboards-checkpoint-1":      "leaderboards 1\n",
		"following-checkpoint-9":         "following 9\n",
		"following-checkpoint-10":        "following 10\n",
		"backend-rewrite-3":              "backend-rewrite 3\n",
		"leaderboards-checkpoint":        "",
		"982-workflow-docs":              "",
		"main":                           "",
		"Leaderboards-Checkpoint-2":      "",
		"leaderboards-checkpoint-1\nx-1": "",
		"graphite-base/1015":             "",
	} {
		out, err := exec.Command("bash", script, ref).Output()
		var exit *exec.ExitError
		switch {
		case want != "" && (err != nil || string(out) != want):
			t.Errorf("%q: %q %v, want %q", ref, out, err, want)
		case want == "" && (!errors.As(err, &exit) || exit.ExitCode() != 1 || len(out) != 0):
			t.Errorf("%q: %q %v, want exit 1 and no output", ref, out, err)
		}
	}
}

func TestFeatureBranchPattern_isTheSameInEveryChecker(t *testing.T) {
	for _, file := range []string{"ci/feature-branch-name.sh", "agent-guard.py", "check-pr-format.py"} {
		b, err := os.ReadFile(filepath.Join(repoRoot(t), "scripts", file))
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(b), featureBranchPattern) {
			t.Errorf("scripts/%s does not hold the feature branch pattern %s", file, featureBranchPattern)
		}
	}
}
