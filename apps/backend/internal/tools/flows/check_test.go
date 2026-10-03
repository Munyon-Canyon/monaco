package flows_test

import (
	"slices"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/monaco/monaco/apps/backend/internal/tools/flows"
)

func set(values ...string) flows.Lookup {
	return func(_ flows.Flow, v string) bool { return slices.Contains(values, v) }
}

func testEnv() flows.Env {
	return flows.Env{
		Repo: fstest.MapFS{
			"apps/backend/internal/modules/treasury/app/fund.go": {Data: []byte("package app\n")},
			"apps/backend/internal/modules/README.md":            {Data: []byte("not a module\n")},
			"docs/flows.md": {Data: []byte("# Flows\n\n## Fund\n\n```md\n## Not a heading\n```\n")},
		},
		BackendDir:  "apps/backend",
		Events:      set("cabal.fund_submitted", "cabal.funded"),
		Codes:       set("InsufficientFunds"),
		Triggers:    set("POST /v1/cabals/{id}/fund"),
		Commands:    set("FundCabal"),
		Consumers:   set("treasury.positions", "ranking"),
		Faultpoints: set("after-sign"),
	}
}

func check(t *testing.T, body string, env flows.Env) []string {
	t.Helper()
	parsed, problems := flows.Parse(strings.NewReader(body))
	if len(problems) != 0 {
		t.Fatalf("parse problems = %v", lines(problems))
	}
	return lines(flows.CheckColumns(parsed, env, nil))
}

func fundRowWith(edit func(cells []string)) string {
	cells := strings.Split(fundRow, "\t")
	edit(cells)
	return strings.Join(cells, "\t")
}

func TestCheckColumns_eachColumnFailsWithOneLine(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		edit func(cells []string)
		want string
	}{
		{"bad code", func(c []string) { c[7] = "ok;NoSuchCode" }, "flows.tsv:2: outcome NoSuchCode is not an errs code name"},
		{"missing module", func(c []string) { c[2] = "ghost" }, "flows.tsv:2: module ghost has no directory under internal/modules"},
		{"module is a file", func(c []string) { c[2] = "README.md" }, "flows.tsv:2: module README.md has no directory under internal/modules"},
		{"unknown event", func(c []string) { c[5] = "cabal.funded;cabal.exploded" }, "flows.tsv:2: event cabal.exploded is not in the events registry"},
		{"unknown trigger", func(c []string) { c[3] = "GET /v1/nothing" }, "flows.tsv:2: trigger GET /v1/nothing is not a route, subject or poller"},
		{"unknown command", func(c []string) { c[4] = "Fund" }, "flows.tsv:2: command Fund is not a type in internal/modules/treasury/app"},
		{"unknown consumer", func(c []string) { c[6] = "ghost" }, "flows.tsv:2: consumer ghost is not a registered durable"},
		{"unknown faultpoint", func(c []string) { c[7] = "crash:after-lunch" }, "flows.tsv:2: outcome crash:after-lunch is not a registered faultpoint"},
		{"missing doc", func(c []string) { c[9] = "docs/nope.md#fund" }, "flows.tsv:2: doc docs/nope.md does not exist"},
		{"missing anchor", func(c []string) { c[9] = "docs/flows.md#not-a-heading" }, "flows.tsv:2: doc docs/flows.md has no heading with anchor #not-a-heading"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := check(t, tsv(fundRowWith(tc.edit)), testEnv())
			if !slices.Equal(got, []string{tc.want}) {
				t.Fatalf("problems = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestCheckColumns_validRowsPass(t *testing.T) {
	t.Parallel()
	docOnly := fundRowWith(func(c []string) { c[0], c[9] = "08", "docs/flows.md" })
	if got := check(t, tsv(fundRow, docOnly), testEnv()); len(got) != 0 {
		t.Fatalf("problems = %q", got)
	}
}

func TestCheckColumns_duplicateIDNamesTheFirstLine(t *testing.T) {
	t.Parallel()
	got := check(t, tsv(fundRow, fundRow), testEnv())
	if want := []string{"flows.tsv:3: id 07 already used on line 2"}; !slices.Equal(got, want) {
		t.Fatalf("problems = %q, want %q", got, want)
	}
}

func TestAnchors_followGitHubSlugRules(t *testing.T) {
	t.Parallel()
	md := strings.Join([]string{
		"# Backend platform (Go rewrite)",
		"### `flows.tsv` is the source of truth",
		"## Pull requests: small and stacked",
		"## [Linked](https://example.com) heading ##",
		"## Errors",
		"## Errors",
		"   ## Indented_Snake",
		"#NoSpace",
		"~~~",
		"## Fenced",
		"~~~",
		"    ## Code block",
	}, "\n")
	got := flows.Anchors(md)
	want := []string{
		"backend-platform-go-rewrite", "flowstsv-is-the-source-of-truth", "pull-requests-small-and-stacked",
		"linked-heading", "errors", "errors-1", "indented_snake",
	}
	keys := make([]string, 0, len(got))
	for k := range got {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	slices.Sort(want)
	if !slices.Equal(keys, want) {
		t.Fatalf("anchors = %q, want %q", keys, want)
	}
}
