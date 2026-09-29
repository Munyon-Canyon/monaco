package ci_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

const nextBranchGH = `#!/usr/bin/env bash
echo "gh $*" >>"$STUB_LOG"
case "$1 $2" in
  "api repos/o/r/git/ref/"*) [[ -n "$STUB_EXISTING" ]] || exit 1; echo "$STUB_EXISTING" ;;
  "variable set") exit "${STUB_VARIABLE_EXIT:-0}" ;;
  "pr list") [[ -z "$STUB_PR_LIST_FAIL" ]] || exit 1; jq -r "${@: -1}" <<<"${STUB_PRS:-[]}" ;;
  "api repos/o/r/rulesets") jq -r "${@: -1}" <<<'[{"id": 5, "name": "main"}, {"id": 24089171, "name": "feature branches"}]' ;;
  "api repos/o/r/rulesets/24089171") [[ -n "$STUB_RULESET" ]] || exit 1; echo "$STUB_RULESET" ;;
  "api -X") jq -c . >>"$STUB_LOG"; exit "${STUB_PUT_EXIT:-0}" ;;
esac
`

const featureBranches = `{"id": 24089171, "name": "feature branches", "target": "branch", "enforcement": "active", "source": "o/r",
	"bypass_actors": [{"actor_id": 1}], "conditions": {"ref_name": {"include": ["refs/heads/leaderboards-checkpoint-1"], "exclude": []}},
	"rules": [{"type": "merge_queue"}], "_links": {}}`

const squash = "5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a"

type nextBranchRun struct {
	code             int
	out, log, report string
}

func runNextBranch(t *testing.T, headRef string, env ...string) nextBranchRun {
	t.Helper()
	root := repoRoot(t)
	bin, dir := t.TempDir(), t.TempDir()
	if err := os.WriteFile(filepath.Join(bin, "gh"), []byte(nextBranchGH), 0o755); err != nil {
		t.Fatal(err)
	}
	log, summary := filepath.Join(dir, "log"), filepath.Join(dir, "summary")
	cmd := exec.Command("bash", filepath.Join(root, "scripts", "ci", "next-branch.sh"), headRef, squash)
	cmd.Env = append(os.Environ(), "PATH="+bin+":"+os.Getenv("PATH"), "GITHUB_REPOSITORY=o/r", "GH_TOKEN=tok",
		"STUB_LOG="+log, "GITHUB_STEP_SUMMARY="+summary, "STUB_EXISTING=", "STUB_PRS=", "STUB_PR_LIST_FAIL=",
		"STUB_RULESET="+featureBranches, "STUB_PUT_EXIT=")
	cmd.Env = append(cmd.Env, env...)
	out, err := cmd.CombinedOutput()
	run := nextBranchRun{out: string(out)}
	if exit, ok := err.(*exec.ExitError); ok {
		run.code = exit.ExitCode()
	} else if err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(log)
	run.log = string(b)
	b, _ = os.ReadFile(summary)
	run.report = string(b)
	return run
}

func TestNextBranch_cutsNPlusOneAtTheSquashAndPointsTheVariableAtIt(t *testing.T) {
	for head, next := range map[string]string{
		"leaderboards-checkpoint-1": "leaderboards-checkpoint-2",
		"following-checkpoint-9":    "following-checkpoint-10",
		"following-checkpoint-09":   "following-checkpoint-10",
		"backend-rewrite-3":         "backend-rewrite-checkpoint-4",
	} {
		run := runNextBranch(t, head)
		if run.code != 0 {
			t.Fatalf("%s: exit %d\n%s", head, run.code, run.out)
		}
		for _, want := range []string{
			"gh api repos/o/r/git/refs -f ref=refs/heads/" + next + " -f sha=" + squash + " --silent",
			"gh variable set FEATURE_BRANCH --repo o/r --body " + next,
		} {
			if !strings.Contains(run.log, want) {
				t.Errorf("%s: missing %q in\n%s", head, want, run.log)
			}
		}
		if !strings.Contains(run.report, "`"+next+"`") || !strings.Contains(run.report, "gt trunk --add "+next) {
			t.Errorf("%s: summary does not name %s:\n%s", head, next, run.report)
		}
	}
}

