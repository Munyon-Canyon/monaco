package flows

import (
	"net/http"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/testkit/scenario"
)

const notificationPreferences = "/v1/me/notification-preferences"

func (defined) SeedsF32() map[string]Seeder {
	return map[string]Seeder{
		"F32SetNotificationPreferencesOK":                seedSignedIn,
		"F32SetNotificationPreferencesInvalidInput":      seedSignedIn,
		"F32SetNotificationPreferencesUnauthorized":      seedAnonymous,
		"F32SetNotificationPreferencesRateLimited":       seedSignedIn,
		"F32SetNotificationPreferencesCrashBeforeCommit": seedSignedIn,
	}
}

func F32SetNotificationPreferencesOK(s *scenario.Scenario) {
	s.Given(scenario.SeededUser("alice", "active"), scenario.AsUser("alice")).
		When(
			scenario.Get(notificationPreferences),
			scenario.ExpectStatus(http.StatusOK),
			scenario.ExpectJSON("decision_requests", true),
			scenario.ExpectJSON("replies_and_mentions", true),
			scenario.Put(notificationPreferences, `{"decision_requests":false,"replies_and_mentions":true}`),
			scenario.ExpectStatus(http.StatusOK),
			scenario.ExpectJSON("decision_requests", false),
			scenario.Replay(),
			scenario.Put(notificationPreferences, `{"decision_requests":false,"replies_and_mentions":false}`),
			scenario.ExpectStatus(http.StatusOK),
		).
		Then(
			scenario.Get(notificationPreferences),
			scenario.ExpectStatus(http.StatusOK),
			scenario.ExpectJSON("decision_requests", false),
			scenario.ExpectJSON("replies_and_mentions", false),
		)
}

func F32SetNotificationPreferencesInvalidInput(s *scenario.Scenario) {
	s.Given(scenario.SeededUser("alice", "active"), scenario.AsUser("alice")).
		When(
			scenario.Put(notificationPreferences, `{"decision_requests":false}`),
			scenario.ExpectProblem(errs.CodeInvalidInput),
			scenario.Put(notificationPreferences, `{"decision_requests":false,"replies_and_mentions":true,"nudges":true}`),
			scenario.ExpectProblem(errs.CodeInvalidInput),
		).
		Then(
			scenario.Get(notificationPreferences),
			scenario.ExpectJSON("decision_requests", true),
			scenario.ExpectJSON("replies_and_mentions", true),
		)
}

func F32SetNotificationPreferencesUnauthorized(s *scenario.Scenario) {
	s.Given(scenario.Anonymous()).
		When(scenario.Put(notificationPreferences, `{"decision_requests":false,"replies_and_mentions":true}`)).
		Then(scenario.ExpectProblem(errs.CodeUnauthorized))
}

func F32SetNotificationPreferencesRateLimited(s *scenario.Scenario) {
	steps := make([]scenario.Step, 0, 60)
	for range 30 {
		steps = append(steps,
			scenario.Put(notificationPreferences, `{"decision_requests":false,"replies_and_mentions":true}`),
			scenario.ExpectStatus(http.StatusOK))
	}
	s.Given(scenario.SeededUser("alice", "active"), scenario.AsUser("alice")).
		When(steps...).
		Then(
			bucketsDrained(),
			scenario.Put(notificationPreferences, `{"decision_requests":true,"replies_and_mentions":true}`),
			scenario.ExpectProblem(errs.CodeRateLimited),
		)
}

func F32SetNotificationPreferencesCrashBeforeCommit(s *scenario.Scenario) {
	s.Given(scenario.SeededUser("alice", "active"), scenario.AsUser("alice")).
		When(
			scenario.Put(notificationPreferences, `{"decision_requests":false,"replies_and_mentions":false}`),
			preferencesRolledBack(),
			scenario.Retry(),
			scenario.ExpectStatus(http.StatusOK),
		).
		Then(
			scenario.Get(notificationPreferences),
			scenario.ExpectJSON("decision_requests", false),
			scenario.ExpectJSON("replies_and_mentions", false),
		)
}

func bucketsDrained() scenario.Step {
	return func(s *scenario.Scenario) {
		s.Helper()
		_, err := s.DB().Exec(s.Context(),
			`UPDATE rate_limit_buckets SET tokens_milli = 0, updated_at = now() + interval '1 hour'`)
		if err != nil {
			s.Fatalf("flows: drain rate limit buckets: %v", err)
		}
	}
}

func preferencesRolledBack() scenario.Step {
	return func(s *scenario.Scenario) {
		s.Helper()
		var rows int
		err := s.DB().QueryRow(s.Context(), `SELECT count(*) FROM notification_preferences`).Scan(&rows)
		if err != nil {
			s.Fatalf("flows: count notification preferences: %v", err)
		}
		if rows != 0 {
			s.Fatalf("flows: a crashed save left %d preference rows, want none", rows)
		}
	}
}
