package flows

import (
	"encoding/json"
	"net/http"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/platform/faultpoint"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/testkit/scenario"
)

const flagPath = "/v1/admin/system/pings/{ping}/flag"

func pingedBy(adminRole string) []scenario.Step {
	return []scenario.Step{
		scenario.SeededUser("pinger", "active"),
		scenario.SeededAdmin("admin", adminRole),
		scenario.AsUser("pinger"),
		scenario.Post(pings, `{"note":"flag me"}`),
		scenario.ExpectStatus(http.StatusCreated),
		scenario.Remember("id", "ping"),
		scenario.AsUser("admin"),
	}
}

func expectNothingFlagged() []scenario.Step {
	return []scenario.Step{
		scenario.ExpectEvents(events.TypeSystemPingFlagged, 0),
		scenario.ExpectEvents(events.TypeAdminAction, 0),
	}
}

func expectAudited(ping string) scenario.Step {
	return scenario.Eventually("one admin_actions row for ping "+ping, func(s *scenario.Scenario) bool {
		var rows int
		var ok bool
		err := s.DB().QueryRow(s.Context(), `SELECT count(*), coalesce(bool_and(reason = 'spam'
			AND before = '{"flagged_at": null}'::jsonb AND after->>'flagged_at' IS NOT NULL), false)
			FROM admin_actions WHERE action = 'ping_flag' AND target_type = 'system_ping' AND target_id = $1`,
			ping).Scan(&rows, &ok)
		if err != nil || rows > 1 {
			s.Fatalf("flows: admin_actions rows for ping %s = %d (%v), want one", ping, rows, err)
		}
		return rows == 1 && ok
	})
}

func expectListed(ping string) scenario.Step {
	return scenario.ExpectField("items", func(s *scenario.Scenario, raw json.RawMessage) {
		var items []struct {
			Action   string `json:"action"`
			Reason   string `json:"reason"`
			TargetID string `json:"target_id"`
		}
		if err := json.Unmarshal(raw, &items); err != nil || len(items) != 1 || items[0].Action != "ping_flag" ||
			items[0].Reason != "spam" || items[0].TargetID != ping {
			s.Fatalf("flows: GET /v1/admin/actions items = %s (%v), want the one ping_flag of ping %s", raw, err, ping)
		}
	})
}

func F26FlagPingOK(s *scenario.Scenario) {
	s.Given(pingedBy("moderator")...)
	ping := s.Recall("ping")
	s.When(
		scenario.Post(flagPath, `{"reason":"spam"}`),
		scenario.ExpectStatus(http.StatusNoContent),
	).Then(
		scenario.ExpectAdminAction(events.AdminActionPingFlag, ping),
		scenario.ExpectEvents(events.TypeSystemPingFlagged, 1),
		scenario.EventuallyPublished(events.TypeAdminAction, 1),
		expectAudited(ping),
		scenario.Get("/v1/admin/actions?target_type=system_ping&target_id={ping}"),
		scenario.ExpectStatus(http.StatusOK),
		expectListed(ping),
	)
}

func F26FlagPingReasonRequired(s *scenario.Scenario) {
	s.Given(pingedBy("moderator")...)
	s.When(
		scenario.Post(flagPath, `{"reason":"  "}`),
		scenario.ExpectProblem(errs.CodeReasonRequired),
	).Then(expectNothingFlagged()...)
}

func F26FlagPingAdminForbidden(s *scenario.Scenario) {
	s.Given(pingedBy("viewer")...)
	s.When(
		scenario.Post(flagPath, `{"reason":"spam"}`),
		scenario.ExpectProblem(errs.CodeAdminForbidden),
	).Then(expectNothingFlagged()...)
}

func F26FlagPingAlreadyFlagged(s *scenario.Scenario) {
	s.Given(pingedBy("moderator")...)
	s.When(
		scenario.Post(flagPath, `{"reason":"spam"}`),
		scenario.ExpectStatus(http.StatusNoContent),
		scenario.Post(flagPath, `{"reason":"again"}`),
		scenario.ExpectStatus(http.StatusConflict),
		scenario.ExpectProblem(errs.CodeAlreadyFlagged),
	).Then(
		scenario.ExpectEvents(events.TypeSystemPingFlagged, 1),
		scenario.ExpectEvents(events.TypeAdminAction, 1),
	)
}

func F26FlagPingNotFound(s *scenario.Scenario) {
	missing := "/v1/admin/system/pings/" + ids.Real{}.NewV7().String() + "/flag"
	s.Given(scenario.SeededAdmin("admin", "moderator"), scenario.AsUser("admin"))
	s.When(
		scenario.Post(missing, `{"reason":"spam"}`),
		scenario.ExpectProblem(errs.CodeNotFound),
	).Then(expectNothingFlagged()...)
}

func F26FlagPingCrashAfterPublish(s *scenario.Scenario) {
	s.Given(append([]scenario.Step{scenario.HoldRelay()}, pingedBy("moderator")...)...)
	ping := s.Recall("ping")
	s.When(
		scenario.Post(flagPath, `{"reason":"spam"}`),
		scenario.ExpectStatus(http.StatusNoContent),
		scenario.PublishCrashingAt(faultpoint.AfterPublish),
	).Then(
		scenario.ExpectAdminAction(events.AdminActionPingFlag, ping),
		scenario.EventuallyPublished(events.TypeAdminAction, 1),
		expectAudited(ping),
	)
}
