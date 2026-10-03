package flows_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/tools/flows"
)

func testJSON(events ...string) string {
	return strings.Join(events, "\n") + "\n"
}

func passed(name string) string {
	return `{"Action":"pass","Package":"p","Test":"` + name + `"}`
}

func TestReadTestResults_keepsTheWorstResultPerTestAndSkipsNoise(t *testing.T) {
	t.Parallel()
	input := testJSON(
		`{"Action":"run","Package":"p","Test":"TestA"}`,
		passed("TestA"),
		`{"Action":"fail","Package":"p","Test":"TestB"}`,
		passed("TestB"),
		"# p [build failed]",
		`{"Action":"pass","Package":"p"}`,
		passed("TestC/sub"),
	) + `{"Action":"pass","Package":"p","Test":"TestNoNewline"}`
	got, err := flows.ReadTestResults(strings.NewReader(input))
	if err != nil {
		t.Fatal(err)
	}
	want := flows.TestResults{"TestA": true, "TestB": false, "TestC/sub": true, "TestNoNewline": true}
	if len(got) != len(want) {
		t.Fatalf("results = %v, want %v", got, want)
	}
	for name, ok := range want {
		if got[name] != ok {
			t.Fatalf("results = %v, want %v", got, want)
		}
	}
}

func TestCheckTests_builtFlowsNeedAPassingTestPerOutcome(t *testing.T) {
	t.Parallel()
	all := testJSON(
		passed("TestFlow07_FundCabal_OK"),
		passed("TestFlow07_FundCabal_InsufficientFunds"),
		passed("TestFlow07_FundCabal_CrashAfterSign"),
	)
	oneMissing := testJSON(passed("TestFlow07_FundCabal_OK"), passed("TestFlow07_FundCabal_CrashAfterSign"))
	oneFailed := all + `{"Action":"fail","Package":"p","Test":"TestFlow07_FundCabal_CrashAfterSign"}` + "\n"
	pass07OK := passed("TestFlow07_FundCabal_OK")
	planned := fundRowWith(func(c []string) { c[8] = "planned" })
	verified := fundRowWith(func(c []string) { c[8] = "verified" })
	sub := fundRowWith(func(c []string) { c[0], c[4], c[7] = "01a", "SetHandle", "ok" })
	for _, tc := range []struct {
		name  string
		row   string
		input string
		want  []string
	}{
		{"built with all tests passing", fundRow, all, nil},
		{"verified with all tests passing", verified, all, nil},
		{"planned needs no tests", planned, "", nil},
		{
			"built with one test missing", fundRow, oneMissing,
			[]string{"flows.tsv:2: outcome InsufficientFunds has no test TestFlow07_FundCabal_InsufficientFunds in the go test -json input"},
		},
		{
			"built with one test failed", fundRow, oneFailed,
			[]string{"flows.tsv:2: outcome crash:after-sign test TestFlow07_FundCabal_CrashAfterSign failed"},
		},
		{"planned row owns its tests", planned, pass07OK, nil},
		{"non-flow tests and subtests are ignored", fundRow, all + testJSON(passed("TestFlowsCheck"), passed("TestFlow07_FundCabal_OK/sub")), nil},
		{
			"test for a deleted row", fundRow, all + passed("TestFlow09_Vote_OK"),
			[]string{"flows.tsv: test TestFlow09_Vote_OK matches no flow outcome; delete the test or add its row"},
		},
		{
			"test for a removed outcome", fundRow, all + `{"Action":"fail","Package":"p","Test":"TestFlow07_FundCabal_Paused"}`,
			[]string{"flows.tsv: test TestFlow07_FundCabal_Paused matches no flow outcome; delete the test or add its row"},
		},
		{"sub-row test matches its row", sub, passed("TestFlow01a_SetHandle_OK"), nil},
		{
			"test for a deleted sub-row", sub, testJSON(passed("TestFlow01a_SetHandle_OK"), passed("TestFlow01b_SetHandle_OK")),
			[]string{"flows.tsv: test TestFlow01b_SetHandle_OK matches no flow outcome; delete the test or add its row"},
		},
		{
			"uppercase letter does not make a flow test name", sub, passed("TestFlow01A_SetHandle_OK"),
			[]string{"flows.tsv:2: outcome ok has no test TestFlow01a_SetHandle_OK in the go test -json input"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			parsed, problems := flows.Parse(strings.NewReader(tsv(tc.row)))
			if len(problems) != 0 {
				t.Fatalf("parse problems = %v", lines(problems))
			}
			results, err := flows.ReadTestResults(strings.NewReader(tc.input))
			if err != nil {
				t.Fatal(err)
			}
			if got := lines(flows.CheckTests(parsed, results)); !slices.Equal(got, tc.want) {
				t.Fatalf("problems = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestCheckTests_aMultiCommandRowTakesEachOutcomeFromAnyCommandAndTestsEveryCommand(t *testing.T) {
	t.Parallel()
	row := fundRowWith(func(c []string) { c[4], c[7] = "FundCabal;Refund", "ok;InsufficientFunds" })
	split := testJSON(passed("TestFlow07_FundCabal_OK"), passed("TestFlow07_Refund_InsufficientFunds"))
	for _, tc := range []struct {
		name  string
		input string
		want  []string
	}{
		{"each outcome from one command", split, nil},
		{"an outcome from both commands", split + passed("TestFlow07_Refund_OK"), nil},
		{
			"an outcome no command tests", testJSON(passed("TestFlow07_FundCabal_OK"), passed("TestFlow07_Refund_OK")),
			[]string{"flows.tsv:2: outcome InsufficientFunds has no test TestFlow07_FundCabal_InsufficientFunds " +
				"or TestFlow07_Refund_InsufficientFunds in the go test -json input"},
		},
		{
			"a command with no test",
			testJSON(passed("TestFlow07_FundCabal_OK"), passed("TestFlow07_FundCabal_InsufficientFunds")),
			[]string{"flows.tsv:2: command Refund has no flow test in the go test -json input"},
		},
		{
			"one of two tests for an outcome failed",
			split + `{"Action":"fail","Package":"p","Test":"TestFlow07_Refund_OK"}` + "\n",
			[]string{"flows.tsv:2: outcome ok test TestFlow07_Refund_OK failed"},
		},
		{
			"a test for a command the row does not list", split + passed("TestFlow07_Close_OK"),
			[]string{"flows.tsv: test TestFlow07_Close_OK matches no flow outcome; delete the test or add its row"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			parsed, problems := flows.Parse(strings.NewReader(tsv(row)))
			if len(problems) != 0 {
				t.Fatalf("parse problems = %v", lines(problems))
			}
			results, err := flows.ReadTestResults(strings.NewReader(tc.input))
			if err != nil {
				t.Fatal(err)
			}
			if got := lines(flows.CheckTests(parsed, results)); !slices.Equal(got, tc.want) {
				t.Fatalf("problems = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestCheckScripts_aMultiCommandRowNeedsOneScriptPerOutcomeFromAnyCommand(t *testing.T) {
	t.Parallel()
	row := fundRowWith(func(c []string) { c[4], c[7] = "FundCabal;Refund", "ok;InsufficientFunds" })
	parsed, _ := flows.Parse(strings.NewReader(tsv(row)))
	env := testEnv()
	env.Scripts = func(_ flows.Flow, name string) bool { return name == "F07RefundOK" }
	want := []string{"flows.tsv:2: built flow outcome InsufficientFunds has no script " +
		"F07FundCabalInsufficientFunds or F07RefundInsufficientFunds in internal/testkit/flows; " +
		"monacoctl verify all fails without it"}
	if got := lines(flows.CheckScripts(parsed, env)); !slices.Equal(got, want) {
		t.Fatalf("problems = %q, want %q", got, want)
	}
}

func TestCheckScripts_builtFlowsNeedNonCrashScriptsAndVerifiedFlowsNeedEveryOutcome(t *testing.T) {
	t.Parallel()
	ok := "F07FundCabalOK"
	crash := "F07FundCabalCrashAfterSign"
	builtMissing := "flows.tsv:2: built flow outcome ok has no script F07FundCabalOK " +
		"in internal/testkit/flows; monacoctl verify all fails without it"
	verifiedOK := "flows.tsv:2: verified flow outcome ok has no script F07FundCabalOK " +
		"in internal/testkit/flows for monacoctl verify all"
	verifiedCrash := "flows.tsv:2: verified flow outcome crash:after-sign has no script F07FundCabalCrashAfterSign " +
		"in internal/testkit/flows for monacoctl verify all"
	for _, tc := range []struct {
		name, status string
		scripts      []string
		want         []string
	}{
		{"planned needs no script", "planned", nil, nil},
		{"built with the non-crash script", "built", []string{ok}, nil},
		{"built with every script", "built", []string{ok, crash}, nil},
		{"built missing the non-crash script", "built", nil, []string{builtMissing}},
		{"built holding only the crash script", "built", []string{crash}, []string{builtMissing}},
		{"verified with every script", "verified", []string{ok, crash}, nil},
		{"verified missing the crash script", "verified", []string{ok}, []string{verifiedCrash}},
		{"verified missing every script", "verified", nil, []string{verifiedOK, verifiedCrash}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			env := testEnv()
			env.Scripts = func(_ flows.Flow, name string) bool { return slices.Contains(tc.scripts, name) }
			row := fundRowWith(func(c []string) { c[7] = "ok;crash:after-sign"; c[8] = tc.status })
			parsed, _ := flows.Parse(strings.NewReader(tsv(row)))
			if got := lines(flows.CheckScripts(parsed, env)); !slices.Equal(got, tc.want) {
				t.Fatalf("problems = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestScriptName_isTheFlowTestNameWithoutTheTestPrefix(t *testing.T) {
	t.Parallel()
	f := flows.Flow{ID: "00", Commands: []string{"RecordPing"}}
	for o, want := range map[flows.Outcome]string{
		"ok": "F00RecordPingOK", "InvalidInput": "F00RecordPingInvalidInput",
		"crash:after-publish": "F00RecordPingCrashAfterPublish",
	} {
		if got := flows.ScriptName(f, "RecordPing", o); got != want {
			t.Errorf("ScriptName(%s) = %s, want %s", o, got, want)
		}
	}
}

func TestMarkdown_rendersTheRFCTable(t *testing.T) {
	t.Parallel()
	dead := "27\tDead letters | advisory\tadmin\tconsumer:$JS.EVENT.ADVISORY\t\t\t\tok\tplanned\tdocs/flows.md"
	parsed, problems := flows.Parse(strings.NewReader(tsv(fundRow, dead)))
	if len(problems) != 0 {
		t.Fatalf("parse problems = %v", lines(problems))
	}
	want := "| # | Flow | Command / trigger | Events | Consumers |\n" +
		"| --- | --- | --- | --- | --- |\n" +
		"| 07 | Fund cabal | `FundCabal` on `POST /v1/cabals/{id}/fund` | `cabal.fund_submitted`, `cabal.funded` | " +
		"treasury.positions, ranking |\n" +
		"| 27 | Dead letters \\| advisory | `consumer:$JS.EVENT.ADVISORY` | none | none |\n"
	if got := flows.Markdown(parsed); got != want {
		t.Fatalf("markdown =\n%s\nwant\n%s", got, want)
	}
}

func TestMarkdown_listsEveryCommandOfAMultiCommandRow(t *testing.T) {
	t.Parallel()
	row := fundRowWith(func(c []string) { c[4], c[7] = "FundCabal;Refund", "ok" })
	parsed, _ := flows.Parse(strings.NewReader(tsv(row)))
	if got := flows.Markdown(
		parsed,
	); !strings.Contains(
		got,
		"| `FundCabal`, `Refund` on `POST /v1/cabals/{id}/fund` |",
	) {
		t.Fatalf("markdown =\n%s\nwant both commands in the trigger cell", got)
	}
	want := "- Command: `FundCabal`, `Refund`\n"
	tests := "| `ok` | `TestFlow07_FundCabal_OK` or `TestFlow07_Refund_OK` |"
	if got := flows.FeatureMap(parsed); !strings.Contains(got, want) || !strings.Contains(got, tests) {
		t.Fatalf("feature map =\n%s\nwant %q and %q", got, want, tests)
	}
}

func TestMarkdown_pairsEachCommandWithItsTrigger(t *testing.T) {
	t.Parallel()
	row := fundRowWith(func(c []string) {
		c[3], c[4], c[7] = "POST /v1/cabals/{id}/fund;DELETE /v1/cabals/{id}/fund", "FundCabal;Refund", "ok"
	})
	parsed, problems := flows.Parse(strings.NewReader(tsv(row)))
	if len(problems) != 0 {
		t.Fatalf("parse problems = %v", lines(problems))
	}
	cell := "| `FundCabal` on `POST /v1/cabals/{id}/fund`, `Refund` on `DELETE /v1/cabals/{id}/fund` |"
	if got := flows.Markdown(parsed); !strings.Contains(got, cell) {
		t.Fatalf("markdown =\n%s\nwant %q", got, cell)
	}
	trigger := "- Trigger: `POST /v1/cabals/{id}/fund`, `DELETE /v1/cabals/{id}/fund`\n"
	if got := flows.FeatureMap(parsed); !strings.Contains(got, trigger) {
		t.Fatalf("feature map =\n%s\nwant %q", got, trigger)
	}
}
