package scripts_test

import (
	"encoding/json"
	"os/exec"
	"path/filepath"
	"reflect"
	"testing"
)

type featureRuleset struct {
	Name         string `json:"name"`
	Enforcement  string `json:"enforcement"`
	BypassActors []any  `json:"bypass_actors"`
	Conditions   struct {
		RefName struct {
			Include []string `json:"include"`
		} `json:"ref_name"`
	} `json:"conditions"`
	Rules []struct {
		Type       string `json:"type"`
		Parameters struct {
			AllowedMergeMethods []string `json:"allowed_merge_methods"`
			Strict              bool     `json:"strict_required_status_checks_policy"`
			Checks              []struct {
				Context       string `json:"context"`
				IntegrationID int    `json:"integration_id"`
			} `json:"required_status_checks"`
		} `json:"parameters"`
	} `json:"rules"`
}

func TestFeatureBranchRuleset_gatesTheBranchOnCIAndTheVerifierApp(t *testing.T) {
	out, err := exec.Command("bash", filepath.Join(repoRoot(t), "scripts", "feature-branch.sh"), "ruleset", "backend-rewrite-9").Output()
	if err != nil {
		t.Fatalf("feature-branch.sh ruleset: %v", err)
	}
	var rs featureRuleset
	if err := json.Unmarshal(out, &rs); err != nil {
		t.Fatalf("ruleset is not JSON: %v\n%s", err, out)
	}

	if rs.Enforcement != "active" || len(rs.BypassActors) != 0 {
		t.Fatalf("want active with no bypass actors, got %q and %v", rs.Enforcement, rs.BypassActors)
	}
	if want := []string{"refs/heads/backend-rewrite-9"}; !reflect.DeepEqual(rs.Conditions.RefName.Include, want) {
		t.Fatalf("targets %v, want %v", rs.Conditions.RefName.Include, want)
	}
	types := map[string]int{}
	for i, r := range rs.Rules {
		types[r.Type] = i
	}
	for _, want := range []string{"deletion", "non_fast_forward", "pull_request", "required_status_checks"} {
		if _, ok := types[want]; !ok {
			t.Fatalf("missing rule %q in %v", want, types)
		}
	}
	pr := rs.Rules[types["pull_request"]].Parameters
	if !reflect.DeepEqual(pr.AllowedMergeMethods, []string{"squash"}) {
		t.Fatalf("merge methods %v, want squash only", pr.AllowedMergeMethods)
	}
	checks := rs.Rules[types["required_status_checks"]].Parameters
	if !checks.Strict {
		t.Fatal("branches must be up to date before merging")
	}
	got := map[string]int{}
	for _, c := range checks.Checks {
		got[c.Context] = c.IntegrationID
	}
	if want := map[string]int{"ci / ci-ok": 15368, "verify": 5101392}; !reflect.DeepEqual(got, want) {
		t.Fatalf("required checks %v, want %v", got, want)
	}
}

func TestFeatureBranch_rejectsAMissingName(t *testing.T) {
	err := exec.Command("bash", filepath.Join(repoRoot(t), "scripts", "feature-branch.sh"), "ruleset").Run()
	if err == nil {
		t.Fatal("want a usage error without a branch name")
	}
}
