package flows

import (
	"net/http"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/testkit/scenario"
)

const linkSocialsPath = "/v1/me/onboarding/socials"

const (
	xOK       = "did:privy:qa-onb-x-ok"
	xNoHandle = "did:privy:qa-onb-x-nohandle"
	xNone     = "did:privy:qa-onb-x-none"
	xDown     = "did:privy:qa-onb-x-down"
)

func F01cLinkSocialsOK(s *scenario.Scenario) {
	s.Given(scenario.Anonymous()).
		When(append(withHandle(xOK, "onb_x_ok"),
			scenario.Post(linkSocialsPath, `{"x_user_id":"spoofed"}`),
			scenario.ExpectStatus(http.StatusOK),
			scenario.ExpectJSON("auth_state", "AWAITING_PHONE"),
			scenario.ExpectJSON("x_username", "onb_x_ok"),
		)...).
		Then(
			scenario.ExpectEvents(events.TypeUserAuthStateChanged, 1),
			scenario.ExpectEventPayload(events.TypeUserAuthStateChanged, map[string]any{
				"from": "CREATED", "to": "AWAITING_PHONE", "cause": "onboarding",
			}),
			onboardingAdvanced("CREATED", "AWAITING_PHONE"),
		)
}

func F01cLinkSocialsHandleRequired(s *scenario.Scenario) {
	s.Given(scenario.Anonymous()).
		When(scenario.SignIn(xNoHandle), scenario.ExpectStatus(http.StatusOK), scenario.Post(linkSocialsPath, ``)).
		Then(scenario.ExpectProblem(errs.CodeHandleRequired), scenario.ExpectEvents(events.TypeUserAuthStateChanged, 0))
}

func F01cLinkSocialsXNotLinked(s *scenario.Scenario) {
	s.Given(scenario.Anonymous()).
		When(append(withHandle(xNone, "onb_x_none"), scenario.Post(linkSocialsPath, ``))...).
		Then(scenario.ExpectProblem(errs.CodeXNotLinked), scenario.ExpectEvents(events.TypeUserAuthStateChanged, 0))
}

func F01cLinkSocialsPrivyUnavailable(s *scenario.Scenario) {
	s.Given(scenario.Anonymous()).
		When(append(withHandle(xDown, "onb_x_down"), privyDown(xDown), scenario.Post(linkSocialsPath, ``))...).
		Then(scenario.ExpectProblem(errs.CodePrivyUnavailable), scenario.ExpectEvents(events.TypeUserAuthStateChanged, 0))
}
