package nogo

import (
	"go/token"
	"strings"
	"testing"

	"golang.org/x/tools/go/analysis"
)

func TestAllowFrom_requiresOneReasonPerLine(t *testing.T) {
	t.Parallel()
	list, err := allowFrom("# comment\n\ninternal/ok/ok_test.go a real reason\n")
	if err != nil || !allowedPath(list, "/repo/internal/ok/ok_test.go") || allowedPath(list, "/repo/other_test.go") {
		t.Fatalf("allowFrom = %v, %v", list, err)
	}
	_, err = allowFrom("onlypath\n")
	if err == nil || !strings.Contains(err.Error(), "allow line 1") {
		t.Fatalf("missing reason = %v", err)
	}
	_, err = allowFrom("a/b.go reason\na/b.go again\n")
	if err == nil {
		t.Fatal("duplicate allow line was accepted")
	}
	got := err.Error()
	if !strings.Contains(got, "allow line 2") || !strings.Contains(got, "duplicate a/b.go") {
		t.Fatalf("duplicate = %v", err)
	}
	orig := wallclockAllow
	wallclockAllow = "onlypath\n"
	t.Cleanup(func() { wallclockAllow = orig })
	if _, err := runWallclock(&analysis.Pass{Fset: token.NewFileSet(), Analyzer: WallclockAnalyzer()}); err == nil ||
		!strings.Contains(err.Error(), "allow line 1") {
		t.Fatalf("runWallclock = %v, want the bad allow file", err)
	}
}
