package flows

import (
	"net/http"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/testkit/scenario"
)

func blockPath(id string) string { return "/v1/users/" + id + "/block" }

const blockInvariants = `SELECT
	(SELECT count(*) FROM user_blocks WHERE blocker_id = $1::uuid AND blocked_id = $2::uuid),
	(SELECT count(*) FROM follows WHERE deleted_at IS NULL AND (
		(follower_id = $1::uuid AND followee_id = $2::uuid) OR (follower_id = $2::uuid AND followee_id = $1::uuid))),
	(SELECT count(*) FROM events WHERE type = 'block.created'
		AND payload->>'blocker_id' = $1::text AND payload->>'blocked_id' = $2::text),
	(SELECT count(*) FROM events WHERE type = 'block.removed'
		AND payload->>'blocker_id' = $1::text AND payload->>'blocked_id' = $2::text),
	(SELECT count(*) FROM events WHERE type IN ('block.created', 'block.removed')
		AND payload->>'blocker_id' = $1::text AND payload->>'blocked_id' = $2::text AND published_at IS NULL)`

func blockRows(s *scenario.Scenario) (blocks, follows, created, removed, unpublished int) {
	s.Helper()
	blocker, blocked := s.Recall("alice"), s.Recall("bob")
	if err := s.DB().QueryRow(s.Context(), blockInvariants, blocker, blocked).
		Scan(&blocks, &follows, &created, &removed, &unpublished); err != nil {
		s.Fatalf("flows: check block %s -> %s invariants: %v", blocker, blocked, err)
	}
	return blocks, follows, created, removed, unpublished
}

func blockRelayed() scenario.Step {
	return scenario.Eventually("all block events relayed", func(s *scenario.Scenario) bool {
		_, _, _, _, unpublished := blockRows(s)
		return unpublished == 0
	})
}

func blockHolds() scenario.Step {
	return func(s *scenario.Scenario) {
		s.Helper()
		blockRelayed()(s)
		blocks, follows, created, removed, unpublished := blockRows(s)
		if unpublished != 0 || blocks != created-removed || removed > created || (blocks == 1 && follows != 0) {
			s.Fatalf("flows: block has blocks/live follows/created/removed/unrelayed = %d/%d/%d/%d/%d",
				blocks, follows, created, removed, unpublished)
		}
	}
}

func blockRolledBack(follows int) scenario.Step {
	return func(s *scenario.Scenario) {
		s.Helper()
		blocks, live, created, _, _ := blockRows(s)
		var removed int
		if err := s.DB().QueryRow(s.Context(), `SELECT count(*) FROM events WHERE type = 'follow.removed'`).
			Scan(&removed); err != nil {
			s.Fatalf("flows: count follow.removed events: %v", err)
		}
		if blocks != 0 || live != follows || created != 0 || removed != 0 {
			s.Fatalf("flows: a crashed block left %d blocks, %d live follows, %d block.created and %d follow.removed",
				blocks, live, created, removed)
		}
	}
}

func followBothWays() []scenario.Step {
	return []scenario.Step{
		scenario.AsUser("alice"),
		scenario.Post(followPath("{bob}"), `{}`),
		scenario.ExpectStatus(http.StatusOK),
		scenario.AsUser("bob"),
		scenario.Post(followPath("{alice}"), `{}`),
		scenario.ExpectStatus(http.StatusOK),
		scenario.AsUser("alice"),
	}
}

func F20aBlockUserOK(s *scenario.Scenario) {
	s.Given(followUsers(), scenario.AsUser("alice")).
		When(append(followBothWays(),
			scenario.Post(blockPath("{bob}"), ``),
			scenario.ExpectStatus(http.StatusNoContent),
			scenario.Retry(),
			scenario.ExpectStatus(http.StatusNoContent),
			scenario.Post(blockPath("{bob}"), ``),
			scenario.ExpectStatus(http.StatusNoContent),
		)...).
		Then(
			scenario.ExpectEvents(events.TypeBlockCreated, 1),
			scenario.ExpectEvents(events.TypeFollowRemoved, 2),
			scenario.EventuallyPublished(events.TypeBlockCreated, 1),
			scenario.EventuallyPublished(events.TypeFollowRemoved, 2),
			blockHolds(),
		)
}

func F20aBlockUserCannotBlockSelf(s *scenario.Scenario) {
	s.Given(followUsers(), scenario.AsUser("alice")).
		When(scenario.Post(blockPath("{alice}"), ``)).
		Then(scenario.ExpectProblem(errs.CodeCannotBlockSelf), scenario.ExpectEvents(events.TypeBlockCreated, 0))
}

