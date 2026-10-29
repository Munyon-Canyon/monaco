package flows

import (
	"net/http"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/testkit/scenario"
)

const closeReminderPoller = "governance.close_reminders"

func reminderPath(p openProposal) string { return p.path + "/reminder" }

func F10bSetReminderOK(s *scenario.Scenario) {
	p := seedOpenProposal(s, 3)
	s.Given(scenario.AsSeededUser("alice", p.voters[0])).
		When(
			scenario.Put(reminderPath(p), ""),
			scenario.ExpectStatus(http.StatusOK),
			scenario.ExpectJSON("proposal_id", p.id.String()),
			scenario.ExpectJSON("reminder_set", true),
			scenario.Replay(),
			scenario.Get(p.path),
			scenario.ExpectJSON("reminder_set", true),
		).
		Then(scenario.ExpectEvents(events.TypeProposalCloseReminderDue, 0))
}

func F10bSetReminderUnauthorized(s *scenario.Scenario) {
	p := seedOpenProposal(s, 1)
	s.Given(scenario.Anonymous()).
		When(scenario.Put(reminderPath(p), "")).
		Then(scenario.ExpectProblem(errs.CodeUnauthorized))
}

func F10bSetReminderProposalNotFound(s *scenario.Scenario) {
	s.Given(scenario.AsUser("alice")).
		When(scenario.Put("/v1/proposals/"+ids.Real{}.NewV7().String()+"/reminder", "")).
		Then(scenario.ExpectProblem(errs.CodeProposalNotFound))
}

func F10bSetReminderNotAVoter(s *scenario.Scenario) {
	p := seedOpenProposal(s, 1)
	s.Given(scenario.AsUser("mallory")).
		When(scenario.Put(reminderPath(p), "")).
		Then(scenario.ExpectProblem(errs.CodeNotAVoter))
}

func F10bSetReminderProposalClosed(s *scenario.Scenario) {
	p := seedOpenProposal(s, 1)
	s.Given(scenario.AsSeededUser("alice", p.voters[0])).
		When(
			scenario.Post(p.votes, no),
			scenario.ExpectJSON("status", "failed"),
			scenario.Put(reminderPath(p), ""),
		).
		Then(scenario.ExpectProblem(errs.CodeProposalClosed))
}

func F10bClearReminderOK(s *scenario.Scenario) {
	p := seedOpenProposal(s, 3)
	s.Given(scenario.AsSeededUser("alice", p.voters[0])).
		When(
			scenario.Put(reminderPath(p), ""),
			scenario.ExpectJSON("reminder_set", true),
			scenario.Delete(reminderPath(p)),
			scenario.ExpectStatus(http.StatusOK),
			scenario.ExpectJSON("reminder_set", false),
			scenario.Get(p.path),
			scenario.ExpectJSON("reminder_set", false),
		).
		Then(scenario.ExpectEvents(events.TypeProposalCloseReminderDue, 0))
}

func F10bClearReminderUnauthorized(s *scenario.Scenario) {
	p := seedOpenProposal(s, 1)
	s.Given(scenario.Anonymous()).
		When(scenario.Delete(reminderPath(p))).
		Then(scenario.ExpectProblem(errs.CodeUnauthorized))
}

func F10bClearReminderProposalNotFound(s *scenario.Scenario) {
	s.Given(scenario.AsUser("alice")).
		When(scenario.Delete("/v1/proposals/" + ids.Real{}.NewV7().String() + "/reminder")).
		Then(scenario.ExpectProblem(errs.CodeProposalNotFound))
}

func F10bCloseReminderPollerOK(s *scenario.Scenario) {
	p := seedOpenProposal(s, 3)
	s.Given(scenario.AsSeededUser("alice", p.voters[0])).
		When(
			scenario.Put(reminderPath(p), ""),
			scenario.ExpectJSON("reminder_set", true),
			scenario.AsSeededUser("bob", p.voters[1]),
			scenario.Post(p.votes, yes),
			dueReminderNow(p, p.voters[0]),
			scenario.AwaitTick(closeReminderPoller),
			scenario.ExpectTick(closeReminderPoller, 1, 1),
		).
		Then(
			scenario.ExpectEvents(events.TypeProposalCloseReminderDue, 1),
			scenario.ExpectEventPayload(events.TypeProposalCloseReminderDue, map[string]any{
				"v": 1, "proposal_id": p.id.String(), "cabal_id": p.cabalID.String(),
				"user_id": p.voters[0].String(), "kind": "buy", "symbol": "AAPLx",
				"yes": 1, "no": 0, "voters": 3,
			}),
		)
}

func dueReminderNow(p openProposal, user ids.UserID) scenario.Step {
	return func(s *scenario.Scenario) {
		s.Helper()
		if _, err := s.DB().Exec(s.Context(),
			`UPDATE proposal_close_reminders SET due_at = now() - interval '1 minute'
			WHERE proposal_id = $1 AND user_id = $2`, p.id.UUID(), user.UUID()); err != nil {
			s.Fatalf("flows: make the reminder due: %v", err)
		}
	}
}
