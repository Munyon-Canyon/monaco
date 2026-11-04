package flows

import (
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/platform/faultpoint"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
	"github.com/monaco/monaco/apps/backend/internal/testkit/fakes"
	"github.com/monaco/monaco/apps/backend/internal/testkit/scenario"
)

func proposeBuy(c f09Cabal) []scenario.Step {
	return []scenario.Step{
		f09Member(c),
		func(s *scenario.Scenario) { f09Route(s, "25000000", fakes.ActionSucceed, "/jupiter/swap/v2/order") },
	}
}

func proposalRowsIn(s *scenario.Scenario, cabal string) int {
	s.Helper()
	var got int
	err := s.DB().QueryRow(s.Context(),
		`SELECT count(*) FROM cabal_messages WHERE cabal_id = $1::uuid AND kind = 'proposal'`, cabal).Scan(&got)
	if err != nil {
		s.Fatalf("flows: count the proposal rows: %v", err)
	}
	return got
}

func F22eWriteChatProposalMessageOK(s *scenario.Scenario) {
	c := seedF09(s, "100000000")
	s.Given(proposeBuy(c)...).
		When(
			scenario.Post(f09Path(c), f09Buy),
			scenario.ExpectStatus(http.StatusCreated),
		).
		Then(
			scenario.Eventually("the proposal row", func(s *scenario.Scenario) bool {
				return proposalRowsIn(s, c.id.String()) == 1
			}),
			func(s *scenario.Scenario) {
				if got := proposalRowsIn(s, c.id.String()); got != 1 {
					s.Fatalf("flows: the cabal holds %d proposal rows, want 1", got)
				}
			},
		)
}

func F22eWriteChatProposalMessageProposalNotFound(s *scenario.Scenario) {
	s.Given(chatCabal("proposal-missing")...).
		When(
			func(s *scenario.Scenario) {
				publishEvent(s, events.ProposalCreated{
					V: 1, ProposalID: uuid.MustParse("01920000-0000-7000-8000-0000000000d1"),
					CabalID: uuid.MustParse(s.Recall("cabal")), ProposerID: s.ActorID().UUID(),
					Kind: "buy", Symbol: "NVDA", ExpiresAt: time.Date(2026, 9, 30, 14, 0, 0, 0, time.UTC),
				})
			},
			scenario.EventuallyLog(observability.BusDispatched, map[string]string{
				"subject": s.Bus().Subject(events.TypeProposalCreated.Subject()),
				"code":    string(errs.CodeProposalNotFound),
			}),
		).
		Then(func(s *scenario.Scenario) {
			if got := proposalRowsIn(s, s.Recall("cabal")); got != 0 {
				s.Fatalf("flows: the cabal holds %d proposal rows, want 0", got)
			}
		})
}

func F22eWriteChatProposalMessageCrashBeforeCommit(s *scenario.Scenario) {
	c := seedF09(s, "100000000")
	s.Given(proposeBuy(c)...).
		When(
			scenario.Post(f09Path(c), f09Buy),
			scenario.ExpectStatus(http.StatusCreated),
			scenario.PublishCrashingAt(faultpoint.BeforeCommit),
		).
		Then(
			scenario.Eventually("the proposal row", func(s *scenario.Scenario) bool {
				return proposalRowsIn(s, c.id.String()) == 1
			}),
			func(s *scenario.Scenario) {
				if got := proposalRowsIn(s, c.id.String()); got != 1 {
					s.Fatalf("flows: the cabal holds %d proposal rows, want 1", got)
				}
			},
		)
}

func (defined) AloneF22e() []Script {
	return []Script{F22eWriteChatProposalMessageProposalNotFound}
}

func (defined) LettersF22e() map[string][]Letter {
	missing := Letter{Subject: events.TypeProposalCreated, Code: errs.CodeProposalNotFound}
	return map[string][]Letter{"F22eWriteChatProposalMessageProposalNotFound": {missing, missing}}
}
