package flows

import (
	"net/http"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/testkit/fakes"
	"github.com/monaco/monaco/apps/backend/internal/testkit/scenario"
)

const leavePath = membersPath + "/me"

func cabalIn(s *scenario.Scenario) ids.CabalID {
	id, err := ids.ParseCabalID(s.Recall("cabal"))
	if err != nil {
		s.Fatalf("flows: remembered cabal %q: %v", s.Recall("cabal"), err)
	}
	return id
}

func founderOf(script string) string { return "did:privy:qa-f04-" + script + "-creator" }

func memberOf(script string) string { return "did:privy:qa-f04-" + script + "-member" }

func founded(script string) []scenario.Step {
	return []scenario.Step{
		scenario.SignIn(founderOf(script)),
		scenario.Post(cabalsPath, cabalOf("open", "all")),
		scenario.ExpectStatus(http.StatusCreated),
		scenario.Remember("id", "cabal"),
	}
}

func joined(script string) []scenario.Step {
	return append(founded(script),
		scenario.SignIn(memberOf(script)),
		scenario.Post(membersPath, ""),
		scenario.ExpectStatus(http.StatusOK),
	)
}

func treasuryFor(s *scenario.Scenario, tr *fakes.Treasury) *fakes.Treasury {
	if tr == nil {
		s.Fatalf("flows: flow 04 sets shares and the pot on a fakes.Treasury until treasury reads real ones")
	}
	return tr
}

func left() []scenario.Step {
	return []scenario.Step{
		scenario.ExpectEvents(events.TypeCabalMemberLeft, 1),
		scenario.EventuallyPublished(events.TypeCabalMemberLeft, 1),
		scenario.EventuallyHint("cabal_access"),
	}
}

func F04LeaveCabalOK(s *scenario.Scenario) {
	s.Given(joined("ok")...).
		When(
			scenario.Delete(leavePath),
			scenario.ExpectStatus(http.StatusNoContent),
			scenario.Retry(),
			scenario.ExpectStatus(http.StatusNoContent),
		).
		Then(left()...)
}

func F04LeaveCabalUnauthorized(s *scenario.Scenario) {
	s.Given(scenario.Anonymous()).
		When(scenario.Delete(missingCabal + "/members/me")).
		Then(scenario.ExpectProblem(errs.CodeUnauthorized))
}

func F04LeaveCabalNotCabalMember(s *scenario.Scenario) {
	s.Given(founded("outsider")...).
		When(scenario.SignIn(memberOf("outsider")), scenario.Delete(leavePath)).
		Then(refused(errs.CodeNotCabalMember, events.TypeCabalMemberLeft, 0)...)
}

func F04LeaveCabalLeaveHoldsShares(s *scenario.Scenario) { LeaveHoldingShares(s, nil) }

func LeaveHoldingShares(s *scenario.Scenario, tr *fakes.Treasury) {
	s.Given(append(joined("shares"), func(s *scenario.Scenario) {
		treasuryFor(s, tr).SetStake(treasury.Stake{
			CabalID: cabalIn(s), UserID: s.ActorID(), ShareUnits: money.SharesUnitsFromUint64(1),
		})
	})...).
		When(scenario.Delete(leavePath)).
		Then(refused(errs.CodeLeaveHoldsShares, events.TypeCabalMemberLeft, 0)...)
}

func F04LeaveCabalLeaveLastMemberPotNotEmpty(s *scenario.Scenario) { LeaveLastOfAFullPot(s, nil) }

func LeaveLastOfAFullPot(s *scenario.Scenario, tr *fakes.Treasury) {
	s.Given(append(founded("last"), func(s *scenario.Scenario) {
		treasuryFor(s, tr).SetPotValue(cabalIn(s), money.MicrosFromUint64(1))
	})...).
		When(scenario.Delete(leavePath)).
		Then(refused(errs.CodeLeaveLastMemberPotNotEmpty, events.TypeCabalMemberLeft, 0)...)
}

func F04LeaveCabalLeaveCreatorWithMembers(s *scenario.Scenario) {
	s.Given(joined("creator")...).
		When(scenario.SignIn(founderOf("creator")), scenario.Delete(leavePath)).
		Then(refused(errs.CodeLeaveCreatorWithMembers, events.TypeCabalMemberLeft, 0)...)
}

func F04LeaveCabalPriceUnavailable(s *scenario.Scenario) { LeaveAnUnpricedPot(s, nil) }

func LeaveAnUnpricedPot(s *scenario.Scenario, tr *fakes.Treasury) {
	s.Given(append(founded("unpriced"), func(s *scenario.Scenario) {
		treasuryFor(s, tr).Fail("PotValue", errs.New(errs.CodePriceUnavailable, "flows.LeaveAnUnpricedPot"))
	})...).
		When(scenario.Delete(leavePath)).
		Then(refused(errs.CodePriceUnavailable, events.TypeCabalMemberLeft, 0)...)
}

func F04LeaveCabalCrashBeforeCommit(s *scenario.Scenario) {
	s.Given(joined("crash")...).
		When(
			scenario.Delete(leavePath),
			scenario.Retry(),
			scenario.ExpectStatus(http.StatusNoContent),
		).
		Then(left()...)
}
