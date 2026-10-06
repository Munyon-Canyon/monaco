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

func verifyGatePhrase(line string) string {
	line = strings.ToLower(line)
	for _, phrase := range verifyGatePhrases {
		if strings.Contains(line, phrase) {
			return phrase
		}
	}
	return ""
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
		if got := verifyGatePhrase(line); got != want {
			t.Errorf("verifyGatePhrase(%q) = %q, want %q", line, got, want)
		}
	}

	root := repoRoot(t)
	out, err := exec.Command("git", "-C", root, "ls-files", "-z", "--", "docs", ".claude").Output()
	if err != nil {
		t.Fatalf("git ls-files: %v", err)
	}
	files := strings.Split(strings.TrimSuffix(string(out), "\x00"), "\x00")
	for _, name := range append(files, "apps/backend/AGENTS.md", "scripts/agent-guard.py") {
		for i, line := range strings.Split(readRepo(t, root, name), "\n") {
			if phrase := verifyGatePhrase(line); phrase != "" {
				t.Errorf("%s:%d: %q makes landing wait on verify, but land-stack and agents watch land on ci / ci-ok and mergeability only", name, i+1, phrase)
			}
		}
	}
}
