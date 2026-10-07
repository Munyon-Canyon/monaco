package flows

import (
	"net/http"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/platform/bus"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
	"github.com/monaco/monaco/apps/backend/internal/testkit/fakes"
	"github.com/monaco/monaco/apps/backend/internal/testkit/scenario"
)

const (
	sinkPoller      = "admin.deadletters"
	letterBody      = `{"reason":"posthog fixed"}`
	liveLetterQuery = `SELECT id::text FROM dead_letters
		WHERE consumer = 'analytics' AND handler = 'analytics.posthog.follow.created' AND code = $1
		AND status = $2 AND occurrences = 1
		AND event_id = (SELECT id FROM events WHERE type = 'follow.created' AND payload->>'follower_id' = $3)`
)

func (defined) WorkerEnvF27() []string { return []string{"ADMIN_DEADLETTERS_INTERVAL=1s"} }

func (defined) AloneF27() []Script {
	return []Script{F27RecordDeadLetterOK, F27RedriveDeadLetterOK, F27DiscardDeadLetterOK}
}

func (defined) LettersF27() map[string][]Letter {
	refused := Letter{Subject: events.TypeFollowCreated, Code: errs.CodePostHogRejected}
	resolved := Letter{Subject: events.TypeFollowCreated, Code: errs.Code(bus.ResolvedCode)}
	return map[string][]Letter{
		"F27RecordDeadLetterOK":  {refused},
		"F27DiscardDeadLetterOK": {refused},
		"F27RedriveDeadLetterOK": {refused, resolved},
	}
}

func letterPath(id, action string) string { return "/v1/admin/dead-letters/" + id + "/" + action }

func postHogRefusesOneCapture() scenario.Step {
	return scenario.FakeUpstream(fakes.Step{
		Route: "/posthog/batch/", Action: fakes.ActionFail, Status: http.StatusBadRequest, Times: 1,
	})
}

func followedWhilePostHogRefuses() []scenario.Step {
	return []scenario.Step{
		followUsers(), postHogRefusesOneCapture(), scenario.AsUser("alice"),
		scenario.Post(followPath("{bob}"), `{}`), scenario.ExpectStatus(http.StatusOK),
	}
}

func recordedLetter(letter *string, status string) scenario.Step {
	return scenario.Eventually("the sink to record the "+status+" analytics letter", func(s *scenario.Scenario) bool {
		scenario.AwaitTick(sinkPoller)(s)
		rows, err := s.DB().
			Query(s.Context(), liveLetterQuery, string(errs.CodePostHogRejected), status, s.Recall("alice"))
		if err != nil {
			s.Fatalf("flows: read the dead letter: %v", err)
		}
		defer rows.Close()
		found := rows.Next()
		if found {
			if err := rows.Scan(letter); err != nil {
				s.Fatalf("flows: scan the dead letter: %v", err)
			}
		}
		return found
	})
}

func asOperator() []scenario.Step {
	return []scenario.Step{scenario.SeededAdmin("op", "operator"), scenario.AsUser("op")}
}

func postLetter(letter *string, action, body string) scenario.Step {
	return func(s *scenario.Scenario) { scenario.Post(letterPath(*letter, action), body)(s) }
}

func expectLetterRow(letter *string, status, reason string) scenario.Step {
	return func(s *scenario.Scenario) {
		var n int
		err := s.DB().QueryRow(s.Context(), `SELECT count(*) FROM dead_letters WHERE id = $1::uuid AND status = $2
			AND resolved_by = $3::uuid AND resolve_reason = $4`, *letter, status, s.Recall("op"), reason).Scan(&n)
		if err != nil || n != 1 {
			s.Fatalf("flows: dead letter %s is not %s by the operator with reason %q: %d rows (%v)",
				*letter, status, reason, n, err)
		}
	}
}

func expectNoAdminActions(letter *string) scenario.Step {
	return func(s *scenario.Scenario) {
		s.Helper()
		var n int
		err := s.DB().QueryRow(s.Context(), `SELECT count(*) FROM events WHERE type = $1
			AND (payload->>'admin_id' = $2 OR payload->>'target_id' = $3)`,
			string(events.TypeAdminAction), s.Recall("op"), *letter).Scan(&n)
		if err != nil || n != 0 {
			s.Fatalf("flows: %d admin.action events by the operator or on dead letter %s, want 0 (%v)", n, *letter, err)
		}
	}
}

