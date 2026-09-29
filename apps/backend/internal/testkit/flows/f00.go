package flows

import (
	"net/http"
	"strings"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/platform/faultpoint"
	"github.com/monaco/monaco/apps/backend/internal/testkit/scenario"
)

const pings = "/v1/system/pings"

func F00RecordPingOK(s *scenario.Scenario) {
	s.Given(scenario.AsUser("alice")).
		When(
			scenario.Post(pings, `{"note":"hi"}`),
			scenario.ExpectStatus(http.StatusCreated),
			scenario.ExpectJSON("echoed", false),
			scenario.Remember("id", "ping"),
			scenario.Replay(),
		).
		Then(f00Echoed()...)
}

func F00RecordPingInvalidInput(s *scenario.Scenario) {
	s.Given(scenario.AsUser("alice")).
		When(scenario.Post(pings, `{"note":"`+strings.Repeat("a", 141)+`"}`)).
		Then(scenario.ExpectProblem(errs.CodeInvalidInput), scenario.ExpectEvents(events.TypeSystemPinged, 0))
}

func F00RecordPingUnauthorized(s *scenario.Scenario) {
	s.Given(scenario.Anonymous()).
		When(scenario.Post(pings, `{"note":"hi"}`)).
		Then(scenario.ExpectProblem(errs.CodeUnauthorized), scenario.ExpectEvents(events.TypeSystemPinged, 0))
}

func F00RecordPingCrashAfterPublish(s *scenario.Scenario) {
	s.Given(scenario.AsUser("alice"), scenario.HoldRelay()).
		When(
			scenario.Post(pings, `{"note":"hi"}`),
			scenario.ExpectStatus(http.StatusCreated),
			scenario.Remember("id", "ping"),
			scenario.PublishCrashingAt(faultpoint.AfterPublish),
		).
		Then(f00Echoed()...)
}

func f00Echoed() []scenario.Step {
	return []scenario.Step{
		scenario.ExpectEvents(events.TypeSystemPinged, 1),
		scenario.EventuallyEvent(events.TypeSystemPinged),
		scenario.ExpectPublished(events.TypeSystemPinged, 1),
		scenario.EventuallyHint("ping_echoed"),
		scenario.Get(pings + "/{ping}"),
		scenario.ExpectStatus(http.StatusOK),
		scenario.ExpectJSON("echoed", true),
	}
}
