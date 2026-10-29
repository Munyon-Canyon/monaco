package flows

import (
	"net/http"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/testkit/scenario"
)

func (defined) SeedsF10a() map[string]Seeder {
	return map[string]Seeder{
		"F10aRetractVoteOK":               seedSignedInWith(arrangeVotable),
		"F10aRetractVoteUnauthorized":     seedAnonymous,
		"F10aRetractVoteProposalNotFound": seedSignedIn,
		"F10aRetractVoteNotAVoter":        seedSignedInWith(arrangeForeignProposal),
		"F10aRetractVoteProposalClosed":   seedSignedInWith(arrangeClosedProposal),
		"F10aRetractVoteVoteNotFound":     seedSignedInWith(arrangeVotable),
		"F10aRetractVoteUndoExpired":      seedSignedInWith(arrangeVotable),
	}
}

func F10aRetractVoteOK(s *scenario.Scenario) {
	p := seedOpenProposal(s, 3)
	s.Given(scenario.AsSeededUser("alice", p.voters[0])).
		When(
			scenario.Post(p.votes, yes),
			scenario.ExpectStatus(http.StatusOK),
			scenario.Delete(p.votes),
			scenario.ExpectStatus(http.StatusOK),
			scenario.ExpectJSON("status", "open"),
			scenario.ExpectJSON("tally", map[string]int{"yes": 0, "no": 0, "voters": 3, "needed": 2}),
			scenario.Replay(),
			scenario.Get(p.path),
			scenario.ExpectJSON("my_ballot", nil),
		).
		Then(
			scenario.ExpectEvents(events.TypeVoteRetracted, 1),
			scenario.ExpectEventPayload(events.TypeVoteRetracted, map[string]any{
				"v": 1, "proposal_id": p.id.String(), "cabal_id": p.cabalID.String(),
				"voter_id": p.voters[0].String(), "choice": "yes",
			}),
			scenario.EventuallyPublished(events.TypeVoteRetracted, 1),
			scenario.EventuallyCaptured(events.TypeVoteRetracted, "vote_retracted", p.id),
			scenario.EventuallyCabalHint(p.cabalID, "proposal_updated"),
		)
}

func F10aRetractVoteUnauthorized(s *scenario.Scenario) {
	p := seedOpenProposal(s, 1)
	s.Given(scenario.Anonymous()).
		When(scenario.Delete(p.votes)).
		Then(scenario.ExpectProblem(errs.CodeUnauthorized), scenario.ExpectEvents(events.TypeVoteRetracted, 0))
}

func F10aRetractVoteProposalNotFound(s *scenario.Scenario) {
	s.Given(scenario.AsUser("alice")).
		When(scenario.Delete("/v1/proposals/" + ids.Real{}.NewV7().String() + "/votes")).
		Then(scenario.ExpectProblem(errs.CodeProposalNotFound))
}

func F10aRetractVoteNotAVoter(s *scenario.Scenario) {
	p := seedOpenProposal(s, 1)
	s.Given(scenario.AsUser("mallory")).
		When(scenario.Delete(p.votes)).
		Then(scenario.ExpectProblem(errs.CodeNotAVoter), scenario.ExpectEvents(events.TypeVoteRetracted, 0))
}

func F10aRetractVoteProposalClosed(s *scenario.Scenario) {
	p := seedOpenProposal(s, 1)
	s.Given(scenario.AsSeededUser("alice", p.voters[0])).
		When(
			scenario.Post(p.votes, no),
			scenario.ExpectJSON("status", "failed"),
			scenario.Delete(p.votes),
		).
		Then(
			scenario.ExpectProblem(errs.CodeProposalClosed),
			scenario.ExpectEvents(events.TypeVoteRetracted, 0),
			scenario.Get(p.path),
			scenario.ExpectJSON("my_ballot", "no"),
		)
}

func F10aRetractVoteVoteNotFound(s *scenario.Scenario) {
	p := seedOpenProposal(s, 3)
	s.Given(scenario.AsSeededUser("alice", p.voters[0])).
		When(scenario.Delete(p.votes)).
		Then(scenario.ExpectProblem(errs.CodeVoteNotFound), scenario.ExpectEvents(events.TypeVoteRetracted, 0))
}

func F10aRetractVoteUndoExpired(s *scenario.Scenario) {
	p := seedOpenProposal(s, 3)
	s.Given(scenario.AsSeededUser("alice", p.voters[0])).
		When(
			scenario.Post(p.votes, yes),
			scenario.Post(p.votes, no),
			scenario.ExpectJSON("my_ballot", "no"),
			scenario.Delete(p.votes),
		).
		Then(
			scenario.ExpectProblem(errs.CodeUndoExpired),
			scenario.ExpectEvents(events.TypeVoteRetracted, 0),
			scenario.Get(p.path),
			scenario.ExpectJSON("my_ballot", "no"),
		)
}
