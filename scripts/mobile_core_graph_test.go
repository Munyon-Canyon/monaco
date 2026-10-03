package scripts_test

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"testing"
)

// allowedGraph is the module dependency table in docs/architecture/ios.md#module-targets: each
// mobile-core module target and the module targets it may import beyond MonacoAPI and MonacoFlows.
var allowedGraph = map[string][]string{
	"MonacoSystem":     {},
	"MonacoIdentity":   {},
	"MonacoAnalytics":  {},
	"MonacoMarket":     {},
	"MonacoNotify":     {"MonacoIdentity"},
	"MonacoReferrals":  {"MonacoIdentity"},
	"MonacoCabal":      {"MonacoIdentity"},
	"MonacoSocial":     {"MonacoIdentity", "MonacoCabal"},
	"MonacoTreasury":   {"MonacoCabal"},
	"MonacoTrading":    {"MonacoTreasury", "MonacoMarket"},
	"MonacoGovernance": {"MonacoCabal", "MonacoTreasury"},
	"MonacoRanking":    {"MonacoCabal", "MonacoMarket", "MonacoTrading"},
}

var nonModuleTargets = []string{"MonacoCore", "MonacoAPI", "MonacoTestClock", "MonacoTestSupport"}

const graphFix = "edit the allowed table in scripts/mobile_core_graph_test.go and the doc together"

type dumpedTarget struct {
	Name         string             `json:"name"`
	Type         string             `json:"type"`
	Dependencies []map[string][]any `json:"dependencies"`
}

// targetEdges keeps the in-package dependencies of each regular target; products from other
// packages, such as MonacoFlows, are not edges of the graph.
func targetEdges(targets []dumpedTarget) map[string][]string {
	edges := map[string][]string{}
	for _, t := range targets {
		if t.Type != "regular" {
			continue
		}
		deps := []string{}
		for _, d := range t.Dependencies {
			for kind, args := range d {
				if kind != "byName" && kind != "target" {
					continue
				}
				if name, ok := args[0].(string); ok {
					deps = append(deps, name)
				}
			}
		}
		edges[t.Name] = deps
	}
	return edges
}

func graphProblems(edges map[string][]string) []string {
	var problems []string
	for name := range edges {
		if _, ok := allowedGraph[name]; !ok && !slices.Contains(nonModuleTargets, name) {
			problems = append(problems, fmt.Sprintf("target %s: not in the allowed table", name))
		}
	}
	for name, allowed := range allowedGraph {
		deps, ok := edges[name]
		if !ok {
			problems = append(problems, fmt.Sprintf("target %s: missing from Package.swift", name))
			continue
		}
		for _, d := range deps {
			switch {
			case d == "MonacoCore":
				problems = append(problems, fmt.Sprintf("target %s: depends on MonacoCore, which no module target may import", name))
			case d == "MonacoAPI":
			case !slices.Contains(allowed, d):
				problems = append(problems, fmt.Sprintf("target %s: extra edge %s -> %s", name, name, d))
			}
		}
		for _, a := range allowed {
			if !slices.Contains(deps, a) {
				problems = append(problems, fmt.Sprintf("target %s: missing edge %s -> %s", name, name, a))
			}
		}
	}
	sort.Strings(problems)
	return problems
}

func allowedTable() string {
	names := make([]string, 0, len(allowedGraph))
	for name := range allowedGraph {
		names = append(names, name)
	}
	sort.Strings(names)
	var b strings.Builder
	for _, name := range names {
		deps := strings.Join(allowedGraph[name], ", ")
		if deps == "" {
			deps = "none"
		}
		fmt.Fprintf(&b, "  %-17s -> %s\n", name, deps)
	}
	return b.String()
}

func TestMobileCoreGraph_packageMatchesTheAllowedTable(t *testing.T) {
	if _, err := exec.LookPath("swift"); err != nil {
		t.Skip("swift is not on PATH; the mobile-core graph check needs swift package dump-package")
	}
	cmd := exec.Command("swift", "package", "dump-package")
	cmd.Dir = filepath.Join(repoRoot(t), "packages/mobile-core")
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("swift package dump-package: %v", err)
	}
	var pkg struct {
		Targets []dumpedTarget `json:"targets"`
	}
	if err := json.Unmarshal(out, &pkg); err != nil {
		t.Fatalf("decode dump-package: %v", err)
	}
	if problems := graphProblems(targetEdges(pkg.Targets)); len(problems) > 0 {
		t.Fatalf("mobile-core module graph differs from the allowed table:\n  %s\n%s.\nallowed table:\n%s",
			strings.Join(problems, "\n  "), graphFix, allowedTable())
	}
}

func TestMobileCoreGraph_reportsEachViolation(t *testing.T) {
	cases := map[string]struct {
		edit func(map[string][]string)
		want string
	}{
		"extra edge": {
			func(e map[string][]string) { e["MonacoIdentity"] = append(e["MonacoIdentity"], "MonacoTrading") },
			"target MonacoIdentity: extra edge MonacoIdentity -> MonacoTrading",
		},
		"missing edge": {
			func(e map[string][]string) { e["MonacoTrading"] = []string{"MonacoAPI", "MonacoMarket"} },
			"target MonacoTrading: missing edge MonacoTrading -> MonacoTreasury",
		},
		"depends on MonacoCore": {
			func(e map[string][]string) { e["MonacoSystem"] = append(e["MonacoSystem"], "MonacoCore") },
			"target MonacoSystem: depends on MonacoCore",
		},
		"removed module": {
			func(e map[string][]string) { delete(e, "MonacoGovernance") },
			"target MonacoGovernance: missing from Package.swift",
		},
		"unknown module": {
			func(e map[string][]string) { e["MonacoWallet"] = nil },
			"target MonacoWallet: not in the allowed table",
		},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			edges := map[string][]string{"MonacoCore": {"MonacoAPI"}, "MonacoAPI": {}}
			for module, deps := range allowedGraph {
				edges[module] = append([]string{"MonacoAPI"}, deps...)
			}
			if problems := graphProblems(edges); len(problems) != 0 {
				t.Fatalf("allowed graph reported problems: %v", problems)
			}
			c.edit(edges)
			problems := graphProblems(edges)
			if len(problems) != 1 || !strings.HasPrefix(problems[0], c.want) {
				t.Fatalf("problems = %q, want one starting %q", problems, c.want)
			}
		})
	}
}
