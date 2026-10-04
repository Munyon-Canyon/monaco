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

const followInvariants = `SELECT
	(SELECT count(*) FROM follows WHERE follower_id = $1::uuid AND followee_id = $2::uuid AND deleted_at IS NULL),
	(SELECT count(*) FROM events WHERE type = 'follow.created' AND payload->>'follower_id' = $1::text AND payload->>'followee_id' = $2::text),
	(SELECT count(*) FROM events WHERE type = 'follow.removed' AND payload->>'follower_id' = $1::text AND payload->>'followee_id' = $2::text),
	(SELECT count(*) FROM events WHERE type IN ('follow.created', 'follow.removed') AND payload->>'follower_id' = $1::text AND payload->>'followee_id' = $2::text AND published_at IS NULL)`

func followRelayed() scenario.Step {
	return scenario.Eventually("all follow events relayed", func(s *scenario.Scenario) bool {
		follower, followee := s.Recall("alice"), s.Recall("bob")
		var unpublished int
		if err := s.DB().QueryRow(s.Context(), `SELECT count(*) FROM events
			WHERE type IN ('follow.created', 'follow.removed')
			AND payload->>'follower_id' = $1::text AND payload->>'followee_id' = $2::text
			AND published_at IS NULL`, follower, followee).Scan(&unpublished); err != nil {
			s.Fatalf("flows: count unrelayed follow events: %v", err)
		}
		return unpublished == 0
	})
}

func followHolds() scenario.Step {
	return func(s *scenario.Scenario) {
		follower, followee := s.Recall("alice"), s.Recall("bob")
		followRelayed()(s)
		var live, created, removed, unpublished int
		if err := s.DB().QueryRow(s.Context(), followInvariants, follower, followee).
			Scan(&live, &created, &removed, &unpublished); err != nil {
			s.Fatalf("flows: check follow %s -> %s invariants: %v", follower, followee, err)
		}
		if unpublished != 0 {
			s.Fatalf("flows: follow %s -> %s has %d unrelayed events", follower, followee, unpublished)
		}
		if (live == 0 && created == 0 && removed == 0) ||
			(live == 1 && created == 1 && removed == 0) ||
			(live == 1 && created == 2 && removed == 1) {
			return
		}
		s.Fatalf("flows: follow %s -> %s has live/created/removed = %d/%d/%d",
			follower, followee, live, created, removed)
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
			followHolds(),
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
			followHolds(),
			scenario.Retry(),
			scenario.ExpectStatus(http.StatusOK),
			scenario.ExpectJSON("following", true),
		).
		Then(
			scenario.ExpectEvents(events.TypeFollowCreated, 1),
			scenario.EventuallyPublished(events.TypeFollowCreated, 1),
			followHolds(),
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
			scenario.EventuallyPublished(events.TypeFollowCreated, 2),
			scenario.EventuallyPublished(events.TypeFollowRemoved, 1),
			followHolds(),
		)
}
