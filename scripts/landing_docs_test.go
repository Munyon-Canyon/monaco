package scripts_test

import (
	"os/exec"
	"strings"
	"testing"
)

var verifyGatePhrases = []string{
	"verify` success",
	"submitted and verified",
	"ci-ok and verify pass",
	"verifier reviews the new head, then",
}

var queueRunsTestsPhrases = []string{
	"merge queue runs the full suite",
	"stage 2 the full suite",
	"stage 2 ci job `e2e`",
}

func phraseIn(phrases []string, line string) string {
	line = strings.ToLower(line)
	for _, phrase := range phrases {
		if strings.Contains(line, phrase) {
			return phrase
		}
	}
	return ""
}

func forbidInLandingDocs(t *testing.T, phrases []string, why string) {
	t.Helper()
	root := repoRoot(t)
	out, err := exec.Command("git", "-C", root, "ls-files", "-z", "--", "docs", ".claude").Output()
	if err != nil {
		t.Fatalf("git ls-files: %v", err)
	}
	files := strings.Split(strings.TrimSuffix(string(out), "\x00"), "\x00")
	for _, name := range append(files, "apps/backend/AGENTS.md", "scripts/agent-guard.py") {
		for i, line := range strings.Split(readRepo(t, root, name), "\n") {
			if phrase := phraseIn(phrases, line); phrase != "" {
				t.Errorf("%s:%d: %q %s", name, i+1, phrase, why)
			}
		}
	}
}

func TestLandingDocs_neverWaitOnVerify(t *testing.T) {
	t.Parallel()
	for line, want := range map[string]string{
		"Run it once the `verify` success status is posted.": "verify` success",
		"The PR is Submitted and Verified.":                  "submitted and verified",
		"The queue takes it after CI-OK and verify pass.":    "ci-ok and verify pass",
		"The verifier reviews the new head, then it lands.":  "verifier reviews the new head, then",
		"PR #7 head has no verify success (latest: missing)": "",
		"The verify status is advisory.":                     "",
	} {
		if got := phraseIn(verifyGatePhrases, line); got != want {
			t.Errorf("phraseIn(verifyGatePhrases, %q) = %q, want %q", line, got, want)
		}
	}

	forbidInLandingDocs(t, verifyGatePhrases, "makes landing wait on verify, but land-stack and agents watch land on ci / ci-ok and mergeability only")
}

func TestLandingDocs_queueOnlyBuilds(t *testing.T) {
	t.Parallel()
	for line, want := range map[string]string{
		"The Graphite merge queue runs the full suite, and the nightly runs mutation.":              "merge queue runs the full suite",
		"Stage 1 and the queue's Ready job verify generated freshness, and stage 2 the full suite.": "stage 2 the full suite",
		"The stage 2 CI job `e2e` runs `scripts/ci/e2e.sh`.":                                        "stage 2 ci job `e2e`",
		"Stage 1 CI runs the full suite behind its path filters, and the nightly runs mutation.":    "",
		"The stage 1 CI job `e2e` runs `scripts/ci/e2e.sh`.":                                        "",
		"The queue's stage 2 runs `ci-ok` alone, which runs `scripts/ci/ready.sh` and no tests.":    "",
	} {
		if got := phraseIn(queueRunsTestsPhrases, line); got != want {
			t.Errorf("phraseIn(queueRunsTestsPhrases, %q) = %q, want %q", line, got, want)
		}
	}

	forbidInLandingDocs(t, queueRunsTestsPhrases, "says the merge queue runs tests, but the queue runs only scripts/ci/ready.sh and stage 1 runs the tests")
}
