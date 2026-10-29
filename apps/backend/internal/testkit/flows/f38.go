package flows

import (
	"net/http"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
	"github.com/monaco/monaco/apps/backend/internal/testkit/scenario"
)

const (
	f38ShowSides = `{"change":{"rule":"show_sides","to":true},"thesis":"see who voted"}`
	f38Unchanged = `{"change":{"rule":"show_sides","to":false}}`
	f38BadValue  = `{"change":{"rule":"show_sides","to":"yes"}}`
)

func f38Cabal(s *scenario.Scenario, opts ...testkit.CabalOption) testkit.SeededCabal {
	s.Helper()
	opts = append([]testkit.CabalOption{testkit.WithMembers(3)}, opts...)
	return testkit.NewCabal(seedT{s}, s.DB(), opts...)
}

func f38Path(c testkit.SeededCabal) string { return "/v1/cabals/" + c.ID.String() + "/rule-proposals" }

func f38Refused(s *scenario.Scenario, as scenario.Step, c testkit.SeededCabal, body string, code errs.Code) {
	s.Given(as).
		When(scenario.Post(f38Path(c), body)).
		Then(scenario.ExpectProblem(code), scenario.ExpectEvents(events.TypeProposalCreated, 0))
}

func F38ProposeRuleChangeOK(s *scenario.Scenario) {
	c := f38Cabal(s)
	s.Given(scenario.AsSeededUser("bob", c.Members[1].ID)).
		When(
			scenario.Post(f38Path(c), f38ShowSides),
			scenario.ExpectStatus(http.StatusCreated),
			scenario.ExpectJSON("kind", "rule"),
			scenario.ExpectJSON("status", "open"),
			scenario.ExpectJSON("symbol", nil),
			scenario.ExpectJSON("usdc_micros", nil),
			scenario.ExpectJSON("rule_change", map[string]any{"rule": "show_sides", "from": false, "to": true, "user": nil}),
			scenario.ExpectJSON("tally", map[string]int{"yes": 0, "no": 0, "voters": 3, "needed": 2}),
			scenario.Replay(),
			scenario.Remember("id", "proposal"),
			scenario.AsSeededUser("alice", c.Members[0].ID),
			scenario.Post("/v1/proposals/{proposal}/votes", yes),
			scenario.ExpectJSON("status", "open"),
			scenario.AsSeededUser("bob", c.Members[1].ID),
			scenario.Post("/v1/proposals/{proposal}/votes", yes),
			scenario.ExpectJSON("status", "passed"),
		).
		Then(
			scenario.ExpectEvents(events.TypeProposalCreated, 1),
			scenario.ExpectEventPayload(events.TypeProposalCreated, map[string]any{"kind": "rule", "voter_count": 3}),
			scenario.ExpectEvents(events.TypeProposalRulePassed, 1),
			scenario.ExpectEventPayload(events.TypeProposalRulePassed, map[string]any{
				"cabal_id": c.ID.String(), "proposer_id": c.Members[1].ID.String(),
			}),
			scenario.ExpectEvents(events.TypeProposalPassed, 0),
		)
}

func F38ProposeRuleChangeInvalidInput(s *scenario.Scenario) {
	c := f38Cabal(s)
	f38Refused(s, scenario.AsSeededUser("bob", c.Members[1].ID), c, f38BadValue, errs.CodeInvalidInput)
}

func F38ProposeRuleChangeNotAVoter(s *scenario.Scenario) {
	c := f38Cabal(s, testkit.WithVoterMode("list"))
	f38Refused(s, scenario.AsSeededUser("bob", c.Members[1].ID), c, f38ShowSides, errs.CodeNotAVoter)
}

func F38ProposeRuleChangeNotCabalMember(s *scenario.Scenario) {
	c := f38Cabal(s)
	f38Refused(s, scenario.AsUser("mallory"), c, f38ShowSides, errs.CodeNotCabalMember)
}

func F38ProposeRuleChangeRuleUnchanged(s *scenario.Scenario) {
	c := f38Cabal(s)
	f38Refused(s, scenario.AsSeededUser("bob", c.Members[1].ID), c, f38Unchanged, errs.CodeRuleUnchanged)
}

func F38ProposeRuleChangeRuleChangeOpen(s *scenario.Scenario) {
	c := f38Cabal(s)
	s.Given(scenario.AsSeededUser("bob", c.Members[1].ID)).
		When(
			scenario.Post(f38Path(c), f38ShowSides),
			scenario.ExpectStatus(http.StatusCreated),
			scenario.AsSeededUser("alice", c.Members[0].ID),
			scenario.Post(f38Path(c), f38ShowSides),
		).
		Then(scenario.ExpectProblem(errs.CodeRuleChangeOpen), scenario.ExpectEvents(events.TypeProposalCreated, 1))
}
