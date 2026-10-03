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
		{ID: "00", Module: "system", Trigger: "POST /v1/system/pings"},
		{ID: "01", Module: "identity", Trigger: "POST /v1/auth/session"},
		{ID: "01a", Module: "identity", Trigger: "PUT /v1/me/handle"},
		{ID: "02", Module: "cabal", Trigger: "POST /v1/cabals;DELETE /v1/cabals/{id}"},
		{ID: "10", Module: "governance", Trigger: "consumer:proposal.passed"},
	}
	every := []string{"00", "01", "01a", "02", "10"}
	for _, tc := range []struct {
		changed, ops, want []string
	}{
		{nil, nil, nil},
		{[]string{"README.md", "apps/backend/internal/platform/db/db.go", "docs/flows/00.md"}, nil, nil},
		{[]string{"packages/flows/app/00.tsv"}, nil, []string{"00"}},
		{[]string{"packages/flows/app/01a.tsv"}, nil, []string{"01a"}},
		{[]string{"packages/flows/app/99.tsv", "packages/flows/app/README.md"}, nil, nil},
		{[]string{"packages/flows/backend/10.tsv"}, nil, []string{"10"}},
		{[]string{"packages/flows/Sources/MonacoFlows/Flow01.gen.swift"}, nil, []string{"01"}},
		{[]string{"packages/flows/Sources/MonacoFlows/Flow01a.gen.swift"}, nil, []string{"01a"}},
		{[]string{"packages/flows/Sources/MonacoFlows/Flow00Scenarios.gen.swift"}, nil, []string{"00"}},
		{[]string{"packages/mobile-core/Sources/MonacoSystem/Flow00SystemPingModel.swift"}, nil, []string{"00"}},
		{[]string{"packages/mobile-core/Sources/MonacoSystem/Module.swift"}, nil, nil},
		{[]string{"packages/mobile-core/Tests/MonacoCoreTests/F00IntegrationTests.swift"}, nil, []string{"00"}},
		{[]string{"packages/mobile-core/Tests/MonacoCoreTests/Flow02ModelTests.swift"}, nil, []string{"02"}},
		{[]string{"packages/mobile-core/Tests/MonacoCoreTests/FlowOutcomeTests.swift"}, nil, nil},
		{[]string{"packages/mobile-core/Tests/MonacoAPITests/FakeStreamTransport.swift"}, nil, nil},
		{[]string{"apps/backend/internal/testkit/flows/f01a.go"}, nil, []string{"01a"}},
		{[]string{"apps/backend/internal/testkit/flows/seed.go"}, nil, nil},
		{[]string{"apps/backend/internal/modules/system/http.go"}, nil, []string{"00"}},
		{[]string{"apps/backend/internal/modules/identity/app/app.go"}, nil, []string{"01", "01a"}},
		{[]string{"apps/backend/internal/modules/funding/app.go"}, nil, nil},
		{[]string{"apps/backend/api/openapi.yaml"}, nil, nil},
		{[]string{"apps/backend/api/openapi.yaml"}, []string{"PUT /v1/me/handle"}, []string{"01a"}},
		{[]string{"apps/backend/api/spec/cabal.yaml"}, []string{"DELETE /v1/cabals/{id}"}, []string{"02"}},
		{[]string{"apps/backend/api/openapi.yaml"}, []string{"GET /v1/cabals/{id}", "POST /v1/auth/session"}, []string{"01"}},
		{[]string{"README.md"}, []string{"POST /v1/auth/session"}, nil},
		{[]string{"packages/flows/Package.swift"}, nil, every},
		{[]string{"apps/backend/internal/tools/flows/check.go"}, nil, every},
		{[]string{"apps/backend/internal/testkit/scenarios/scenarios.go"}, nil, every},
		{[]string{"apps/backend/internal/testkit/fakes/privy.go"}, nil, every},
		{[]string{"apps/backend/internal/testkit/fakes/f01a_handles.go"}, nil, []string{"01a"}},
		{[]string{"apps/backend/internal/testkit/scenarios/02-cabal-with-creator.jsonl"}, nil, []string{"02"}},
		{[]string{"apps/backend/internal/testkit/scenarios/flow10_votes.go"}, nil, []string{"10"}},
		{[]string{"apps/backend/internal/testkit/scenarios/99-unknown.jsonl"}, nil, nil},
		{[]string{"apps/backend/internal/testkit/scenarios/cabal-with-creator.jsonl"}, nil, nil},
		{[]string{"apps/backend/internal/testkit/fakes/testdata/fakes/privy/v1/users/qa-onb-x-ok.json"}, nil, nil},
		{[]string{"apps/backend/internal/testkit/fakes/fakes_test.go"}, nil, nil},
		{[]string{"packages/flows/app/10.tsv", "apps/backend/internal/modules/system/x.go"}, nil, []string{"00", "10"}},
	} {
		if got := flows.Affected(tc.changed, tc.ops, backend); !slices.Equal(got, tc.want) {
			t.Errorf("Affected(%q, %q) = %q, want %q", tc.changed, tc.ops, got, tc.want)
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