func TestNextBranch_addsTheExactNewBranchToTheFeatureBranchRuleset(t *testing.T) {
	run := runNextBranch(t, "leaderboards-checkpoint-1")
	want := "gh api -X PUT repos/o/r/rulesets/24089171 --input - --silent\n" +
		`{"name":"feature branches","target":"branch","enforcement":"active","bypass_actors":[{"actor_id":1}],` +
		`"conditions":{"ref_name":{"include":["refs/heads/leaderboards-checkpoint-1","refs/heads/leaderboards-checkpoint-2"],"exclude":[]}},` +
		`"rules":[{"type":"merge_queue"}]}`
	if run.code != 0 || !strings.Contains(run.log, want) || strings.Count(run.log, "-X PUT") != 1 {
		t.Fatalf("exit %d, want one PUT of %s in\n%s", run.code, want, run.log)
	}
	if !strings.Contains(run.report, "Added `refs/heads/leaderboards-checkpoint-2` to the feature branch ruleset.") {
		t.Errorf("summary does not report the ruleset:\n%s", run.report)
	}
}

func TestNextBranch_leavesTheRulesetAloneWhenItHasTheBranch(t *testing.T) {
	run := runNextBranch(t, "leaderboards-checkpoint-0")
	if run.code != 0 || strings.Contains(run.log, "-X PUT") || strings.Contains(run.out, "::warning::") {
		t.Fatalf("a rerun must not write the ruleset: exit %d\n%s\n%s", run.code, run.log, run.out)
	}
	if !strings.Contains(run.report, "The feature branch ruleset already includes `refs/heads/leaderboards-checkpoint-1`.") {
		t.Errorf("summary does not report the ruleset:\n%s", run.report)
	}
}

func TestNextBranch_warnsAndCarriesOnWhenTheRulesetWriteFails(t *testing.T) {
	for name, env := range map[string]string{"PUT refused": "STUB_PUT_EXIT=1", "GET refused": "STUB_RULESET="} {
		run := runNextBranch(t, "leaderboards-checkpoint-1", env)
		want := "::warning::Could not add refs/heads/leaderboards-checkpoint-2 to the feature branch ruleset"
		if run.code != 0 || !strings.Contains(run.out, want) || !strings.Contains(run.out, "Administration: write") ||
			!strings.Contains(run.out, "scripts/feature-branch.sh apply leaderboards-checkpoint-2") {
			t.Errorf("%s: exit %d\n%s", name, run.code, run.out)
		}
		if !strings.Contains(run.log, "gh pr list") || !strings.Contains(run.report, "Run `scripts/feature-branch.sh apply leaderboards-checkpoint-2`.") {
			t.Errorf("%s: stopped after the ruleset failed:\n%s\n%s", name, run.log, run.report)
		}
	}
}

func TestNextBranch_leavesABranchAlreadyAtTheSquash(t *testing.T) {
	run := runNextBranch(t, "leaderboards-checkpoint-1", "STUB_EXISTING="+squash)
	if run.code != 0 || strings.Contains(run.log, "git/refs -f") {
		t.Fatalf("a rerun must not recreate the branch: exit %d\n%s\n%s", run.code, run.log, run.out)
	}
	if !strings.Contains(run.log, "variable set FEATURE_BRANCH") {
		t.Fatalf("a rerun still sets the variable:\n%s", run.log)
	}
}

