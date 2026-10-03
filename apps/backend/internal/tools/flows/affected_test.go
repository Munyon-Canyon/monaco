package flows_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/tools/flows"
)

func TestAffected(t *testing.T) {
	t.Parallel()
	backend := []flows.Flow{
		{ID: "00", Module: "system"},
		{ID: "01", Module: "identity"},
		{ID: "01a", Module: "identity"},
		{ID: "02", Module: "cabal"},
		{ID: "10", Module: "governance"},
	}
	app := map[string]flows.AppRow{
		"00": {ID: "00", Status: flows.AppVerified},
		"01": {ID: "01", Status: flows.AppBuilt},
		"02": {ID: "02", Status: flows.AppPlanned},
	}
	every := []string{"00", "01", "01a", "02", "10"}
	for _, tc := range []struct {
		changed []string
		want    []string
	}{
		{nil, nil},
		{[]string{"README.md", "apps/backend/internal/platform/db/db.go", "docs/flows/00.md"}, nil},
		{[]string{"packages/flows/app/00.tsv"}, []string{"00"}},
		{[]string{"packages/flows/app/01a.tsv"}, []string{"01a"}},
		{[]string{"packages/flows/app/99.tsv", "packages/flows/app/README.md"}, nil},
		{[]string{"packages/flows/backend/10.tsv"}, []string{"10"}},
		{[]string{"packages/flows/Sources/MonacoFlows/Flow01.gen.swift"}, []string{"01"}},
		{[]string{"packages/flows/Sources/MonacoFlows/Flow01a.gen.swift"}, []string{"01a"}},
		{[]string{"packages/flows/Sources/MonacoFlows/Flow00Scenarios.gen.swift"}, []string{"00"}},
		{[]string{"packages/mobile-core/Sources/MonacoSystem/Flow00SystemPingModel.swift"}, []string{"00"}},
		{[]string{"packages/mobile-core/Sources/MonacoSystem/Module.swift"}, nil},
		{[]string{"packages/mobile-core/Tests/MonacoCoreTests/F00IntegrationTests.swift"}, []string{"00"}},
		{[]string{"packages/mobile-core/Tests/MonacoCoreTests/Flow02ModelTests.swift"}, []string{"02"}},
		{[]string{"packages/mobile-core/Tests/MonacoCoreTests/FlowOutcomeTests.swift"}, nil},
		{[]string{"packages/mobile-core/Tests/MonacoAPITests/FakeStreamTransport.swift"}, nil},
		{[]string{"apps/backend/internal/testkit/flows/f01a.go"}, []string{"01a"}},
		{[]string{"apps/backend/internal/testkit/flows/seed.go"}, nil},
		{[]string{"apps/backend/internal/modules/system/http.go"}, []string{"00"}},
		{[]string{"apps/backend/internal/modules/identity/app/app.go"}, []string{"01", "01a"}},
		{[]string{"apps/backend/internal/modules/funding/app.go"}, nil},
		{[]string{"apps/backend/api/openapi.yaml"}, []string{"00", "01"}},
		{[]string{"apps/backend/api/spec/cabal.yaml"}, []string{"00", "01"}},
		{[]string{"packages/flows/Package.swift"}, every},
		{[]string{"apps/backend/internal/tools/flows/check.go"}, every},
		{[]string{"apps/backend/internal/testkit/scenarios/scenarios.go"}, every},
		{[]string{"apps/backend/internal/testkit/fakes/privy.go"}, every},
		{[]string{"packages/flows/app/10.tsv", "apps/backend/internal/modules/system/x.go"}, []string{"00", "10"}},
	} {
		if got := flows.Affected(tc.changed, backend, app); !slices.Equal(got, tc.want) {
			t.Errorf("Affected(%q) = %q, want %q", tc.changed, got, tc.want)
		}
	}
}

func TestChecks_reportOnlyTheFlowsInIDs(t *testing.T) {
	t.Parallel()
	broken := func(id string) string {
		return fundRowWith(func(c []string) { c[0], c[5] = id, "cabal.exploded" })
	}
	parsed, problems := flows.Parse(fundFile, strings.NewReader(tsv(broken("07"), broken("08"))))
	if len(problems) != 0 {
		t.Fatalf("parse problems = %v", lines(problems))
	}
	env := testEnv()
	env.Scripts = func(flows.Flow, string) bool { return false }
	results, err := flows.ReadTestResults(strings.NewReader(passed("TestFlow07_Ghost_OK") + "\n" +
		passed("TestFlow08_Ghost_OK") + "\n"))
	if err != nil {
		t.Fatal(err)
	}
	all := func(ids []string) []string {
		return lines(slices.Concat(
			flows.CheckColumns(
				parsed,
				env,
				ids,
			),
			flows.CheckTests(parsed, results, ids),
			flows.CheckScripts(parsed, env, ids),
		))
	}
	every, only := all(nil), all([]string{"08"})
	if len(only) == 0 || len(every) != 2*len(only) {
		t.Fatalf("problems for 08 = %q, for every flow = %q", only, every)
	}
	for _, p := range only {
		if !strings.HasPrefix(p, fundFile+":3:") && !strings.Contains(p, "TestFlow08_") {
			t.Errorf("problem %q is not flow 08's", p)
		}
	}
}
