package flows_test

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"maps"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/testkit/flows"
	"github.com/monaco/monaco/apps/backend/internal/testkit/scenario"
)

func scriptFuncs(t *testing.T, name string, src any) []string {
	t.Helper()
	file, err := parser.ParseFile(token.NewFileSet(), name, src, parser.SkipObjectResolution)
	if err != nil {
		t.Fatal(err)
	}
	script := regexp.MustCompile(`^F[0-9]+[a-z]?[A-Z]`)
	var declared []string
	for _, decl := range file.Decls {
		if fn, ok := decl.(*ast.FuncDecl); ok && fn.Recv == nil && script.MatchString(fn.Name.Name) {
			declared = append(declared, fn.Name.Name)
		}
	}
	return declared
}

func registryProblem(declared []string, registry map[string]flows.Script) string {
	declared = slices.Sorted(slices.Values(declared))
	registered := slices.Sorted(maps.Keys(registry))
	if slices.Equal(declared, registered) {
		return ""
	}
	return fmt.Sprintf("flow scripts declared %v, registered in Scripts() %v", declared, registered)
}

func TestScripts_registersEveryFlowScriptInThePackage(t *testing.T) {
	t.Parallel()
	names, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	var declared []string
	for _, name := range names {
		if !strings.HasSuffix(name, "_test.go") {
			declared = append(declared, scriptFuncs(t, name, nil)...)
		}
	}
	if problem := registryProblem(declared, flows.Scripts()); problem != "" {
		t.Fatal(problem)
	}
}

func TestEnv_shortensThePricePollerForFlow18(t *testing.T) {
	t.Parallel()
	got := flows.Env()["18"]
	want := []string{"MARKET_PRICE_POLL_INTERVAL=2s", "MONACO_TIMEOUT_JUPITER_QUOTE=1s"}
	if !slices.Equal(got, want) {
		t.Fatalf("Env()[18] = %q, want %q", got, want)
	}
}

func TestScripts_aSubRowScriptIsRegisteredUnderItsLowercaseLetter(t *testing.T) {
	t.Parallel()
	registry := map[string]flows.Script{"F01aSetHandleOK": func(*scenario.Scenario) {}}
	for _, tc := range []struct{ name, declares, problem string }{
		{"a lettered script is registered under its own name", "F01aSetHandleOK", ""},
		{
			"an uppercase letter does not stand in for the sub-row letter", "F01ASetHandleOK",
			"flow scripts declared [F01ASetHandleOK], registered in Scripts() [F01aSetHandleOK]",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			declared := scriptFuncs(t, "scratch.go", "package flows\n\nfunc "+tc.declares+"() {}\n")
			if got := registryProblem(declared, registry); got != tc.problem {
				t.Fatalf("problem = %q, want %q", got, tc.problem)
			}
		})
	}
}

func TestAlone_marksOnlyTheDeclaredScripts(t *testing.T) {
	t.Parallel()
	if !flows.Alone(flows.F14CashOutPayoutsRPCUnavailable) {
		t.Error("flow 14 RPCUnavailable shares the fake getLatestBlockhash route, want it to run alone")
	}
	if flows.Alone(flows.F14CashOutPayoutsPrivyUnavailable) {
		t.Error("flow 14 PrivyUnavailable scripts its own treasury wallet route, want it to run with others")
	}
}
