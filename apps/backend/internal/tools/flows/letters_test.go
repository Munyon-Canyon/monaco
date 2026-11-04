package flows_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/tools/flows"
)

func TestCheckLetterConsumers_isSilentWhenTheEnvCannotCountLetters(t *testing.T) {
	t.Parallel()
	parsed, _ := flows.Parse(fundFile, strings.NewReader(tsv(fundRowWith(func(c []string) { c[8] = "built" }))))
	if got := flows.CheckLetterConsumers(parsed, testEnv(), nil); len(got) != 0 {
		t.Fatalf("problems = %v, want none", got)
	}
}

func TestCheckLetterConsumers_aBuiltOrVerifiedFlowNamesTheConsumerOfEachDeclaredLetter(t *testing.T) {
	t.Parallel()
	unnamed := fundFile + ":2: script F07FundCabalOK declares 2 dead letters without a consumer; " +
		"set Consumer on each Letter in internal/testkit/flows so monacoctl verify compares which consumer wrote it"
	for _, tc := range []struct {
		name, status string
		unstated     map[string]int
		want         []string
	}{
		{"built with unnamed consumers", "built", map[string]int{"F07FundCabalOK": 2}, []string{unnamed}},
		{"verified with unnamed consumers", "verified", map[string]int{"F07FundCabalOK": 2}, []string{unnamed}},
		{"built with every consumer named", "built", map[string]int{"F07FundCabalOK": 0}, nil},
		{"built with no letters", "built", nil, nil},
		{"planned is not checked", "planned", map[string]int{"F07FundCabalOK": 2}, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			env := testEnv()
			env.UnstatedLetters = func(name string) int { return tc.unstated[name] }
			row := fundRowWith(func(c []string) { c[8] = tc.status })
			parsed, _ := flows.Parse(fundFile, strings.NewReader(tsv(row)))
			if got := lines(flows.CheckLetterConsumers(parsed, env, nil)); !slices.Equal(got, tc.want) {
				t.Fatalf("problems = %q, want %q", got, tc.want)
			}
		})
	}
}
