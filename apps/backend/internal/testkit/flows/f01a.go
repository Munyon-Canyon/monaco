package flows

import (
	"net/http"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/testkit/scenario"
)

const setHandlePath = "/v1/me/handle"

const (
	handleOK       = "did:privy:qa-handle-ok"
	handleInvalid  = "did:privy:qa-handle-invalid"
	handleReserved = "did:privy:qa-handle-reserved"
	handleTaken    = "did:privy:qa-handle-taken"
	handleHolder   = "did:privy:qa-handle-holder"
	handleSoon     = "did:privy:qa-handle-soon"
)

func F01aSetHandleOK(s *scenario.Scenario) {
	s.Given(scenario.Anonymous()).
		When(scenario.SignIn(handleOK), scenario.Put(setHandlePath, `{"handle":"qa_one"}`), scenario.ExpectStatus(http.StatusOK), scenario.ExpectJSON("handle", "qa_one")).
		Then(
			scenario.ExpectEvents(events.TypeUserProfileUpdated, 1),
			scenario.ExpectEventPayload(events.TypeUserProfileUpdated, map[string]any{
				"fields": []any{"handle"}, "handle": "qa_one",
			}),
		)
}

func F01aSetHandleHandleInvalid(s *scenario.Scenario) {
	s.Given(scenario.Anonymous()).
		When(scenario.SignIn(handleInvalid), scenario.Put(setHandlePath, `{"handle":"x"}`)).
		Then(scenario.ExpectProblem(errs.CodeHandleInvalid))
}

func F01aSetHandleHandleReserved(s *scenario.Scenario) {
	s.Given(scenario.Anonymous()).
		When(scenario.SignIn(handleReserved), scenario.Put(setHandlePath, `{"handle":"admin"}`)).
		Then(scenario.ExpectProblem(errs.CodeHandleReserved))
}

func F01aSetHandleHandleTaken(s *scenario.Scenario) {
	s.Given(scenario.Anonymous()).When(
		scenario.SignIn(handleHolder),
		scenario.ExpectStatus(http.StatusOK),
		scenario.Put(setHandlePath, `{"handle":"qa_taken"}`),
		scenario.ExpectStatus(http.StatusOK),
		scenario.SignIn(handleTaken),
		scenario.ExpectStatus(http.StatusOK),
		scenario.Put(setHandlePath, `{"handle":"qa_taken"}`),
	).Then(scenario.ExpectProblem(errs.CodeHandleTaken))
}

func F01aSetHandleHandleTooSoon(s *scenario.Scenario) {
	s.Given(scenario.Anonymous()).When(
		scenario.SignIn(handleSoon),
		scenario.ExpectStatus(http.StatusOK),
		scenario.Put(setHandlePath, `{"handle":"qa_first"}`),
		scenario.ExpectStatus(http.StatusOK),
		scenario.Put(setHandlePath, `{"handle":"qa_second"}`),
	).Then(scenario.ExpectProblem(errs.CodeHandleTooSoon))
}
