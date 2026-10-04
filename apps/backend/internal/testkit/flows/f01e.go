package flows

import (
	"net/http"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
	"github.com/monaco/monaco/apps/backend/internal/testkit/scenario"
)

const (
	deleteOK      = "did:privy:qa-del-ok"
	deleteOther   = "did:privy:qa-del-ok-other"
	deleteStaked  = "did:privy:qa-del-staked"
	deleteFunded  = "did:privy:qa-del-funded"
	deleteCrash   = "did:privy:qa-del-crash"
	deletedHandle = "del_gone"
)

func F01eDeleteAccountOK(s *scenario.Scenario) {
	s.Given(withHandle(deleteOK, deletedHandle)...).
		When(scenario.Delete(me), scenario.ExpectStatus(http.StatusNoContent)).
		Then(
			actorLogged(observability.IdentityAccountDeleted),
			scenario.ExpectEvents(events.TypeUserDeleted, 1),
			scenario.EventuallyPublished(events.TypeUserDeleted, 1),
			scenario.SignIn(deleteOK),
			scenario.ExpectProblem(errs.CodeAccountDeleted),
			scenario.SignIn(deleteOther),
			scenario.ExpectStatus(http.StatusOK),
			scenario.Put(setHandlePath, `{"handle":"`+deletedHandle+`"}`),
			scenario.ExpectProblem(errs.CodeHandleTaken),
		)
}

func F01eDeleteAccountAccountHasPositions(s *scenario.Scenario) {
	deleteRefused(s, deleteStaked, errs.CodeAccountHasPositions)
}

func F01eDeleteAccountAccountHasBalance(s *scenario.Scenario) {
	deleteRefused(s, deleteFunded, errs.CodeAccountHasBalance)
}

func F01eDeleteAccountCrashBeforeCommit(s *scenario.Scenario) {
	s.Given(scenario.SignIn(deleteCrash), scenario.ExpectStatus(http.StatusOK)).
		When(scenario.Delete(me), scenario.Retry(), scenario.ExpectStatus(http.StatusNoContent)).
		Then(
			scenario.ExpectEvents(events.TypeUserDeleted, 1),
			scenario.EventuallyPublished(events.TypeUserDeleted, 1),
			scenario.SignIn(deleteCrash),
			scenario.ExpectProblem(errs.CodeAccountDeleted),
		)
}

func deleteRefused(s *scenario.Scenario, sub string, code errs.Code) {
	s.Given(scenario.SignIn(sub), scenario.ExpectStatus(http.StatusOK)).
		When(scenario.Delete(me)).
		Then(
			scenario.ExpectProblem(code),
			scenario.ExpectEvents(events.TypeUserDeleted, 0),
			scenario.Get(me),
			scenario.ExpectStatus(http.StatusOK),
		)
}
