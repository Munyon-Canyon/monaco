package flows

import (
	"net/http"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/testkit/fakes"
	"github.com/monaco/monaco/apps/backend/internal/testkit/scenario"
)

const linkPhonePath = "/v1/me/onboarding/phone"

const (
	phoneOK       = "did:privy:qa-onb-phone-ok"
	phoneNoHandle = "did:privy:qa-onb-phone-nohandle"
	phoneNone     = "did:privy:qa-onb-phone-none"
	phoneDown     = "did:privy:qa-onb-phone-down"
)

func withHandle(sub, handle string) []scenario.Step {
	return []scenario.Step{
		scenario.SignIn(sub),
		scenario.ExpectStatus(http.StatusOK),
		scenario.Put(setHandlePath, `{"handle":"`+handle+`"}`),
		scenario.ExpectStatus(http.StatusOK),
	}
}

func privyDown(sub string) scenario.Step {
	return scenario.FakeUpstream(fakes.Step{
		Route: privyUserPath + sub, Action: fakes.ActionFail, Status: http.StatusServiceUnavailable,
		Times: privyAttempts,
	})
}

func F01bLinkPhoneOK(s *scenario.Scenario) {
	s.Given(scenario.Anonymous()).
		When(append(withHandle(phoneOK, "onb_phone_ok"),
			scenario.Post(linkPhonePath, `{"phone":"+15550000000"}`),
			scenario.ExpectStatus(http.StatusOK),
			scenario.ExpectJSON("auth_state", "AWAITING_SOCIALS"),
			scenario.ExpectJSON("phone_linked", true),
		)...).
		Then(
			scenario.ExpectEvents(events.TypeUserAuthStateChanged, 1),
			scenario.ExpectEventPayload(events.TypeUserAuthStateChanged, map[string]any{
				"from": "CREATED", "to": "AWAITING_SOCIALS", "cause": "onboarding",
			}),
		)
}

func F01bLinkPhoneHandleRequired(s *scenario.Scenario) {
	s.Given(scenario.Anonymous()).
		When(scenario.SignIn(phoneNoHandle), scenario.ExpectStatus(http.StatusOK), scenario.Post(linkPhonePath, ``)).
		Then(scenario.ExpectProblem(errs.CodeHandleRequired), scenario.ExpectEvents(events.TypeUserAuthStateChanged, 0))
}

func F01bLinkPhonePhoneNotLinked(s *scenario.Scenario) {
	s.Given(scenario.Anonymous()).
		When(append(withHandle(phoneNone, "onb_phone_none"), scenario.Post(linkPhonePath, ``))...).
		Then(scenario.ExpectProblem(errs.CodePhoneNotLinked), scenario.ExpectEvents(events.TypeUserAuthStateChanged, 0))
}

func F01bLinkPhonePrivyUnavailable(s *scenario.Scenario) {
	s.Given(scenario.Anonymous()).
		When(append(withHandle(phoneDown, "onb_phone_down"), privyDown(phoneDown), scenario.Post(linkPhonePath, ``))...).
		Then(scenario.ExpectProblem(errs.CodePrivyUnavailable), scenario.ExpectEvents(events.TypeUserAuthStateChanged, 0))
}
