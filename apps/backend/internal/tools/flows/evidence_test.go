package flows_test

import (
	"slices"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/monaco/monaco/apps/backend/internal/errs"
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
		{"non-flow tests and subtests are ignored", fundRow, all + passed("TestFlowsCheck") + passed("TestFlow07_FundCabal_OK/sub"), nil},
		{
			"test for a deleted row", fundRow, all + passed("TestFlow09_Vote_OK"),
			[]string{"flows.tsv: test TestFlow09_Vote_OK matches no flow outcome; delete the test or add its row"},
		},
		{
			"test for a removed outcome", fundRow, all + `{"Action":"fail","Package":"p","Test":"TestFlow07_FundCabal_Paused"}`,
			[]string{"flows.tsv: test TestFlow07_FundCabal_Paused matches no flow outcome; delete the test or add its row"},
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

const (
	sha1 = "1111111111111111111111111111111111111111"
	sha2 = "2222222222222222222222222222222222222222"
)

func TestCheckEvidence_verifiedFlowsNeedAFreshStamp(t *testing.T) {
	t.Parallel()
	const path = "apps/backend/test/evidence/07.json"
	for _, tc := range []struct {
		name     string
		status   string
		evidence string
		fresh    flows.Fresh
		want     []string
	}{
		{"built needs no evidence", "built", "", nil, nil},
		{"fresh stamp", "verified", `{"sha":"` + sha1 + `","responses":[]}`, freshIf(sha1), nil},
		{
			"option-shaped stamp", "verified", `{"sha":"--output=/tmp/x"}`, nil,
			[]string{`flows.tsv:2: test/evidence/07.json sha stamp "--output=/tmp/x" is not a full git object id`},
		},
		{"missing file", "verified", "", nil, []string{"flows.tsv:2: verified flow has no test/evidence/07.json"}},
		{"no stamp", "verified", `{"responses":[]}`, nil, []string{"flows.tsv:2: test/evidence/07.json has no sha stamp"}},
		{"not json", "verified", `sha=abc`, nil, []string{"flows.tsv:2: test/evidence/07.json has no sha stamp"}},
		{
			"stale stamp", "verified", `{"sha":"` + sha2 + `"}`, freshIf(sha1),
			[]string{"flows.tsv:2: test/evidence/07.json stamp " + sha2 + " is older than the newest commit in internal/modules/treasury"},
		},
		{
			"history error", "verified", `{"sha":"` + sha1 + `"}`,
			func(string, string) (bool, error) { return false, errs.New(errs.CodeInternal, "git") },
			[]string{"flows.tsv:2: test/evidence/07.json stamp " + sha1 + ": git: internal"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			env := testEnv()
			repo, ok := env.Repo.(fstest.MapFS)
			if !ok {
				t.Fatal("testEnv repo is not a MapFS")
			}
			if tc.evidence != "" {
				repo[path] = &fstest.MapFile{Data: []byte(tc.evidence)}
			}
			env.Fresh = tc.fresh
			row := fundRowWith(func(c []string) { c[8] = tc.status })
			parsed, _ := flows.Parse(strings.NewReader(tsv(row)))
			if got := lines(flows.CheckEvidence(parsed, env)); !slices.Equal(got, tc.want) {
				t.Fatalf("problems = %q, want %q", got, tc.want)
			}
		})
	}
}

func freshIf(want string) flows.Fresh {
	return func(module, sha string) (bool, error) { return module == "treasury" && sha == want, nil }
}

func TestScriptName_isTheFlowTestNameWithoutTheTestPrefix(t *testing.T) {
	t.Parallel()
	f := flows.Flow{ID: "00", Command: "RecordPing"}
	for o, want := range map[flows.Outcome]string{
		"ok": "F00RecordPingOK", "InvalidInput": "F00RecordPingInvalidInput",
		"crash:after-publish": "F00RecordPingCrashAfterPublish",
	} {
		if got := flows.ScriptName(f, o); got != want {
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
