package ci_test

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const fakeStatusGH = `#!/usr/bin/env bash
set -euo pipefail
endpoint="$2"
filter=""
while [[ $# -gt 0 ]]; do
  if [[ "$1" == --jq ]]; then filter="$2"; fi
  shift
done
if [[ -f "$FAKE_GH/fail" ]]; then
  echo "gh: API rate limit exceeded (HTTP 403)" >&2
  exit 1
fi
case "$endpoint" in
  */commits/*/statuses) jq -r "$filter" "$FAKE_GH/statuses.json" ;;
  *) echo "fake gh: unexpected $endpoint" >&2; exit 1 ;;
esac
`

func allFilters(value string) []string {
	var env []string
	for _, k := range []string{"IOS", "MOBILE_CORE", "MOBILE_TESTS", "JOURNEYS", "WEB", "WORKFLOWS", "UNKNOWN"} {
		env = append(env, k+"="+value)
	}
	return env
}

func runAttested(t *testing.T, statusTree string, fail bool, filters []string) (string, string) {
	return runAttestedRows(t, statusTree, "", fail, filters)
}

func runAttestedRows(t *testing.T, statusTree, rows string, fail bool, filters []string) (string, string) {
	t.Helper()
	r := newReuseRepo(t)
	head := r.commit("change.txt", "one\n")
	tree := r.git("rev-parse", head+"^{tree}")
	if statusTree == "head" {
		statusTree = tree
	}
	if rows != "" {
		statusTree += " rows " + rows
	}
	statuses := []map[string]string{
		{"context": "verify", "state": "success", "description": "tree " + tree},
		{"context": "stage0/ok", "state": "success", "description": "tree " + statusTree},
	}
	data, _ := json.Marshal(statuses)
	r.write(filepath.Join(r.fake, "statuses.json"), string(data), 0o644)
	r.write(filepath.Join(r.bin, "gh"), fakeStatusGH, 0o755)
	if fail {
		r.write(filepath.Join(r.fake, "fail"), "x", 0o644)
	}
	out := filepath.Join(t.TempDir(), "out")
	cmd := exec.Command("bash", filepath.Join(repoRoot(t), "scripts", "ci", "stage0-attested.sh"))
	cmd.Dir = r.dir
	cmd.Env = append(os.Environ(),
		"PATH="+r.bin+string(os.PathListSeparator)+os.Getenv("PATH"),
		"FAKE_GH="+r.fake, "HEAD_SHA="+head, "GITHUB_REPOSITORY=o/r", "GITHUB_OUTPUT="+out)
	cmd.Env = append(cmd.Env, filters...)
	log, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("stage0-attested.sh: %v\n%s", err, log)
	}
	written, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	return strings.TrimSpace(string(written)), string(log)
}

func TestStage0Attested_skipsOnAMatchingStatusWithEveryFilterFalse(t *testing.T) {
	got, log := runAttested(t, "head", false, allFilters("false"))
	if got != "skip=true" || !strings.Contains(log, "stage0/ok") {
		t.Fatalf("output %q log %q, want skip=true", got, log)
	}
}

func TestStage0Attested_aTrueFilterKeepsStage1(t *testing.T) {
	for _, k := range []string{"IOS", "MOBILE_CORE", "MOBILE_TESTS", "JOURNEYS", "WEB", "WORKFLOWS", "UNKNOWN"} {
		got, log := runAttested(t, "head", false, append(allFilters("false"), k+"=true"))
		if got != "skip=false" || !strings.Contains(log, k) {
			t.Fatalf("%s=true: output %q log %q, want skip=false naming the filter", k, got, log)
		}
	}
}

func TestStage0Attested_aStatusOnAnotherTreeKeepsStage1(t *testing.T) {
	got, log := runAttested(t, strings.Repeat("0", 40), false, allFilters("false"))
	if got != "skip=false" || !strings.Contains(log, "no stage0/ok status for tree") {
		t.Fatalf("output %q log %q, want skip=false", got, log)
	}
}

func TestStage0Attested_anAPIFailureWarnsAndKeepsStage1(t *testing.T) {
	got, log := runAttested(t, "head", true, allFilters("false"))
	if got != "skip=false" || !strings.Contains(log, "::warning::") {
		t.Fatalf("output %q log %q, want skip=false and a warning", got, log)
	}
}

const allRows = "go build,go vet,go test -short,swift test,xcode,journeys"

func TestStage0Attested_aCoveringRowSkipsStage1(t *testing.T) {
	for filter, row := range map[string]string{
		"IOS": "xcode", "MOBILE_CORE": "swift test", "MOBILE_TESTS": "swift test", "JOURNEYS": "journeys",
	} {
		got, log := runAttestedRows(t, "head", allRows, false, append(allFilters("false"), filter+"=true"))
		if got != "skip=true" || !strings.Contains(log, filter+" covered by "+row) {
			t.Fatalf("%s=true: output %q log %q, want skip=true naming %s", filter, got, log, row)
		}
	}
}

func TestStage0Attested_aMissingRowKeepsStage1(t *testing.T) {
	got, log := runAttestedRows(t, "head", "go build,swift test", false, append(allFilters("false"), "IOS=true"))
	if got != "skip=false" || !strings.Contains(log, "no xcode row passed") {
		t.Fatalf("output %q log %q, want skip=false", got, log)
	}
}

func TestStage0Attested_aFilterNoRowCoversKeepsStage1WithEveryRow(t *testing.T) {
	for _, k := range []string{"WEB", "WORKFLOWS", "UNKNOWN"} {
		got, log := runAttestedRows(t, "head", allRows, false, append(allFilters("false"), k+"=true"))
		if got != "skip=false" || !strings.Contains(log, k) {
			t.Fatalf("%s=true: output %q log %q, want skip=false", k, got, log)
		}
	}
}

func TestStage0Attested_anOldDescriptionWithoutRowsKeepsStage1ForAMobileFilter(t *testing.T) {
	got, log := runAttested(t, "head", false, append(allFilters("false"), "IOS=true"))
	if got != "skip=false" || !strings.Contains(log, "no xcode row passed") {
		t.Fatalf("output %q log %q, want skip=false", got, log)
	}
}
