package flows

import (
	"encoding/json"
	"net/http"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
	"github.com/monaco/monaco/apps/backend/internal/testkit/scenario"
)

const (
	catchUp      = "/v1/me/catch-up"
	catchUpCount = "/v1/me/catch-up/count"
)

func (defined) SeedsF34() map[string]Seeder {
	return map[string]Seeder{
		"F34ReadCatchUpOK":           seedSignedIn,
		"F34ReadCatchUpUnauthorized": seedAnonymous,
	}
}

func F34ReadCatchUpOK(s *scenario.Scenario) {
	member := testkit.SeedUser(seedT{s}, s.DB(), testkit.UserOpts{})
	testkit.NewCabal(seedT{s}, s.DB(), testkit.WithCreator(member.ID))
	s.Given(scenario.AsSeededUser("member", member.ID)).
		When(
			scenario.Get(catchUp),
			scenario.ExpectStatus(http.StatusOK),
			scenario.ExpectJSON("count", 0),
			scenario.ExpectJSON("waiting", []any{}),
			scenario.ExpectJSON("replies", []any{}),
			scenario.ExpectJSON("invites", []any{}),
			scenario.ExpectJSON("results", []any{}),
			scenario.ExpectJSON("cleared", []any{}),
			scenario.ExpectField("cabals", func(s *scenario.Scenario, raw json.RawMessage) {
				var cabals []struct {
					ID string `json:"id"`
				}
				if err := json.Unmarshal(raw, &cabals); err != nil || len(cabals) != 1 {
					s.Fatalf("flows: catch up cabals = %s, want the one cabal alice belongs to", raw)
				}
			}),
		).
		Then(
			scenario.Get(catchUpCount),
			scenario.ExpectStatus(http.StatusOK),
			scenario.ExpectJSON("count", 0),
		)
}

func F34ReadCatchUpUnauthorized(s *scenario.Scenario) {
	s.Given(scenario.Anonymous()).
		When(scenario.Get(catchUp)).
		Then(scenario.ExpectProblem(errs.CodeUnauthorized))
}
