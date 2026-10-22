package nogo

import (
	"go/token"
	"strings"
	"testing"

	"golang.org/x/tools/go/analysis"
)

func TestRunCaldate_rejectsAMalformedAllowFile(t *testing.T) {
	t.Parallel()
	orig := caldateAllow
	caldateAllow = "onlypath\n"
	t.Cleanup(func() { caldateAllow = orig })
	if _, err := runCaldate(&analysis.Pass{Fset: token.NewFileSet(), Analyzer: CaldateAnalyzer()}); err == nil ||
		!strings.Contains(err.Error(), "allow line 1") {
		t.Fatalf("runCaldate = %v, want the bad allow file", err)
	}
}