func F27RecordDeadLetterOK(s *scenario.Scenario) {
	var letter string
	s.Given(followedWhilePostHogRefuses()...).
		Then(
			recordedLetter(&letter, "open"),
			scenario.EventuallyLog(observability.AdminDeadLetterRecorded,
				map[string]string{"consumer": "analytics", "status": "open"}),
		)
}

func F27RedriveDeadLetterOK(s *scenario.Scenario) {
	var letter string
	s.Given(followedWhilePostHogRefuses()...).When(recordedLetter(&letter, "open")).
		When(append(asOperator(), postLetter(&letter, "redrive", letterBody), scenario.ExpectStatus(http.StatusNoContent))...).
		Then(
			redrivenOrResolved(&letter),
			scenario.Eventually("the marker to resolve the letter", func(s *scenario.Scenario) bool {
				scenario.AwaitTick(sinkPoller)(s)
				return letterIs(s, letter, "resolved")
			}),
			scenario.EventuallyCapturedBy(events.TypeFollowCreated, "follow_created", "follower_id", s.Recall("alice")),
			expectNoAdminActions(&letter),
		)
}

func redrivenOrResolved(letter *string) scenario.Step {
	return func(s *scenario.Scenario) {
		var n int
		err := s.DB().QueryRow(s.Context(), `SELECT count(*) FROM dead_letters WHERE id = $1::uuid
			AND status IN ('redriven', 'resolved') AND redriven_at IS NOT NULL AND resolved_by = $2::uuid
			AND resolve_reason = 'posthog fixed'`, *letter, s.Recall("op")).Scan(&n)
		if err != nil || n != 1 {
			s.Fatalf("flows: dead letter %s was not redriven by the operator: %d rows (%v)", *letter, n, err)
		}
	}
}

func letterIs(s *scenario.Scenario, letter, status string) bool {
	var n int
	if err := s.DB().QueryRow(s.Context(), `SELECT count(*) FROM dead_letters WHERE id = $1::uuid AND status = $2`,
		letter, status).Scan(&n); err != nil {
		s.Fatalf("flows: read dead letter %s: %v", letter, err)
	}
	return n == 1
}

func F27DiscardDeadLetterOK(s *scenario.Scenario) {
	var letter string
	s.Given(followedWhilePostHogRefuses()...).When(recordedLetter(&letter, "open")).
		When(append(asOperator(), postLetter(&letter, "discard", letterBody), scenario.ExpectStatus(http.StatusNoContent))...).
		Then(expectLetterRow(&letter, "discarded", "posthog fixed"), expectNoAdminActions(&letter))
}

func F27RedriveDeadLetterDeadLetterNotOpen(s *scenario.Scenario) {
	var letter string
	s.Given(asOperator()...).Given(func(s *scenario.Scenario) {
		err := s.DB().QueryRow(s.Context(), `INSERT INTO dead_letters (id, stream_seq, consumer, code, letter, status,
			first_seen_at, last_seen_at) VALUES (gen_random_uuid(), 9000000001, 'analytics', 'post_hog_rejected', '{}',
			'discarded', now(), now()) RETURNING id::text`).Scan(&letter)
		if err != nil {
			s.Fatalf("flows: seed a discarded dead letter: %v", err)
		}
	}).When(postLetter(&letter, "redrive", letterBody), scenario.ExpectProblem(errs.CodeDeadLetterNotOpen))
}

func F27RedriveDeadLetterNotFound(s *scenario.Scenario) {
	missing := ids.Real{}.NewV7().String()
	s.Given(asOperator()...).
		When(scenario.Post(letterPath(missing, "redrive"), letterBody), scenario.ExpectProblem(errs.CodeNotFound))
}

func F27RedriveDeadLetterAdminForbidden(s *scenario.Scenario) {
	missing := ids.Real{}.NewV7().String()
	s.Given(scenario.SeededAdmin("mod", "moderator"), scenario.AsUser("mod")).
		When(scenario.Post(letterPath(missing, "redrive"), letterBody), scenario.ExpectProblem(errs.CodeAdminForbidden))
}
