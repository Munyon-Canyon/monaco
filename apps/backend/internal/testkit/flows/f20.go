package flows

import (
	"net/http"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/testkit/scenario"
)

func followPath(id string) string { return "/v1/users/" + id + "/follow" }

func followUsers() scenario.Step {
	seed := []scenario.Step{
		scenario.SeededUser("alice", "active"),
		scenario.SeededUser("bob", "active"),
		scenario.SeededUser("mallory", "banned"),
	}
	return func(s *scenario.Scenario) {
		for _, step := range seed {
			step(s)
		}
	}
}

func F20FollowOK(s *scenario.Scenario) {
	s.Given(followUsers(), scenario.AsUser("alice")).
		When(
			scenario.Post(followPath("{bob}"), `{}`),
			scenario.ExpectStatus(http.StatusOK),
			scenario.ExpectJSON("following", true),
			scenario.Replay(),
			scenario.Post(followPath("{bob}"), `{"source":"feed"}`),
			scenario.ExpectStatus(http.StatusOK),
			scenario.ExpectJSON("following", true),
		).
		Then(
			scenario.ExpectEvents(events.TypeFollowCreated, 1),
			scenario.EventuallyPublished(events.TypeFollowCreated, 1),
		)
}

func F20FollowCannotFollowSelf(s *scenario.Scenario) {
	s.Given(followUsers(), scenario.AsUser("alice")).
		When(scenario.Post(followPath("{alice}"), `{}`)).
		Then(scenario.ExpectProblem(errs.CodeCannotFollowSelf), scenario.ExpectEvents(events.TypeFollowCreated, 0))
}

func F20FollowUserNotFound(s *scenario.Scenario) {
	s.Given(followUsers(), scenario.AsUser("alice")).
		When(scenario.Post(followPath(ids.Real{}.NewV7().String()), `{}`)).
		Then(scenario.ExpectProblem(errs.CodeUserNotFound), scenario.ExpectEvents(events.TypeFollowCreated, 0))
}

func F20FollowUserBanned(s *scenario.Scenario) {
	s.Given(followUsers(), scenario.AsUser("alice")).
		When(scenario.Post(followPath("{mallory}"), `{}`)).
		Then(scenario.ExpectProblem(errs.CodeUserBanned), scenario.ExpectEvents(events.TypeFollowCreated, 0))
}

func F20FollowUnauthorized(s *scenario.Scenario) {
	s.Given(followUsers(), scenario.Anonymous()).
		When(scenario.Post(followPath("{bob}"), `{}`)).
		Then(scenario.ExpectProblem(errs.CodeUnauthorized), scenario.ExpectEvents(events.TypeFollowCreated, 0))
}

func F20FollowCrashBeforeCommit(s *scenario.Scenario) {
	s.Given(followUsers(), scenario.AsUser("alice")).
		When(
			scenario.Post(followPath("{bob}"), `{}`),
			scenario.Post(followPath("{bob}"), `{}`),
			scenario.ExpectStatus(http.StatusOK),
			scenario.ExpectJSON("following", true),
		).
		Then(
			scenario.ExpectEvents(events.TypeFollowCreated, 1),
			scenario.EventuallyPublished(events.TypeFollowCreated, 1),
		)
}

func F20UnfollowOK(s *scenario.Scenario) {
	s.Given(followUsers(), scenario.AsUser("alice")).
		When(
			scenario.Post(followPath("{bob}"), `{}`),
			scenario.Delete(followPath("{bob}")),
			scenario.ExpectStatus(http.StatusOK),
			scenario.ExpectJSON("following", false),
			scenario.Delete(followPath("{bob}")),
			scenario.ExpectStatus(http.StatusOK),
			scenario.ExpectJSON("following", false),
			scenario.Post(followPath("{bob}"), `{}`),
			scenario.ExpectJSON("following", true),
		).
		Then(
			scenario.ExpectEvents(events.TypeFollowCreated, 2),
			scenario.ExpectEvents(events.TypeFollowRemoved, 1),
			scenario.EventuallyPublished(events.TypeFollowRemoved, 1),
		)
}
