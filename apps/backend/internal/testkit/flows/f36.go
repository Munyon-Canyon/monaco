package flows

import (
	"net/http"
	"strings"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/testkit/scenario"
)

const cabalOrderPath = "/v1/me/cabals/order"

func twoCabals(script string) []scenario.Step {
	return []scenario.Step{
		scenario.SignIn("did:privy:qa-f36-" + script + "-" + uuid.NewString()[:8]),
		scenario.Post(cabalsPath, cabalOf("open", "all")),
		scenario.ExpectStatus(http.StatusCreated),
		scenario.Remember("id", "first"),
		scenario.Post(cabalsPath, cabalOf("open", "all")),
		scenario.ExpectStatus(http.StatusCreated),
		scenario.Remember("id", "second"),
	}
}

func putOrder(names ...string) scenario.Step {
	return func(s *scenario.Scenario) {
		quoted := make([]string, len(names))
		for i, name := range names {
			quoted[i] = `"` + s.Recall(name) + `"`
		}
		scenario.Put(cabalOrderPath, `{"cabal_ids":[`+strings.Join(quoted, ",")+`]}`)(s)
	}
}

func sortIndexIs(name string, want int) scenario.Step {
	return func(s *scenario.Scenario) {
		var got *int
		if err := s.DB().QueryRow(s.Context(), `SELECT sort_index FROM cabal_members WHERE cabal_id = $1`,
			s.Recall(name)).Scan(&got); err != nil {
			s.Fatalf("flows: read the sort index of cabal %s: %v", s.Recall(name), err)
		}
		if got == nil || *got != want {
			s.Fatalf("flows: cabal %s sort_index = %v, want %d", s.Recall(name), got, want)
		}
	}
}

func F36ReorderCabalsOK(s *scenario.Scenario) {
	s.Given(twoCabals("ok")...).
		When(putOrder("second", "first"), scenario.ExpectStatus(http.StatusNoContent)).
		Then(sortIndexIs("second", 0), sortIndexIs("first", 1))
}

func F36ReorderCabalsInvalidInput(s *scenario.Scenario) {
	s.Given(twoCabals("invalid")...).
		When(putOrder("first", "first")).
		Then(scenario.ExpectProblem(errs.CodeInvalidInput))
}

func F36ReorderCabalsUnauthorized(s *scenario.Scenario) {
	s.Given(scenario.Anonymous()).
		When(scenario.Put(cabalOrderPath, `{"cabal_ids":[]}`)).
		Then(scenario.ExpectProblem(errs.CodeUnauthorized))
}
