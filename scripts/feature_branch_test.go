package scripts_test

import (
	"encoding/json"
	"os/exec"
	"path/filepath"
	"reflect"
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
	rs := printRuleset(t, "ruleset", "backend-rewrite-9")

	if want := []string{"refs/heads/backend-rewrite-9"}; !reflect.DeepEqual(rs.Conditions.RefName.Include, want) {
		t.Fatalf("targets %v, want %v", rs.Conditions.RefName.Include, want)
	}
	if len(rs.BypassActors) != 1 || rs.BypassActors[0].ActorID != 15368 || rs.BypassActors[0].ActorType != "Integration" {
		t.Fatalf("bypass actors %+v, want only GitHub Actions, which pushes the checkpoint merge-back", rs.BypassActors)
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
	if got, want := rs.requiredChecks(t), map[string]float64{"ci / ci-ok": 15368}; !reflect.DeepEqual(got, want) {
		t.Fatalf("required checks %v, want %v", got, want)
	}
}

func TestFeatureBranch_rejectsAMissingName(t *testing.T) {
	err := exec.Command("bash", filepath.Join(repoRoot(t), "scripts", "feature-branch.sh"), "ruleset").Run()
	if err == nil {
		t.Fatal("want a usage error without a branch name")
	}
}
