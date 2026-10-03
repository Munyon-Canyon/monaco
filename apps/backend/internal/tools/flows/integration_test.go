package flows_test

import (
	"slices"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/tools/flows"
)

func TestCheckIntegration_aMultiCommandFlowNeedsOnePassingTestPerOutcome(t *testing.T) {
	t.Parallel()
	backend := []flows.Flow{{
		ID: "03", Commands: []string{"RequestAccess", "DecideAccess"},
		Outcomes: []flows.Outcome{flows.OutcomeOK, "Forbidden", "crash:after-commit", "crash:after-publish"},
	}}
	app := []flows.AppRow{{File: "packages/flows/app/03.tsv", Line: 2, ID: "03", Status: flows.AppVerified}}
	const class = "F03IntegrationTests."
	for _, tc := range []struct {
		name    string
		results flows.IntegrationResults
		want    []string
	}{
		{"each outcome passed under either command", flows.IntegrationResults{
			class + "test_F03_RequestAccess_ok":          flows.IntegrationPassed,
			class + "test_F03_DecideAccess_Forbidden":    flows.IntegrationPassed,
			class + "test_F03_RequestAccess_interrupted": flows.IntegrationPassed,
		}, nil},
		{"a pass under one command covers a skip under the other", flows.IntegrationResults{
			class + "test_F03_RequestAccess_ok":         flows.IntegrationPassed,
			class + "test_F03_DecideAccess_ok":          flows.IntegrationSkipped,
			class + "test_F03_DecideAccess_Forbidden":   flows.IntegrationPassed,
			class + "test_F03_DecideAccess_interrupted": flows.IntegrationPassed,
		}, nil},
		{"no command passed an outcome", flows.IntegrationResults{
			class + "test_F03_RequestAccess_ok":       flows.IntegrationFailed,
			class + "test_F03_DecideAccess_ok":        flows.IntegrationSkipped,
			class + "test_F03_DecideAccess_Forbidden": flows.IntegrationSkipped,
		}, []string{
			"packages/flows/app/03.tsv:2: flow 03: app verified but F03IntegrationTests has a failing test_F03_RequestAccess_ok",
			"packages/flows/app/03.tsv:2: flow 03: app verified but F03IntegrationTests skipped test_F03_DecideAccess_Forbidden",
			"packages/flows/app/03.tsv:2: flow 03: app verified but F03IntegrationTests lacks " +
				"test_F03_RequestAccess_interrupted or test_F03_DecideAccess_interrupted",
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var got []string
			for _, p := range flows.CheckIntegration(app, backend, tc.results) {
				got = append(got, p.String())
			}
			if !slices.Equal(got, tc.want) {
				t.Fatalf("got\n%q\nwant\n%q", got, tc.want)
			}
		})
	}
}
