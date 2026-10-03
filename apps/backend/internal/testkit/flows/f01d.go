package flows

import (
	"net/http"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/testkit/scenario"
)

const skipPath = "/v1/me/onboarding/skip"

const (
	skipOK       = "did:privy:qa-onb-skip-ok"
	skipNoHandle = "did:privy:qa-onb-skip-nohandle"
	skipInvalid  = "did:privy:qa-onb-skip-invalid"
)

func F01dSkipOnboardingStepOK(s *scenario.Scenario) {
	s.Given(scenario.Anonymous()).
		When(append(withHandle(skipOK, "onb_skip_ok"),
			scenario.Post(skipPath, `{"step":"phone"}`),
			scenario.ExpectStatus(http.StatusOK),
			scenario.ExpectJSON("auth_state", "AWAITING_PHONE"),
			scenario.Post(skipPath, `{"step":"socials"}`),
			scenario.ExpectStatus(http.StatusOK),
			scenario.ExpectJSON("auth_state", "AWAITING_PHONE"),
		)...).
		Then(
			scenario.ExpectEvents(events.TypeUserAuthStateChanged, 1),
			scenario.ExpectEventPayload(events.TypeUserAuthStateChanged, map[string]any{
				"from": "CREATED", "to": "AWAITING_PHONE", "cause": "onboarding",
			}),
		)
}

func F01dSkipOnboardingStepHandleRequired(s *scenario.Scenario) {
	s.Given(scenario.Anonymous()).
		When(scenario.SignIn(skipNoHandle), scenario.ExpectStatus(http.StatusOK),
			scenario.Post(skipPath, `{"step":"phone"}`)).
		Then(scenario.ExpectProblem(errs.CodeHandleRequired), scenario.ExpectEvents(events.TypeUserAuthStateChanged, 0))
}

func F01dSkipOnboardingStepInvalidInput(s *scenario.Scenario) {
	s.Given(scenario.Anonymous()).
		When(append(withHandle(skipInvalid, "onb_skip_bad"), scenario.Post(skipPath, `{"step":"email"}`))...).
		Then(scenario.ExpectProblem(errs.CodeInvalidInput), scenario.ExpectEvents(events.TypeUserAuthStateChanged, 0))
}
