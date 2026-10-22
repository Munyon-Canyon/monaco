package nogo

import (
	"go/token"
	"strings"
	"testing"

	"golang.org/x/tools/go/analysis"
)

func TestRunUowdo_rejectsAMalformedAllowFile(t *testing.T) {
	t.Parallel()
	orig := uowdoAllow
	uowdoAllow = "onlypath\n"
	t.Cleanup(func() { uowdoAllow = orig })
	if _, err := runUowdo(&analysis.Pass{Fset: token.NewFileSet(), Analyzer: UowdoAnalyzer()}); err == nil ||
		!strings.Contains(err.Error(), "allow line 1") {
		t.Fatalf("runUowdo = %v, want the bad allow file", err)
	}
}
