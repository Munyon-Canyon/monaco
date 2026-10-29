package flows

import (
	"encoding/json"
	"net/http"

	"github.com/monaco/monaco/apps/backend/internal/testkit/scenario"
)

func (defined) SeedsF39() map[string]Seeder {
	return map[string]Seeder{"F39GetConfigOK": seedAnonymous}
}

func F39GetConfigOK(s *scenario.Scenario) {
	s.Given(scenario.Anonymous()).
		When(
			scenario.Get("/v1/app/config"),
			scenario.ExpectStatus(http.StatusOK),
			scenario.ExpectJSON("min_ios_build", 0),
			scenario.ExpectField("store_url", func(s *scenario.Scenario, raw json.RawMessage) {
				if len(raw) < 3 {
					s.Fatalf("scenario: store_url = %s, want a URL", raw)
				}
			}),
		)
}
