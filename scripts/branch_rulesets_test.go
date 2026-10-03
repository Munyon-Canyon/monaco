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

func (rs ruleset) noMergeQueue(t *testing.T, why string) {
	t.Helper()
	for _, r := range rs.Rules {
		if r.Type == "merge_queue" {
			t.Fatalf("ruleset %q has a GitHub merge_queue rule: %s", rs.Name, why)
		}
	}
}

func printRuleset(t *testing.T, arg string) ruleset {
	t.Helper()
	out, err := exec.Command("bash", filepath.Join(repoRoot(t), "scripts", "branch-rulesets.sh"), arg).Output()
	if err != nil {
		t.Fatalf("branch-rulesets.sh %s: %v", arg, err)
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

func TestStagingRuleset_squashesAndLeavesTheQueueToGraphite(t *testing.T) {
	rs := printRuleset(t, "staging-ruleset")

	if rs.Name != "staging" || !reflect.DeepEqual(rs.Conditions.RefName.Include, []string{"refs/heads/staging"}) {
		t.Fatalf("ruleset %q targets %v, want staging on refs/heads/staging", rs.Name, rs.Conditions.RefName.Include)
	}
	bypass := map[string]int{}
	for _, a := range rs.BypassActors {
		if a.BypassMode != "always" {
			t.Fatalf("bypass actor %+v, want bypass_mode always", a)
		}
		bypass[a.ActorType] = a.ActorID
	}
	if want := map[string]int{"OrganizationAdmin": 1, "Integration": 158384}; !reflect.DeepEqual(bypass, want) {
		t.Fatalf("bypass actors %v, want org admins and the Graphite App, whose queue optimizations push to staging", bypass)
	}
	if got := rs.rule(t, "pull_request")["allowed_merge_methods"]; !reflect.DeepEqual(got, []any{"squash"}) {
		t.Fatalf("merge methods %v, want squash only", got)
	}
	rs.noMergeQueue(t, "the Graphite merge queue lands PRs, and it cannot merge into a branch with a GitHub queue")
	if rs.rule(t, "required_status_checks")["strict_required_status_checks_policy"] != false {
		t.Fatal("the queue tests each PR against the tip, so up to date must stay off")
	}
	if got, want := rs.requiredChecks(t), map[string]float64{"ci / ci-ok": 15368}; !reflect.DeepEqual(got, want) {
		t.Fatalf("required checks %v, want %v", got, want)
	}
}

func TestMainRuleset_takesAMergeCommitWithNoQueue(t *testing.T) {
	rs := printRuleset(t, "main-ruleset")

	if rs.Name != "main" || !reflect.DeepEqual(rs.Conditions.RefName.Include, []string{"refs/heads/main"}) {
		t.Fatalf("ruleset %q targets %v, want main on refs/heads/main, since staging is the default branch", rs.Name, rs.Conditions.RefName.Include)
	}
	if len(rs.BypassActors) != 0 {
		t.Fatalf("bypass actors %+v, want none", rs.BypassActors)
	}
	if got := rs.rule(t, "pull_request")["allowed_merge_methods"]; !reflect.DeepEqual(got, []any{"merge"}) {
		t.Fatalf("merge methods %v, want merge only, so staging and main histories stay linked", got)
	}
	rs.noMergeQueue(t, "main keeps manual merges")
	if rs.rule(t, "required_status_checks")["strict_required_status_checks_policy"] != true {
		t.Fatal("main requires the promotion PR to be up to date")
	}
	if got, want := rs.requiredChecks(t), map[string]float64{"ci / ci-ok": 15368, "Changelog (checkpoint into main)": 15368}; !reflect.DeepEqual(got, want) {
		t.Fatalf("required checks %v, want %v", got, want)
	}
}
