package scripts_test

import (
	"os/exec"
	"path"
	"slices"
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

var queueScopeWords = []string{"queue", "stage 2"}

var queueTestWords = []string{"race", "e2e", "full suite", "monacoctl verify", "verify-backend", "coverage"}

var queueExemptWords = []string{"stage 1", "no tests", "skips", "queue speed"}

var historyDocPaths = []string{"docs/architecture/log/", "docs/legacy/", "docs/milestones/"}

func phraseIn(phrases []string, line string) string {
	line = strings.ToLower(line)
	for _, phrase := range phrases {
		if strings.Contains(line, phrase) {
			return phrase
		}
	}
	return ""
}

func queueTestClaim(line string) string {
	if phraseIn(queueExemptWords, line) != "" {
		return ""
	}
	queue, test := phraseIn(queueScopeWords, line), phraseIn(queueTestWords, line)
	if queue == "" || test == "" {
		return ""
	}
	return queue + " + " + test
}

func isHistoryDoc(name string) bool {
	return path.Base(name) == "CHANGELOG.md" ||
		slices.ContainsFunc(historyDocPaths, func(prefix string) bool { return strings.HasPrefix(name, prefix) })
}

func walkLandingDocs(t *testing.T, visit func(name string, lineNo int, line string)) {
	t.Helper()
	root := repoRoot(t)
	out, err := exec.Command("git", "-C", root, "ls-files", "-z", "--", "docs", ".claude").Output()
	if err != nil {
		t.Fatalf("git ls-files: %v", err)
	}
	files := strings.Split(strings.TrimSuffix(string(out), "\x00"), "\x00")
	for _, name := range append(files, "apps/backend/AGENTS.md", "scripts/agent-guard.py") {
		for i, line := range strings.Split(readRepo(t, root, name), "\n") {
			visit(name, i+1, line)
		}
	}
}

func forbidInLandingDocs(t *testing.T, phrases []string, why string) {
	t.Helper()
	walkLandingDocs(t, func(name string, lineNo int, line string) {
		if phrase := phraseIn(phrases, line); phrase != "" {
			t.Errorf("%s:%d: %q %s", name, lineNo, phrase, why)
		}
	})
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

func TestLandingDocs_queueRunsNoTests(t *testing.T) {
	t.Parallel()
	for _, c := range []struct{ stale, fixed string }{
		{
			"- **The Graphite queue.** Stage 2 runs the `e2e` job in `.github/workflows/ci-jobs.yml`.",
			"- **The PR check.** Stage 1 runs the `e2e` job in `.github/workflows/ci-jobs.yml`.",
		},
		{
			"Use when the E2E job fails in the Graphite queue, when a flow moves to verified, or when reproducing an end-to-end failure.",
			"Use when the E2E job fails in stage 1, when a flow moves to verified, or when reproducing an end-to-end failure.",
		},
		{
			"1. Find the failing run. Open the `ci / E2E ...` check on the Graphite queue's draft PR, whose head branch starts with `gtmq_`, or list those runs with `gh run list --workflow ci.yml --json databaseId,headBranch,conclusion --jq '.[] | select(.headBranch | startswith(\"gtmq_\"))'`.",
			"1. Find the failing run. Open the `ci / E2E ...` check on the top PR of the stack, or list the runs of its branch with `gh run list --workflow ci.yml --branch <pr-branch> --json databaseId,headBranch,conclusion`.",
		},
		{
			"7. The race detector runs in CI. The Graphite queue runs `go test -race` over the backend through `scripts/test-backend.sh`.",
			"7. The race detector runs in CI. Stage 1's `backend` job runs `go test -race` over the backend through `scripts/test-backend.sh`.",
		},
		{
			"- Stage 0 (`go run ./cmd/monacoctl agents check`) runs the short tests without `-race`. The Graphite queue runs them with `-race`. Owners do not run `-race` locally.",
			"- Stage 0 (`go run ./cmd/monacoctl agents check`) runs the short tests without `-race`. Stage 1 runs them with `-race`. Owners do not run `-race` locally.",
		},
		{
			"| End to end | `monacoctl verify` in the Graphite queue | real binaries, see the `verify-backend` skill |",
			"| End to end | `monacoctl verify` in stage 1 | real binaries, see the `verify-backend` skill |",
		},
		{
			"CI runs the race suite and `monacoctl verify` in the queue.",
			"CI runs the race suite and `monacoctl verify` in stage 1.",
		},
		{
			"| QA | `monacoctl verify` on the real binaries; evidence is the `verify-evidence` CI artifact | `cmd/monacoctl verify` | stage 2 (Graphite merge queue) and nightly |",
			"| QA | `monacoctl verify` on the real binaries; evidence is the `verify-evidence` CI artifact | `cmd/monacoctl verify` | stage 1 and nightly |",
		},
		{
			"so `monacoctl verify all` drives it against the real binaries in stage 2.",
			"so `monacoctl verify all` drives it against the real binaries in stage 1.",
		},
		{
			"`verify-backend` runs the real `api` and `worker` binaries against a real database and bus, drives a flow the way the app would, and writes down what the system did. It runs in the Graphite merge queue (stage 2) and nightly, after the tests.",
			"`verify-backend` runs the real `api` and `worker` binaries against a real database and bus, drives a flow the way the app would, and writes down what the system did. It runs in stage 1 and nightly, after the tests.",
		},
		{
			"- **In the Graphite merge queue.** The `e2e` job runs `scripts/ci/e2e.sh` on every backend queue entry and fails on any failed invariant or budget.",
			"- **In stage 1.** The `e2e` job runs `scripts/ci/e2e.sh` behind the `backend` filter and fails on any failed invariant or budget.",
		},
		{
			"- `verify-backend`: the instructions and feature map above. The Graphite merge queue runs it on every backend entry.",
			"- `verify-backend`: the instructions and feature map above. Stage 1 runs it behind the `backend` filter.",
		},
		{
			"and the `coverage` row, which fails on an uncovered statement in any non-test Go file of a package that has a changed non-test Go file, so a change that uncovers an unchanged file of its package fails here and not in the queue.",
			"and the `coverage` row, which fails on an uncovered statement in any non-test Go file of a package that has a changed non-test Go file, so a change that uncovers an unchanged file of its package fails here and not in stage 1.",
		},
		{
			"`monacoctl verify`, every flow against the real binaries, runs only in the Graphite merge queue's `e2e` job and nightly, never as a laptop or owner step.",
			"`monacoctl verify`, every flow against the real binaries, runs only in stage 1's `e2e` job and nightly, never as a laptop or owner step.",
		},
		{
			"the `coverage` row (`monacoctl coverage --only`) fails on any uncovered statement in a non-test Go file the diff changes, as stage 2's 100% gate would.",
			"the `coverage` row (`monacoctl coverage --only`) fails on any uncovered statement in a non-test Go file the diff changes, as stage 1's 100% gate (`backend-report`) would.",
		},
		{
			"`ci-jobs.yml` · `lint`, `ready`, `vuln` (Linux); `backend` and `e2e` in the Graphite merge queue",
			"`ci-jobs.yml` · `lint`, `ready`, `vuln` (Linux); `backend` and `e2e` in stage 1",
		},
		{
			"The Graphite merge queue runs the full suite, and the nightly runs mutation.",
			"Stage 1 CI runs the full suite behind its path filters, and the nightly runs mutation.",
		},
	} {
		if queueTestClaim(c.stale) == "" {
			t.Errorf("queueTestClaim(%q) found no claim, want one", c.stale)
		}
		if got := queueTestClaim(c.fixed); got != "" {
			t.Errorf("queueTestClaim(%q) = %q, want no claim", c.fixed, got)
		}
	}
	for _, line := range []string{
		"| 2. Queue check | the PR entered the Graphite merge queue, which runs it on a `gtmq_` draft PR | `ci-ok` alone: `scripts/ci/ready.sh` (build, vet, tidy, generated files) on the combined stack. No tests, no macOS job. |",
		"Only the top PR of a stack runs stage 1, the full suite; every PR below it passes `ci / ci-ok` with every job skipped, and `monacoctl agents check` is its proof. The queue's stage 2 runs only build, vet and generated files, and every push to `staging` runs the full suite again as an advisory indicator. The only required check is `ci / ci-ok`. `gate-changes` never blocks. [What runs where](../architecture/ci.md#what-runs-where) has every job.",
		"The Graphite queue runs `ci-ok` with no tests, not even `e2e`.",
		"The queue skips the `e2e` job.",
		"| CI manager | `agents watch` and its supervisor, `monacoctl agents` bugs, CI and queue speed, flaky tests, the Xcode and coverage gates | Takes milestone tickets |",
	} {
		if got := queueTestClaim(line); got != "" {
			t.Errorf("queueTestClaim(%q) = %q, want no claim", line, got)
		}
	}

	for name, want := range map[string]bool{
		"docs/architecture/log/ci.md":     true,
		"docs/legacy/README.md":           true,
		"docs/milestones/m8.md":           true,
		"apps/backend/CHANGELOG.md":       true,
		"docs/how-to/CHANGELOG.md":        true,
		"docs/architecture/ci.md":         false,
		"docs/architecture/logging.md":    false,
		"docs/legacy-notes.md":            false,
		"docs/how-to/changelog-habits.md": false,
		".claude/skills/commit/SKILL.md":  false,
	} {
		if got := isHistoryDoc(name); got != want {
			t.Errorf("isHistoryDoc(%q) = %v, want %v", name, got, want)
		}
	}

	walkLandingDocs(t, func(name string, lineNo int, line string) {
		if claim := queueTestClaim(line); claim != "" && !isHistoryDoc(name) {
			t.Errorf("%s:%d: %q says the merge queue runs tests, but the queue runs only scripts/ci/ready.sh and stage 1 runs the tests", name, lineNo, claim)
		}
	})
}
