package main

import (
	"bytes"
	"io"
	"strings"
	"testing"
)

func TestGarden_rejectsAnythingButReportWithUsageAndExit2(t *testing.T) {
	t.Parallel()
	for _, args := range [][]string{nil, {"prune"}, {"report", "--bogus"}, {"report", "extra"}} {
		var stderr bytes.Buffer
		if code := gardenCmd(
			t.TempDir(),
			args,
			io.Discard,
			&stderr,
		); code != 2 ||
			!strings.Contains(stderr.String(), gardenUsage) {
			t.Fatalf("garden %v: exit %d, stderr %q; want 2 and the usage", args, code, stderr.String())
		}
	}
}

func TestGarden_turnsAMutantKeyIntoAFinding(t *testing.T) {
	t.Parallel()
	key := mutantKey("internal/errs", "errs.go", 42, 7, "CONDITIONALS_NEGATION")
	got := mutantFinding(key)
	if got.Rule != "CONDITIONALS_NEGATION" || got.File != "internal/errs/errs.go" || got.Line != 42 {
		t.Fatalf("finding %+v from %q, want CONDITIONALS_NEGATION at internal/errs/errs.go:42", got, key)
	}
}