func TestNextBranch_refusesABranchAtAnotherCommit(t *testing.T) {
	run := runNextBranch(t, "leaderboards-checkpoint-1", "STUB_EXISTING=6b6b")
	if run.code != 1 || !strings.Contains(run.out, "leaderboards-checkpoint-2 already exists at 6b6b, not at the checkpoint squash") {
		t.Fatalf("exit %d\n%s", run.code, run.out)
	}
	if strings.Contains(run.log, "git/refs -f") || strings.Contains(run.log, "variable set") {
		t.Fatalf("moved on after the conflict:\n%s", run.log)
	}
}

func TestNextBranch_warnsWhenTheTokenCannotWriteVariables(t *testing.T) {
	run := runNextBranch(t, "leaderboards-checkpoint-1", "STUB_VARIABLE_EXIT=1")
	if run.code != 0 || !strings.Contains(run.out, "::warning::Could not set the FEATURE_BRANCH repo variable to leaderboards-checkpoint-2") ||
		!strings.Contains(run.out, "Variables: write") {
		t.Fatalf("exit %d\n%s", run.code, run.out)
	}
}

func TestNextBranch_warnsAboutTicketPRsGitHubRetargetedToMain(t *testing.T) {
	run := runNextBranch(t, "leaderboards-checkpoint-1", `STUB_PRS=[
		{"number": 1001, "labels": []},
		{"number": 1002, "labels": [{"name": "integration"}]},
		{"number": 1003, "labels": [{"name": "docs"}]}]`)
	if run.code != 0 {
		t.Fatalf("exit %d\n%s", run.code, run.out)
	}
	if !strings.Contains(run.log, "gh pr list --repo o/r --base main --state open --json number,labels") {
		t.Errorf("did not list open PRs on main:\n%s", run.log)
	}
	for _, n := range []string{"1001", "1003"} {
		want := "::warning::PR #" + n + " is now based on main after the checkpoint; move it onto leaderboards-checkpoint-2 with gt track --parent leaderboards-checkpoint-2 && gt restack --upstack && gt submit"
		if !strings.Contains(run.out, want) {
			t.Errorf("missing %q in\n%s", want, run.out)
		}
		if !strings.Contains(run.report, "- #"+n) {
			t.Errorf("summary does not list #%s:\n%s", n, run.report)
		}
	}
	if strings.Contains(run.out, "PR #1002") || strings.Contains(run.report, "#1002") || strings.Count(run.out, "::warning::") != 2 {
		t.Errorf("warned about the integration-labelled checkpoint PR or warned twice:\n%s\n%s", run.out, run.report)
	}
}

func TestNextBranch_staysQuietWhenNoTicketPRIsOnMain(t *testing.T) {
	run := runNextBranch(t, "following-checkpoint-9", `STUB_PRS=[{"number": 1002, "labels": [{"name": "integration"}]}]`)
	if run.code != 0 || strings.Contains(run.out, "::warning::") || strings.Contains(run.report, "now based on main") {
		t.Fatalf("exit %d\n%s\n%s", run.code, run.out, run.report)
	}
}

func TestNextBranch_onlyWarnsWhenItCannotListPRs(t *testing.T) {
	run := runNextBranch(t, "leaderboards-checkpoint-1", "STUB_PR_LIST_FAIL=1")
	if run.code != 0 || !strings.Contains(run.out, "::warning::Could not list open PRs based on main") {
		t.Fatalf("exit %d\n%s", run.code, run.out)
	}
	if !strings.Contains(run.report, "`leaderboards-checkpoint-2`") {
		t.Errorf("summary missing after the listing failed:\n%s", run.report)
	}
}

