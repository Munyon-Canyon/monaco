package flows_test

import (
	"io"
	"slices"
	"strconv"
	"strings"
	"testing"
	"testing/iotest"

	"github.com/monaco/monaco/apps/backend/internal/tools/flows"
)

const fundRow = "07\tFund cabal\ttreasury\tPOST /v1/cabals/{id}/fund\tFundCabal\tcabal.fund_submitted; cabal.funded\t" +
	"treasury.positions;ranking\tok;InsufficientFunds;crash:after-sign\tbuilt\tdocs/flows.md#fund"

func tsv(rows ...string) string {
	return strings.Join(append([]string{flows.Header}, rows...), "\n") + "\n"
}

func lines(problems []flows.Problem) []string {
	out := make([]string, 0, len(problems))
	for _, p := range problems {
		out = append(out, p.String())
	}
	return out
}

func TestParse_headerOnlyFileHasNoFlowsAndNoProblems(t *testing.T) {
	t.Parallel()
	got, problems := flows.Parse(strings.NewReader(tsv()))
	if len(got) != 0 || len(problems) != 0 {
		t.Fatalf("flows=%v problems=%v", got, lines(problems))
	}
}

func TestParse_readsEveryColumnIntoATypedRow(t *testing.T) {
	t.Parallel()
	got, problems := flows.Parse(strings.NewReader(tsv(fundRow)))
	if len(problems) != 0 {
		t.Fatalf("problems = %v", lines(problems))
	}
	want := flows.Flow{
		Line: 2, ID: "07", Name: "Fund cabal", Module: "treasury", Trigger: "POST /v1/cabals/{id}/fund",
		Command: "FundCabal", Events: []string{"cabal.fund_submitted", "cabal.funded"},
		Consumers: []string{"treasury.positions", "ranking"},
		Outcomes:  []flows.Outcome{"ok", "InsufficientFunds", "crash:after-sign"},
		Status:    flows.StatusBuilt, Doc: "docs/flows.md#fund",
	}
	if len(got) != 1 || !equalFlow(got[0], want) {
		t.Fatalf("flows = %+v, want %+v", got, want)
	}
}

func equalFlow(a, b flows.Flow) bool {
	return a.Line == b.Line && a.ID == b.ID && a.Name == b.Name && a.Module == b.Module &&
		a.Trigger == b.Trigger && a.Command == b.Command && slices.Equal(a.Events, b.Events) &&
		slices.Equal(a.Consumers, b.Consumers) && slices.Equal(a.Outcomes, b.Outcomes) &&
		a.Status == b.Status && a.Doc == b.Doc
}

func TestParse_rejectsMalformedRowsWithTheirLineNumber(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		body string
		want []string
	}{
		{"empty file", "", []string{`flows.tsv:1: header must be "` + strings.ReplaceAll(flows.Header, "\t", `\t`) + `"`}},
		{"wrong header", "id\tflow\n", []string{`flows.tsv:1: header must be "` + strings.ReplaceAll(flows.Header, "\t", `\t`) + `"`}},
		{"short row", tsv("07\tFund cabal"), []string{"flows.tsv:2: has 2 columns, want 10"}},
		{"blank line", tsv(""), []string{"flows.tsv:2: blank line"}},
		{
			"bad cells",
			tsv("7a\t\t\t\tfundCabal\t\t\t\tdone\t"),
			[]string{
				`flows.tsv:2: id "7a" must be digits`,
				"flows.tsv:2: flow is empty",
				"flows.tsv:2: module is empty",
				"flows.tsv:2: doc is empty",
				`flows.tsv:2: status "done" is not planned, built or verified`,
				`flows.tsv:2: command "fundCabal" is not an exported Go identifier`,
				"flows.tsv:2: outcomes is empty",
			},
		},
		{
			"built flow without a command",
			tsv("07\tFund cabal\ttreasury\tt\t\t\t\tok;crash:After_Sign\tbuilt\tdocs/flows.md"),
			[]string{
				"flows.tsv:2: command is empty on a built flow",
				"flows.tsv:2: outcome crash:After_Sign must name a kebab-case crash point",
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, problems := flows.Parse(strings.NewReader(tc.body))
			if len(got) != 0 {
				t.Fatalf("flows = %+v, want none", got)
			}
			if !slices.Equal(lines(problems), tc.want) {
				t.Fatalf("problems =\n%s\nwant\n%s", strings.Join(lines(problems), "\n"), strings.Join(tc.want, "\n"))
			}
		})
	}
}

func TestTestName_namesOneTestPerOutcome(t *testing.T) {
	t.Parallel()
	f := flows.Flow{ID: "07", Command: "FundCabal"}
	for outcome, want := range map[flows.Outcome]string{
		"ok":                  "TestFlow07_FundCabal_OK",
		"InsufficientFunds":   "TestFlow07_FundCabal_InsufficientFunds",
		"crash:after-sign":    "TestFlow07_FundCabal_CrashAfterSign",
		"crash:before-commit": "TestFlow07_FundCabal_CrashBeforeCommit",
	} {
		if got := flows.TestName(f, outcome); got != want {
			t.Errorf("TestName(%s) = %s, want %s", outcome, got, want)
		}
	}
}

func TestParse_reportsAReadErrorAndTheMissingHeader(t *testing.T) {
	t.Parallel()
	parsed, problems := flows.Parse(iotest.ErrReader(io.ErrUnexpectedEOF))
	want := []string{
		"flows.tsv: read: unexpected EOF",
		"flows.tsv:1: header must be " + strconv.Quote(flows.Header),
	}
	if len(parsed) != 0 || !slices.Equal(lines(problems), want) {
		t.Fatalf("Parse = %v, %q; want no flows and %q", parsed, lines(problems), want)
	}
}