func F20aBlockUserUserNotFound(s *scenario.Scenario) {
	s.Given(followUsers(), scenario.AsUser("alice")).
		When(scenario.Post(blockPath(ids.Real{}.NewV7().String()), ``)).
		Then(scenario.ExpectProblem(errs.CodeUserNotFound), scenario.ExpectEvents(events.TypeBlockCreated, 0))
}

func F20aBlockUserUnauthorized(s *scenario.Scenario) {
	s.Given(followUsers(), scenario.Anonymous()).
		When(scenario.Post(blockPath("{bob}"), ``)).
		Then(scenario.ExpectProblem(errs.CodeUnauthorized), scenario.ExpectEvents(events.TypeBlockCreated, 0))
}

func F20aBlockUserCrashBeforeCommit(s *scenario.Scenario) {
	s.Given(followUsers(), scenario.AsUser("alice")).
		When(append(followBothWays(),
			scenario.Post(blockPath("{bob}"), ``),
			blockRolledBack(2),
			scenario.Retry(),
			scenario.ExpectStatus(http.StatusNoContent),
		)...).
		Then(
			scenario.ExpectEvents(events.TypeBlockCreated, 1),
			scenario.ExpectEvents(events.TypeFollowRemoved, 2),
			scenario.EventuallyPublished(events.TypeBlockCreated, 1),
			blockHolds(),
		)
}

func F20aUnblockUserOK(s *scenario.Scenario) {
	s.Given(followUsers(), scenario.AsUser("alice")).
		When(
			scenario.Post(blockPath("{bob}"), ``),
			scenario.ExpectStatus(http.StatusNoContent),
			scenario.Delete(blockPath("{bob}")),
			scenario.ExpectStatus(http.StatusNoContent),
			scenario.Delete(blockPath("{bob}")),
			scenario.ExpectStatus(http.StatusNoContent),
		).
		Then(
			scenario.ExpectEvents(events.TypeBlockCreated, 1),
			scenario.ExpectEvents(events.TypeBlockRemoved, 1),
			scenario.EventuallyPublished(events.TypeBlockRemoved, 1),
			blockHolds(),
		)
}

const reportsPath = "/v1/reports"

func reportBob() scenario.Step {
	return func(s *scenario.Scenario) {
		s.Helper()
		reportUserID(s.Recall("bob"))(s)
	}
}

func reportUserID(id string) scenario.Step {
	return scenario.Post(reportsPath, `{"kind":"user","target_id":"`+id+`","reason":"spam"}`)
}

func openReports(want int) scenario.Step {
	return func(s *scenario.Scenario) {
		s.Helper()
		var got int
		if err := s.DB().QueryRow(s.Context(),
			`SELECT count(*) FROM reports WHERE reporter_id = $1::uuid AND status = 'open'`, s.Recall("alice")).
			Scan(&got); err != nil {
			s.Fatalf("flows: count open reports: %v", err)
		}
		if got != want {
			s.Fatalf("flows: %d open reports, want %d", got, want)
		}
	}
}

func F20aCreateReportOK(s *scenario.Scenario) {
	s.Given(followUsers(), scenario.AsUser("alice")).
		When(
			reportBob(),
			scenario.ExpectStatus(http.StatusCreated),
			scenario.Remember("id", "report"),
			scenario.Replay(),
			reportBob(),
			scenario.ExpectStatus(http.StatusCreated),
			scenario.ExpectRemembered("id", "report"),
		).
		Then(
			openReports(1),
			scenario.ExpectEvents(events.TypeReportCreated, 1),
			scenario.EventuallyPublished(events.TypeReportCreated, 1),
		)
}

func F20aCreateReportReportTargetNotFound(s *scenario.Scenario) {
	s.Given(followUsers(), scenario.AsUser("alice")).
		When(reportUserID(ids.Real{}.NewV7().String())).
		Then(
			scenario.ExpectProblem(errs.CodeReportTargetNotFound),
			openReports(0),
			scenario.ExpectEvents(events.TypeReportCreated, 0),
		)
}

func F20aCreateReportRateLimited(s *scenario.Scenario) {
	steps := make([]scenario.Step, 0, 40)
	for range 20 {
		steps = append(steps, reportBob(), scenario.ExpectStatus(http.StatusCreated))
	}
	s.Given(followUsers(), scenario.AsUser("alice")).
		When(steps...).
		Then(reportBob(), scenario.ExpectProblem(errs.CodeRateLimited))
}
