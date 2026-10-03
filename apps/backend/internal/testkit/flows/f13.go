package flows

import (
	"net/http"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/platform/faultpoint"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/testkit/scenario"
)

func f13Scripts() map[string]Script {
	return map[string]Script{
		"F13WithdrawProposalOK":                 F13WithdrawProposalOK,
		"F13WithdrawProposalUnauthorized":       F13WithdrawProposalUnauthorized,
		"F13WithdrawProposalProposalNotFound":   F13WithdrawProposalProposalNotFound,
		"F13WithdrawProposalNotProposer":        F13WithdrawProposalNotProposer,
		"F13WithdrawProposalProposalClosed":     F13WithdrawProposalProposalClosed,
		"F13WithdrawProposalWithdrawNotAllowed": F13WithdrawProposalWithdrawNotAllowed,
		"F13WithdrawProposalCrashAfterPublish":  F13WithdrawProposalCrashAfterPublish,
	}
}

func F13WithdrawProposalOK(s *scenario.Scenario) {
	p := seedOpenProposal(s, 3)
	s.Given(scenario.AsSeededUser("alice", p.voters[0])).
		When(
			scenario.Post(p.votes, yes),
			scenario.ExpectStatus(http.StatusOK),
			scenario.Delete(p.path),
			scenario.ExpectStatus(http.StatusOK),
			scenario.ExpectJSON("status", "withdrawn"),
			scenario.ExpectJSON("my_ballot", "yes"),
			scenario.ExpectJSON("can_withdraw", false),
			scenario.Replay(),
		).
		Then(
			scenario.ExpectEvents(events.TypeProposalWithdrawn, 1),
			scenario.EventuallyPublished(events.TypeProposalWithdrawn, 1),
		)
}

func F13WithdrawProposalUnauthorized(s *scenario.Scenario) {
	p := seedOpenProposal(s, 1)
	s.Given(scenario.Anonymous()).
		When(scenario.Delete(p.path)).
		Then(scenario.ExpectProblem(errs.CodeUnauthorized), scenario.ExpectEvents(events.TypeProposalWithdrawn, 0))
}

func F13WithdrawProposalProposalNotFound(s *scenario.Scenario) {
	s.Given(scenario.AsUser("alice")).
		When(scenario.Delete("/v1/proposals/" + ids.Real{}.NewV7().String())).
		Then(scenario.ExpectProblem(errs.CodeProposalNotFound))
}

func F13WithdrawProposalNotProposer(s *scenario.Scenario) {
	p := seedOpenProposal(s, 2)
	s.Given(scenario.AsSeededUser("bob", p.voters[1])).
		When(scenario.Delete(p.path)).
		Then(scenario.ExpectProblem(errs.CodeNotProposer), scenario.ExpectEvents(events.TypeProposalWithdrawn, 0))
}

func F13WithdrawProposalProposalClosed(s *scenario.Scenario) {
	p := seedOpenProposal(s, 1)
	s.Given(scenario.AsSeededUser("alice", p.voters[0])).
		When(
			scenario.Delete(p.path),
			scenario.ExpectStatus(http.StatusOK),
			scenario.Delete(p.path),
		).
		Then(scenario.ExpectProblem(errs.CodeProposalClosed), scenario.ExpectEvents(events.TypeProposalWithdrawn, 1))
}

func F13WithdrawProposalWithdrawNotAllowed(s *scenario.Scenario) {
	p := seedOpenProposal(s, 3)
	s.Given(scenario.AsSeededUser("bob", p.voters[1])).
		When(
			scenario.Post(p.votes, no),
			scenario.ExpectStatus(http.StatusOK),
			scenario.AsSeededUser("alice", p.voters[0]),
			scenario.Delete(p.path),
		).
		Then(
			scenario.ExpectProblem(errs.CodeWithdrawNotAllowed),
			scenario.ExpectEvents(events.TypeProposalWithdrawn, 0),
		)
}

func F13WithdrawProposalCrashAfterPublish(s *scenario.Scenario) {
	p := seedOpenProposal(s, 1)
	s.Given(scenario.AsSeededUser("alice", p.voters[0]), scenario.HoldRelay()).
		When(
			scenario.Delete(p.path),
			scenario.ExpectStatus(http.StatusOK),
			scenario.ExpectJSON("status", "withdrawn"),
			scenario.PublishCrashingAt(faultpoint.AfterPublish),
		).
		Then(
			scenario.ExpectEvents(events.TypeProposalWithdrawn, 1),
			scenario.EventuallyPublished(events.TypeProposalWithdrawn, 1),
		)
}