func TestNextBranch_failsWithoutACheckpointHeadOrAToken(t *testing.T) {
	for _, head := range []string{"leaderboards-checkpoint-1a", "leaderboards-checkpoint", "982-workflow-docs", "main", "Leaderboards-Checkpoint-2"} {
		want := "::error::The checkpoint head " + head + " is not a feature branch <feature>-checkpoint-<N>"
		if run := runNextBranch(t, head); run.code != 1 || !strings.Contains(run.out, want) || run.log != "" {
			t.Errorf("%s: exit %d\n%s\n%s", head, run.code, run.out, run.log)
		}
	}
	if run := runNextBranch(t, "leaderboards-checkpoint-1", "GH_TOKEN="); run.code != 1 || !strings.Contains(run.out, "MERGE_BACK_TOKEN") {
		t.Errorf("no token: exit %d\n%s", run.code, run.out)
	}
}

func workflowJob(t *testing.T, file, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(repoRoot(t), ".github", "workflows", file))
	if err != nil {
		t.Fatal(err)
	}
	job := regexp.MustCompile(`(?ms)^  ` + name + `:\n(.*?)(?:\n  \S|\z)`).FindStringSubmatch(string(b))
	if job == nil {
		t.Fatalf("%s has no %s job", file, name)
	}
	return job[1]
}

const integrationLabel = "contains(github.event.pull_request.labels.*.name, 'integration')"

func TestCheckpointWorkflow_needsTheIntegrationLabelAndAFeatureBranchHead(t *testing.T) {
	detect := workflowJob(t, "checkpoint.yml", "detect")
	for _, want := range []string{
		"github.event.pull_request.merged &&", integrationLabel,
		"HEAD_REF: ${{ github.event.pull_request.head.ref }}",
		`if scripts/ci/feature-branch-name.sh "$HEAD_REF" >/dev/null; then`,
		`echo "checkpoint=true" >> "$GITHUB_OUTPUT"`,
	} {
		if !strings.Contains(detect, want) {
			t.Errorf("detect lacks %q:\n%s", want, detect)
		}
	}
	tree := workflowJob(t, "checkpoint.yml", "tree-matches")
	if !strings.Contains(tree, "    needs: detect\n    if: needs.detect.outputs.checkpoint == 'true'\n") {
		t.Errorf("tree-matches does not wait for a detected checkpoint:\n%s", tree)
	}
	changelog := workflowJob(t, "pr-format.yml", "changelog")
	for _, want := range []string{
		"github.event.pull_request.base.ref == 'main' &&\n      " + integrationLabel,
		`if scripts/ci/feature-branch-name.sh "$HEAD_REF" >/dev/null; then`,
		"if: steps.detect.outputs.checkpoint == 'true'\n        env:",
	} {
		if !strings.Contains(changelog, want) {
			t.Errorf("the changelog job lacks %q:\n%s", want, changelog)
		}
	}
	if b, _ := os.ReadFile(filepath.Join(repoRoot(t), ".github", "workflows", "pr-format.yml")); !strings.Contains(string(b),
		"PR_LABELS: ${{ toJson(github.event.pull_request.labels.*.name) }}\n        run: python3 scripts/check-pr-format.py") {
		t.Error("pr-format.yml does not pass the labels to check-pr-format.py")
	}
}

func TestCheckpointWorkflow_cutsTheNextBranchOnlyAfterTheTreesMatch(t *testing.T) {
	job := workflowJob(t, "checkpoint.yml", "next-branch")
	b, err := os.ReadFile(filepath.Join(repoRoot(t), ".github", "workflows", "checkpoint.yml"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"    needs: tree-matches\n",
		"GH_TOKEN: ${{ secrets.MERGE_BACK_TOKEN }}",
		"HEAD_REF: ${{ github.event.pull_request.head.ref }}",
		"SQUASH: ${{ github.event.pull_request.merge_commit_sha }}",
		`run: scripts/ci/next-branch.sh "$HEAD_REF" "$SQUASH"`,
	} {
		if !strings.Contains(job, want) {
			t.Errorf("next-branch lacks %q:\n%s", want, job)
		}
	}
	if strings.Contains(string(b), "merge-back") {
		t.Error("checkpoint.yml still merges main back into the old branch, which GitHub deletes on merge")
	}
}
